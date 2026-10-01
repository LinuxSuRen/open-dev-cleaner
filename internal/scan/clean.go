package scan

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// CleanResult reports the outcome of cleaning one target.
type CleanResult struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	OK      bool   `json:"ok"`
	Freed   int64  `json:"freed"`
	Message string `json:"message"`
}

// Cleaner executes the cleaning of targets. All output is streamed to Out
// line by line so the web UI and the CLI share the same behaviour.
type Cleaner struct {
	Env *Env
	Out io.Writer
}

// NewCleaner builds a cleaner writing logs to out.
func NewCleaner(env *Env, out io.Writer) *Cleaner {
	if out == nil {
		out = os.Stdout
	}
	return &Cleaner{Env: env, Out: out}
}

func (c *Cleaner) log(format string, args ...any) {
	fmt.Fprintf(c.Out, format+"\n", args...)
}

// Clean executes one target. It validates dir targets against AllowedRoot
// before touching the filesystem. For a group it recurses into children.
func (cl *Cleaner) Clean(ctx context.Context, t *Target, dry bool) CleanResult {
	if t == nil {
		return CleanResult{OK: false, Message: "目标不存在(请先重新扫描)"}
	}
	if !t.Available {
		return CleanResult{ID: t.ID, Title: t.Title, OK: false, Message: "该目标当前不可用: " + t.Note}
	}

	switch t.Method {
	case MethodGroup:
		var total int64
		fails := 0
		for _, child := range t.Items {
			r := cl.Clean(ctx, child, dry)
			total += r.Freed
			if !r.OK {
				fails++
			}
		}
		msg := fmt.Sprintf("已处理 %d 个子项,释放 %s", len(t.Items), FormatSize(total))
		if fails > 0 {
			msg += fmt.Sprintf(",%d 个失败", fails)
		}
		return CleanResult{ID: t.ID, Title: t.Title, OK: fails == 0, Freed: total, Message: msg}

	case MethodDir:
		return cl.cleanDir(ctx, t, dry, false)
	case MethodDirContents:
		return cl.cleanDir(ctx, t, dry, true)
	case MethodCommand:
		return cl.cleanCommand(ctx, t, dry)
	default:
		return CleanResult{ID: t.ID, Title: t.Title, OK: false, Message: "未知清理方式: " + t.Method}
	}
}

func (cl *Cleaner) cleanDir(ctx context.Context, t *Target, dry, contentsOnly bool) CleanResult {
	res := CleanResult{ID: t.ID, Title: t.Title, Freed: t.Size}
	path := t.Path

	if err := validateRemovable(t); err != nil {
		res.Message = fmt.Sprintf("拒绝清理 %s: %v", path, err)
		cl.log("[拒绝] %s", res.Message)
		return res
	}
	if !Exists(path) {
		res.OK = true
		res.Message = "路径已不存在,视为成功"
		return res
	}

	label := "目录"
	if contentsOnly {
		label = "目录内容"
	}
	if dry {
		res.OK = true
		res.Message = fmt.Sprintf("[dry-run] 将删除%s: %s (%s)", label, path, FormatSize(t.Size))
		cl.log("%s", res.Message)
		return res
	}

	var err error
	if contentsOnly {
		err = removeContents(path)
	} else {
		err = os.RemoveAll(path)
	}
	if err != nil {
		res.Message = fmt.Sprintf("删除失败: %v", err)
		cl.log("[失败] %s: %v", path, err)
		return res
	}
	res.OK = true
	res.Message = fmt.Sprintf("已删除%s: %s,释放 %s", label, path, FormatSize(t.Size))
	cl.log("[完成] %s", res.Message)
	return res
}

func (cl *Cleaner) cleanCommand(ctx context.Context, t *Target, dry bool) CleanResult {
	res := CleanResult{ID: t.ID, Title: t.Title, Freed: t.Size}
	if len(t.Cmd) == 0 {
		res.Message = "目标没有配置清理命令"
		return res
	}
	display := strings.Join(t.Cmd, " ")
	if dry {
		res.OK = true
		res.Message = fmt.Sprintf("[dry-run] 将执行: %s", display)
		cl.log("%s", res.Message)
		return res
	}

	// ollama commands need a running server; when it is down, start it
	// temporarily (same as during scanning) and stop it afterwards.
	var stopDaemon func()
	if t.Cmd[0] == "ollama" && cl.Env != nil && !cl.Env.NoExternal && !ollamaReady() {
		started, stop, err := startOllamaServer(ctx, cl.Env)
		if started {
			stopDaemon = stop
			cl.log("[服务] Ollama 未运行,已临时启动,执行完成后将自动关闭")
		} else {
			cl.log("[警告] 无法临时启动 Ollama 服务: %v", err)
		}
	}

	cl.log("[执行] %s", display)

	cmd := exec.CommandContext(ctx, t.Cmd[0], t.Cmd[1:]...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if stopDaemon != nil {
		stopDaemon()
		cl.log("[服务] 临时 Ollama 服务已关闭")
	}
	sc := bufio.NewScanner(&buf)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := stripANSI(strings.TrimSpace(sc.Text()))
		if line != "" {
			cl.log("  %s", line)
		}
	}
	if err != nil {
		// Non-zero exit may still free space (e.g. some items in use).
		res.Message = fmt.Sprintf("命令返回错误: %v", err)
		cl.log("[警告] %s", res.Message)
		return res
	}
	res.OK = true
	res.Message = fmt.Sprintf("命令执行成功: %s", display)
	return res
}

// stripANSI removes terminal control sequences (colors, cursor moves)
// that external CLIs (ollama, docker...) print even when not attached to
// a TTY, plus carriage returns from progress lines.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\r`)

func stripANSI(s string) string {
	return strings.TrimSpace(ansiRe.ReplaceAllString(s, ""))
}

// validateRemovable enforces the safety net for filesystem deletion: the
// path must live strictly inside its declared allowed root.
func validateRemovable(t *Target) error {
	if t.AllowedRoot == "" {
		return fmt.Errorf("目标未声明安全根目录 (allowedRoot)")
	}
	if t.Path == "" {
		return fmt.Errorf("目标为空路径")
	}
	root := filepath.Clean(t.AllowedRoot)
	path := filepath.Clean(t.Path)
	if path == root {
		return fmt.Errorf("不允许删除安全根本身: %s", root)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("路径 %s 不在安全根 %s 内", path, root)
	}
	return nil
}

func removeContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
