package scan

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// GoProvider scans Go build/module caches.
type GoProvider struct{}

func (p *GoProvider) Key() string    { return "go" }
func (p *GoProvider) Title() string  { return "Go" }

func (p *GoProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool := p.Key()
	title := p.Title()

	// GOCACHE: ~/Library/Caches/go-build | ~/.cache/go-build | %LocalAppData%\go-build
	cachePath := filepath.Join(env.CacheDir, "go-build")
	if size, files := DirSize(ctx, cachePath); size > 0 || Exists(cachePath) {
		emit(&Target{
			ID: tool + ":build-cache", Tool: tool, ToolTitle: title,
			Category: "构建缓存", Title: "Go 构建缓存 (GOCACHE)",
			Path: cachePath, AllowedRoot: env.CacheDir,
			Description: "go build 的中间编译产物,删除后首次编译会稍慢,无需联网",
			Risk: RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// GOMODCACHE: prefer `go env` (respects GOPATH overrides), fallback default.
	modCache := filepath.Join(env.Home, "go", "pkg", "mod")
	if out, err := runCmd(ctx, env, 5*time.Second, "go", "env", "GOMODCACHE"); err == nil {
		if v := strings.TrimSpace(out); v != "" {
			modCache = v
		}
	}
	if size, files := DirSize(ctx, modCache); size > 0 || Exists(modCache) {
		emit(&Target{
			ID: tool + ":mod-cache", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "Go 模块缓存 (GOMODCACHE)",
			Path: modCache, AllowedRoot: parentOf(modCache),
			Description: "已下载的依赖源码,删除后构建需要重新联网下载 (等效 go clean -modcache)",
			Risk: RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// sumdb cache is tiny and lives next to mod; skip.

	// Editor helper caches on macOS/Linux: goimports / gopls
	for _, name := range []string{"goimports", "gopls"} {
		p := filepath.Join(env.CacheDir, name)
		if size, files := DirSize(ctx, p); size > 0 {
			emit(&Target{
				ID: tool + ":" + name, Tool: tool, ToolTitle: title,
				Category: "索引缓存", Title: name + " 索引缓存",
				Path: p, AllowedRoot: env.CacheDir,
				Description: "代码补全/导入工具的索引缓存,删除后自动重建",
				Risk: RiskSafe, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}
}

func parentOf(p string) string {
	d := filepath.Dir(p)
	if d == "" || d == "." {
		return p
	}
	return d
}
