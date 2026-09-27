package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	exec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// OllamaProvider lists locally installed LLMs which often occupy tens of
// gigabytes.
type OllamaProvider struct{}

func (p *OllamaProvider) Key() string   { return "ollama" }
func (p *OllamaProvider) Title() string { return "Ollama" }

type ollamaModel struct {
	name string
	size int64
}

func (p *OllamaProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()
	modelsDir := filepath.Join(env.Home, ".ollama", "models")
	if !Exists(modelsDir) {
		return // ollama not installed
	}

	out, err := runCmd(ctx, env, 15*time.Second, "ollama", "list")
	note := ""
	if err != nil {
		// Server not running: start it temporarily, scan, then stop it
		// again (only the process we spawned ourselves).
		started, stop, startErr := startOllamaServer(ctx, env)
		if started {
			defer stop()
			note = "Ollama 服务未运行,已临时启动完成扫描,结束后自动关闭"
			out, err = runCmd(ctx, env, 15*time.Second, "ollama", "list")
		}
		if err != nil {
			reason := firstLine(err.Error())
			if startErr != nil {
				reason = firstLine(startErr.Error())
			}
			if errors.Is(err, exec.ErrNotFound) {
				reason = "未找到 ollama 命令"
			}
			full := "ollama list 执行失败:" + reason
			size, files := DirSize(ctx, modelsDir)
			emit(&Target{
				ID: toolUnavailable(tool), Tool: tool, ToolTitle: title,
				Category: "大模型", Title: "Ollama 模型目录",
				Path: modelsDir, Description: full, Note: full,
				Risk: RiskHigh, Size: size, Count: files, Available: false,
			})
			return
		}
	}

	models := parseOllamaList(out)
	if len(models) == 0 {
		return
	}

	var total int64
	children := make([]*Target, 0, len(models))
	for _, m := range models {
		total += m.size
		children = append(children, &Target{
			ID: tool + ":model:" + m.name, Tool: tool, ToolTitle: title,
			Category: "大模型", Title: m.name,
			Description: fmt.Sprintf("模型 %s,删除后可用 ollama pull %s 重新拉取", m.name, m.name),
			Risk:        RiskHigh, Size: m.size,
			Method: MethodCommand, Cmd: []string{"ollama", "rm", m.name},
			Available: true,
		})
	}

	emit(&Target{
		ID: tool + ":models", Tool: tool, ToolTitle: title,
		Category: "大模型", Title: fmt.Sprintf("Ollama 本地模型 (%d 个)", len(models)),
		Path: modelsDir, Note: note,
		Description: "本地大模型权重,删除单个模型后可随时 ollama pull 重新拉取(需要网络与时间)",
		Risk:        RiskHigh, Size: total,
		Method: MethodGroup, Available: true,
		Items: children,
	})
}

// startOllamaServer temporarily boots `ollama serve` so models can be
// listed, returning a stop function for the process we spawned. It never
// touches an externally managed server: when OLLAMA_HOST points to a
// custom address nothing is started.
func startOllamaServer(ctx context.Context, env *Env) (bool, func(), error) {
	if env == nil || env.NoExternal {
		return false, nil, errNoExternal
	}
	if os.Getenv("OLLAMA_HOST") != "" {
		return false, nil, errors.New("OLLAMA_HOST 指向自定义服务,不自动启动本地服务")
	}

	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return false, nil, err
	}
	defer devnull.Close()

	cmd := exec.CommandContext(ctx, "ollama", "serve")
	cmd.Stdout, cmd.Stderr = devnull, devnull
	if err := cmd.Start(); err != nil {
		return false, nil, err
	}

	// Wait until the HTTP API answers (or give up after 20s).
	deadline := time.Now().Add(20 * time.Second)
	ready := false
	for !ready && time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		ready = ollamaReady()
		if !ready {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if !ready {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return false, nil, errors.New("临时启动 Ollama 服务超时(20s)")
	}

	stop := func() {
		if cmd.Process == nil {
			return
		}
		if runtime.GOOS == "windows" {
			_ = cmd.Process.Kill()
		} else {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}
	return true, stop, nil
}

// ollamaReady probes `ollama list` with a short timeout.
func ollamaReady() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ollama", "list")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// parseOllamaList parses output like (both size formats occur in the
// wild depending on version):
//
//	NAME             ID            SIZE      MODIFIED
//	deepseek-r1:14b  3d9f3a1b2c4d  9.0GB     3 days ago
//	llama3.2:3b      a80c4f17acd5  2.0 GB    18 months ago
func parseOllamaList(out string) []ollamaModel {
	var models []ollamaModel
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] == "NAME" {
			continue
		}
		name := fields[0]
		sizeStr := findSizeField(fields)
		if sizeStr == "" {
			continue
		}
		models = append(models, ollamaModel{name: name, size: ParseSize(sizeStr)})
	}
	return models
}

// findSizeField locates the SIZE column: either one token ("9.0GB") or
// two tokens ("2.0" + "GB").
func findSizeField(fields []string) string {
	for i := 1; i < len(fields); i++ {
		f := fields[i]
		// single token like "9.0GB"
		if n := numPrefixLen(f); n > 0 && n < len(f) && isUnitOnly(f[n:]) {
			return f
		}
		// two tokens like "2.0" "GB"
		if i >= 2 && len(fields[i-1]) > 0 && numPrefixLen(fields[i-1]) == len(fields[i-1]) && isUnitOnly(f) {
			return fields[i-1] + f
		}
	}
	return ""
}

func numPrefixLen(s string) int {
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == ',' {
			i++
			continue
		}
		break
	}
	return i
}

func isUnitOnly(s string) bool {
	if s == "" || len(s) > 3 {
		return false
	}
	for _, c := range s {
		switch c {
		case 'k', 'K', 'm', 'M', 'g', 'G', 't', 'T', 'p', 'P', 'i', 'I', 'b', 'B':
		default:
			return false
		}
	}
	return strings.ContainsAny(s, "bB")
}

func statFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
