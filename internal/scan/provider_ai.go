package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AIProvider scans AI coding assistants and agent CLIs: DeepSeek Harness
// (dsh), opencode, Claude Code, Codex, Gemini CLI, aider...
// Each tool offers fine-grained cache cleaning plus a "delete all local
// data" entry for rarely used tools.
type AIProvider struct{}

func (p *AIProvider) Key() string   { return "ai" }
func (p *AIProvider) Title() string { return "AI 编程工具" }

// aiDir describes one cleanable sub directory of an AI tool.
type aiDir struct {
	rel  string // slash separated, relative to the tool root
	name string
	cat  string
	desc string
	risk RiskLevel
}

type aiTool struct {
	key       string
	name      string
	root      string // tool home (data+config)
	rootName  string // display name of root
	hasAll    bool   // offer "delete all local data"
	dirs      []aiDir
	extra     []aiDir       // additional dirs outside root
	extraRoot func() string // base dir for extra entries
}

func (p *AIProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	tools := []aiTool{
		{
			key: "dsh", name: "DeepSeek Harness (dsh)",
			root: filepath.Join(env.Home, ".dsh"), rootName: "~/.dsh", hasAll: true,
			dirs: []aiDir{
				{"sessions", "会话历史 (sessions)", "会话",
					"历史会话记录,删除后不可恢复(不影响配置)", RiskCaution},
				{"storages", "存储 (storages)", "存储",
					"运行时存储数据,删除后按需重建", RiskCaution},
				{"attachments", "附件 (attachments)", "附件",
					"会话上传的附件缓存", RiskCaution},
				{"tools", "工具集成 (tools)", "工具",
					"已集成的外部工具数据,删除后重新初始化", RiskCaution},
				{"profiles", "Agent 预设 (profiles)", "配置",
					"agent preset,删除后需重新配置", RiskCaution},
			},
		},
		{
			key: "opencode", name: "OpenCode",
			root: filepath.Join(env.Home, ".local", "share", "opencode"),
			rootName: "~/.local/share/opencode", hasAll: true,
			dirs: []aiDir{
				{"log", "运行日志 (log)", "日志", "运行日志,可安全删除", RiskSafe},
				{"snapshot", "代码快照 (snapshot)", "快照",
					"会话中的代码快照备份,删除后丢失历史快照", RiskCaution},
				{"bin", "运行时二进制 (bin)", "二进制",
					"自动下载的 rg/node_modules 等运行时,删除后重新下载", RiskCaution},
				{"storage", "本地存储 (storage)", "存储", "本地存储数据", RiskCaution},
			},
			extra: []aiDir{
				{"node_modules", "全局插件依赖 (config/node_modules)", "依赖",
					"~/.config/opencode/node_modules:全局安装的插件与依赖,删除后 opencode 重新安装",
					RiskCaution},
			},
			extraRoot: func() string { return filepath.Join(env.Home, ".config", "opencode") },
		},
		{
			key: "claude", name: "Claude Code",
			root: filepath.Join(env.Home, ".claude"), rootName: "~/.claude", hasAll: true,
			dirs: []aiDir{
				{"transcripts", "会话记录 (transcripts)", "会话",
					"会话完整记录,删除后不可恢复(登录状态不受影响)", RiskCaution},
				{"projects", "项目会话 (projects)", "会话",
					"各项目的会话与检查点数据", RiskCaution},
				{"debug", "调试日志 (debug)", "日志", "调试日志,可安全删除", RiskSafe},
				{"file-history", "文件修改历史 (file-history)", "历史",
					"会话中的文件修改历史", RiskCaution},
				{"shell-snapshots", "Shell 快照 (shell-snapshots)", "缓存",
					"启动加速用的 shell 快照,自动重建", RiskSafe},
				{"todos", "任务清单 (todos)", "数据", "会话 TODO 数据", RiskSafe},
				{"cache", "缓存 (cache)", "缓存", "运行缓存,自动重建", RiskSafe},
				{"plugins", "插件 (plugins)", "插件", "已安装插件,删除后需重新安装", RiskCaution},
			},
		},
		{
			key: "codex", name: "Codex CLI",
			root: filepath.Join(env.Home, ".codex"), rootName: "~/.codex", hasAll: true,
			dirs: []aiDir{
				{"log", "运行日志 (log)", "日志", "运行日志,可安全删除", RiskSafe},
				{"sessions", "会话历史 (sessions)", "会话",
					"会话记录,删除后不可恢复", RiskCaution},
				{"history.jsonl", "命令历史 (history.jsonl)", "历史",
					"提示词历史", RiskCaution},
			},
		},
		{
			key: "gemini", name: "Gemini CLI",
			root: filepath.Join(env.Home, ".gemini"), rootName: "~/.gemini", hasAll: true,
			dirs: []aiDir{
				{"tmp", "临时文件 (tmp)", "缓存", "临时缓存,可安全删除", RiskSafe},
			},
		},
		{
			key: "aider", name: "Aider",
			root: filepath.Join(env.Home, ".aider"), rootName: "~/.aider", hasAll: true,
			dirs: []aiDir{
				{"caches", "模型缓存 (caches)", "缓存", "模型映射与缓存数据", RiskCaution},
				{"linters", "Linter 缓存 (linters)", "缓存", "linter 安装缓存", RiskCaution},
			},
		},
	}

	for _, at := range tools {
		if !Exists(at.root) {
			continue
		}
		group := &Target{
			ID: tool + ":" + at.key, Tool: tool, ToolTitle: title,
			Category: "AI 工具", Title: at.name,
			Path: at.root, AllowedRoot: filepath.Dir(at.root),
			Risk: RiskCaution, Method: MethodGroup, Available: true,
		}

		appendDir := func(base string, d aiDir) {
			path := filepath.Join(base, filepath.FromSlash(d.rel))
			fi, err := os.Stat(path)
			if err != nil {
				return
			}
			var size, files int64
			if fi.IsDir() {
				size, files = DirSize(ctx, path)
			} else {
				size, files = fi.Size(), 1
			}
			if size == 0 {
				return
			}
			group.Items = append(group.Items, &Target{
				ID: tool + ":" + at.key + ":" + d.rel, Tool: tool, ToolTitle: title,
				Category: d.cat, Title: d.name,
				Path: path, AllowedRoot: base,
				Description: d.desc, Risk: d.risk, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}

		for _, d := range at.dirs {
			appendDir(at.root, d)
		}
		for _, d := range at.extra {
			appendDir(at.extraRoot(), d)
		}

		// Whole-data deletion for rarely used tools.
		if at.hasAll {
			size, files := DirSize(ctx, at.root)
			if size > 0 {
				group.Items = append(group.Items, &Target{
					ID: tool + ":" + at.key + ":all", Tool: tool, ToolTitle: title,
					Category: "工具数据", Title: "⚠ 删除 " + at.name + " 全部本地数据 (" + at.rootName + ")",
					Path: at.root, AllowedRoot: filepath.Dir(at.root),
					Description: "删除该工具的全部本地数据(含配置与登录状态),适合不再使用该工具时彻底清理",
					Risk: RiskHigh, Size: size, Count: files,
					Method: MethodDir, Available: true,
					Note: usageHint(at.root, 180),
				})
			}
		}

		if len(group.Items) > 0 {
			sort.SliceStable(group.Items, func(i, j int) bool { return group.Items[i].Size > group.Items[j].Size })
			group.Description = at.name + " 的缓存、会话与工具数据;最近活动: " + lastActivityDesc(at.root)
			group.Risk = RiskCaution
			emit(group)
		}
	}
}

// lastActivity returns the newest mtime among the directory itself and
// its direct children (cheap shallow scan, good enough as a usage signal).
// Epoch-like timestamps (archive extraction artifacts) count as unknown.
func lastActivity(dir string) time.Time {
	fi, err := os.Stat(dir)
	if err != nil {
		return time.Time{}
	}
	newest := fi.ModTime()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return normalizeTime(newest)
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return normalizeTime(newest)
}

// normalizeTime maps pre-2000 timestamps (unpacked archives keep epoch
// mtimes) to the zero time so they render as 未知 instead of absurd ages.
func normalizeTime(t time.Time) time.Time {
	if t.Year() < 2000 {
		return time.Time{}
	}
	return t
}

func lastActivityDesc(dir string) string {
	t := lastActivity(dir)
	if t.IsZero() {
		return "未知"
	}
	days := int(time.Since(t).Hours() / 24)
	return fmt.Sprintf("%s(约 %d 天前)", t.Format("2006-01-02"), days)
}

// usageHint returns a warning when the directory has not been touched for
// the given number of days. Zero/epoch timestamps count as "no recorded
// activity" and get a hint as well.
func usageHint(dir string, staleDays int) string {
	t := lastActivity(dir)
	if t.IsZero() {
		return "无活动时间记录(可能解压后从未使用)"
	}
	days := int(time.Since(t).Hours() / 24)
	if days >= staleDays {
		return fmt.Sprintf("已 %d 天无活动,似乎不再使用", days)
	}
	return ""
}
