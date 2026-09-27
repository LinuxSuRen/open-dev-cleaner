package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if err != nil {
		note := "ollama list 执行失败:" + firstLine(err.Error())
		size, files := DirSize(ctx, modelsDir)
		emit(&Target{
			ID: toolUnavailable(tool), Tool: tool, ToolTitle: title,
			Category: "大模型", Title: "Ollama 模型目录",
			Path: modelsDir, Description: note, Note: note,
			Risk: RiskHigh, Size: size, Count: files, Available: false,
		})
		return
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
			Risk: RiskHigh, Size: m.size,
			Method: MethodCommand, Cmd: []string{"ollama", "rm", m.name},
			Available: true,
		})
	}

	emit(&Target{
		ID: tool + ":models", Tool: tool, ToolTitle: title,
		Category: "大模型", Title: fmt.Sprintf("Ollama 本地模型 (%d 个)", len(models)),
		Path: modelsDir,
		Description: "本地大模型权重,删除单个模型后可随时 ollama pull 重新拉取(需要网络与时间)",
		Risk: RiskHigh, Size: total,
		Method: MethodGroup, Available: true,
		Items: children,
	})
}

// parseOllamaList parses output like:
//
//	NAME                ID          SIZE      MODIFIED
//	deepseek-r1:14b     xxxxxxxx    9.0GB     3 days ago
func parseOllamaList(out string) []ollamaModel {
	var models []ollamaModel
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] == "NAME" {
			continue
		}
		name := fields[0]
		sizeIdx := -1
		for i := 1; i < len(fields); i++ {
			f := fields[i]
			if len(f) > 1 && (f[len(f)-1] == 'B' || f[len(f)-1] == 'b') && f[0] >= '0' && f[0] <= '9' {
				sizeIdx = i
				break
			}
		}
		if sizeIdx < 0 {
			continue
		}
		models = append(models, ollamaModel{name: name, size: ParseSize(fields[sizeIdx])})
	}
	return models
}

func statFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
