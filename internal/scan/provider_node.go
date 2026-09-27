package scan

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// NodeProvider scans Node.js ecosystem caches: npm, pnpm, yarn.
type NodeProvider struct{}

func (p *NodeProvider) Key() string   { return "node" }
func (p *NodeProvider) Title() string { return "Node.js" }

func (p *NodeProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	// npm cache: ~/.npm (unix) | %LocalAppData%\npm-cache (win)
	npmdir := filepath.Join(env.Home, ".npm")
	if env.GOOS == "windows" {
		npmdir = filepath.Join(env.CacheDir, "npm-cache")
	}
	npmCache := filepath.Join(npmdir, "_cacache")
	if size, files := DirSize(ctx, npmCache); size > 0 {
		emit(&Target{
			ID: tool + ":npm-cache", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "npm 缓存 (_cacache)",
			Path: npmCache, AllowedRoot: npmdir,
			Description: "npm 下载缓存,删除后安装依赖需重新联网下载 (等效 npm cache clean --force)",
			Risk: RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// pnpm store: ask pnpm first, fallback to per-OS defaults.
	pnpmStore := detectPnpmStore(ctx, env)
	if size, files := DirSize(ctx, pnpmStore); size > 0 {
		emit(&Target{
			ID: tool + ":pnpm-store", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "pnpm 内容寻址存储 (store)",
			Path: pnpmStore, AllowedRoot: parentOf(pnpmStore),
			Description: "pnpm 全局包存储,删除后所有项目需重新安装依赖 (等效 pnpm store prune 的加强版)",
			Risk: RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// yarn cache: ~/Library/Caches/Yarn | ~/.cache/yarn | %LocalAppData%\Yarn
	yarn := filepath.Join(env.CacheDir, "Yarn")
	if env.GOOS == "linux" {
		yarn = filepath.Join(env.CacheDir, "yarn")
	}
	if size, files := DirSize(ctx, yarn); size > 0 {
		emit(&Target{
			ID: tool + ":yarn-cache", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "Yarn 缓存",
			Path: yarn, AllowedRoot: env.CacheDir,
			Description: "Yarn 包缓存,删除后安装需重新下载 (等效 yarn cache clean)",
			Risk: RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// node-gyp headers: ~/.node-gyp | ~/Library/Caches/node-gyp
	gyp := filepath.Join(env.Home, ".node-gyp")
	if !Exists(gyp) {
		gyp = filepath.Join(env.CacheDir, "node-gyp")
	}
	if size, files := DirSize(ctx, gyp); size > 0 {
		emit(&Target{
			ID: tool + ":node-gyp", Tool: tool, ToolTitle: title,
			Category: "头文件缓存", Title: "node-gyp 编译头文件",
			Path: gyp, AllowedRoot: parentOf(gyp),
			Description: "原生模块编译所需的 Node 头文件,删除后下次编译自动重新下载",
			Risk: RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// npm/pnpm logs are tiny; skip.
}

func detectPnpmStore(ctx context.Context, env *Env) string {
	if out, err := runCmd(ctx, env, 6*time.Second, "pnpm", "store", "path"); err == nil {
		if v := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0]); v != "" {
			return v
		}
	}
	switch env.GOOS {
	case "darwin":
		return filepath.Join(env.Home, "Library", "pnpm", "store")
	case "windows":
		return filepath.Join(env.CacheDir, "pnpm", "store")
	default:
		return filepath.Join(env.Home, ".local", "share", "pnpm", "store")
	}
}
