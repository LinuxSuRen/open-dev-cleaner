// Package cli implements the command line interface: scan, clean, web.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/LinuxSuRen/open-dev-cleaner/internal/scan"
	"github.com/LinuxSuRen/open-dev-cleaner/internal/server"
)

// Version info overridden by -ldflags in main.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

const usage = `Open Dev Cleaner — 开发者磁盘清理工具 (https://github.com/LinuxSuRen/open-dev-cleaner)

用法:
  open-dev-cleaner web [--addr 127.0.0.1:8420] [--no-open]   启动 Web 界面(默认)
  open-dev-cleaner scan [--format table|json] [--risk safe|caution|high|dangerous|all]
  open-dev-cleaner clean --ids id1,id2 [--dry-run] [--yes]
  open-dev-cleaner version

环境变量:
  ODC_HOME       覆盖用户主目录(调试用)
  ODC_CACHE      覆盖用户缓存目录
  ODC_APPDATA    覆盖应用数据目录
  ODC_WORKSPACE  追加工作区根目录(多个用 : 分隔)
`

// Run parses args and dispatches. webFS is the embedded frontend.
func Run(args []string, webFS fs.FS) error {
	cmd := "web"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "web", "serve":
		return runWeb(args, webFS)
	case "scan":
		return runScan(args)
	case "clean":
		return runClean(args)
	case "version", "-v", "--version":
		fmt.Printf("open-dev-cleaner %s (%s, %s, %s/%s)\n", Version, Commit, BuildDate, runtime.GOOS, runtime.GOARCH)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("未知命令 %q\n\n%s", cmd, usage)
	}
}

func runWeb(args []string, webFS fs.FS) error {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	addr := flags.String("addr", "127.0.0.1:8420", "HTTP 监听地址")
	noOpen := flags.Bool("no-open", false, "不自动打开浏览器")
	noExternal := flags.Bool("no-external", false, "不调用外部命令(docker/brew/ollama)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	env := scan.DetectEnv()
	env.NoExternal = *noExternal

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sub, err := fsSub(webFS)
	if err != nil {
		return err
	}
	srv := server.New(env, sub)
	server.Version, server.Commit, server.BuildDate = Version, Commit, BuildDate

	if !*noOpen {
		go openBrowser("http://" + firstNonEmpty(hostOf(*addr), "127.0.0.1") + portPath(*addr))
	}
	return srv.ListenAndServe(ctx, *addr)
}

// fsSub normalizes an embedded FS to its content root, supporting both
// "web/index.html" (embed.FS) and "index.html" (fs.Sub / tests).
func fsSub(webFS fs.FS) (fs.FS, error) {
	if _, err := webFS.Open("index.html"); err == nil {
		return webFS, nil
	}
	if sub, err := fs.Sub(webFS, "web"); err == nil {
		if _, err := sub.Open("index.html"); err == nil {
			return sub, nil
		}
	}
	return nil, fmt.Errorf("前端资源缺失")
}

func hostOf(addr string) string {
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		return addr[:i]
	}
	return ""
}

func portPath(addr string) string {
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		return addr[i:]
	}
	return ":8420"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond) // let the listener start
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func runScan(args []string) error {
	f := flag.NewFlagSet("scan", flag.ContinueOnError)
	format := f.String("format", "table", "输出格式: table | json")
	risk := f.String("risk", "all", "按风险过滤: safe|caution|high|dangerous|all")
	noExternal := f.Bool("no-external", false, "不调用外部命令")
	if err := f.Parse(args); err != nil {
		return err
	}

	env := scan.DetectEnv()
	env.NoExternal = *noExternal
	engine := scan.NewEngine(env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	targets := engine.Run(ctx, func(ev scan.Event) {
		if ev.Type == "target" && ev.Target != nil && *format == "table" && matchRisk(*risk, ev.Target.Risk) {
			printTarget(os.Stdout, ev.Target)
		}
	})

	var filtered []*scan.Target
	for _, t := range targets {
		if matchRisk(*risk, t.Risk) {
			filtered = append(filtered, t)
		}
	}

	if *format == "json" {
		out := struct {
			Targets []*scan.Target `json:"targets"`
			Summary *scan.Summary  `json:"summary"`
		}{Targets: filtered, Summary: scan.Summarize(filtered)}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return nil
	}

	fmt.Printf("\n共 %d 项,合计约 %s\n", len(filtered), scan.FormatSize(scan.Summarize(filtered).TotalSize))
	return nil
}

func matchRisk(filter string, r scan.RiskLevel) bool {
	return filter == "all" || filter == string(r)
}

func printTarget(w io.Writer, t *scan.Target) {
	fmt.Fprintf(w, "%-10s %-4s %10s  %s\n%s\n",
		t.Tool, t.Risk.RiskLabel(), scan.FormatSize(t.SumSize()), t.Title, "    "+t.Path)
}
func runClean(args []string) error {
	f := flag.NewFlagSet("clean", flag.ContinueOnError)
	ids := f.String("ids", "", "要清理的目标 ID,逗号分隔(来自 scan 输出)")
	dry := f.Bool("dry-run", false, "只预览不执行")
	yes := f.Bool("yes", false, "跳过交互确认")
	risk := f.String("risk", "safe", "搭配 --all 使用: 清理指定风险等级")
	all := f.Bool("all", false, "清理扫描结果中匹配 --risk 的全部目标")
	noExternal := f.Bool("no-external", false, "不调用外部命令")
	if err := f.Parse(args); err != nil {
		return err
	}

	env := scan.DetectEnv()
	env.NoExternal = *noExternal
	engine := scan.NewEngine(env)

	fmt.Println("正在扫描……")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var targets []*scan.Target
	targets = engine.Run(ctx, func(ev scan.Event) {
		if ev.Type == "target" && ev.Target != nil {
			targets = append(targets, ev.Target)
		}
	})

	var selected []*scan.Target
	if *all {
		for _, t := range targets {
			if matchRisk(*risk, t.Risk) {
				selected = append(selected, t)
			}
		}
	} else if *ids != "" {
		for _, id := range strings.Split(*ids, ",") {
			id = strings.TrimSpace(id)
			if t := scan.FindTarget(targets, id); t != nil {
				selected = append(selected, t)
			} else {
				fmt.Printf("未找到目标: %s\n", id)
			}
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("没有匹配的清理目标")
	}

	var total int64
	for _, t := range selected {
		total += t.SumSize()
	}
	fmt.Printf("将清理 %d 项,预计释放 %s:\n", len(selected), scan.FormatSize(total))
	for _, t := range selected {
		fmt.Printf("  [%s] %-4s %10s  %s (%s)\n", t.ID, t.Risk.RiskLabel(), scan.FormatSize(t.SumSize()), t.Title, t.Path)
	}

	if !*yes && !*dry {
		fmt.Print("确认执行? 输入 yes 继续: ")
		var answer string
		_, _ = fmt.Scanln(&answer)
		if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
			return fmt.Errorf("已取消")
		}
	}

	cleaner := scan.NewCleaner(env, os.Stdout)
	var freed int64
	for _, t := range selected {
		res := cleaner.Clean(ctx, t, *dry)
		if res.OK {
			freed += res.Freed
		}
	}
	fmt.Printf("完成,预计释放 %s\n", scan.FormatSize(freed))
	return nil
}

// sortTargets is a small helper kept for CLI sorting needs.
func sortTargets(ts []*scan.Target) {
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].SumSize() > ts[j].SumSize() })
}
