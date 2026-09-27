package scan

import (
	"context"
	"path/filepath"
)

// AppCacheProvider scans well-known development related caches inside the
// OS user cache directory (electron binaries, playwright browsers...).
type AppCacheProvider struct{}

func (p *AppCacheProvider) Key() string   { return "apps" }
func (p *AppCacheProvider) Title() string { return "开发应用缓存" }

type cacheEntry struct {
	rel  string // relative to env.CacheDir (slash separated)
	name string
	cat  string
	desc string
	risk RiskLevel
}

func (p *AppCacheProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	entries := []cacheEntry{
		{"electron", "Electron 二进制下载缓存", "包缓存",
			"electron 包管理器下载的 Electron 发行版压缩包,删除后按需重新下载", RiskSafe},
		{"node-gyp", "node-gyp 头文件缓存", "头文件缓存",
			"原生模块编译用的 Node 头文件,删除后下次编译自动重新下载", RiskSafe},
		{"node-sass", "node-sass 二进制缓存", "二进制缓存",
			"node-sass 预编译二进制,删除后按需重新下载", RiskSafe},
		{"ms-playwright", "Playwright 浏览器", "浏览器二进制",
			"Playwright 下载的浏览器与视频编解码器,删除后执行 npx playwright install 重装", RiskCaution},
		{"puppeteer", "Puppeteer Chromium", "浏览器二进制",
			"Puppeteer 下载的 Chromium,删除后安装依赖时自动重下", RiskCaution},
		{"hugo_cache", "Hugo 模块缓存", "包缓存",
			"Hugo modules 下载缓存,删除后构建时重新拉取", RiskSafe},
		{"prettier", "Prettier 插件缓存", "包缓存",
			"Prettier 插件与解析器缓存,删除后自动重建", RiskSafe},
		{"Java", "Java 部署缓存", "运行时缓存",
			"Java 应用缓存与日志", RiskSafe},
		{"D3DSCache", "DirectX 着色器缓存 (Windows)", "着色器缓存",
			"Direct3D 着色器缓存,删除后游戏/图形应用首次启动稍慢", RiskSafe},
		{"typescript", "TypeScript 版本缓存", "包缓存",
			"tsserver/自动类型获取下载的 TypeScript 与 @types 包", RiskSafe},
		{"ts-node", "ts-node 缓存", "编译缓存",
			"ts-node 编译缓存,删除后自动重建", RiskSafe},
		{"Cypress", "Cypress 二进制缓存", "二进制缓存",
			"Cypress 桌面应用下载缓存,删除后执行 cypress install 重装", RiskCaution},
	}

	for _, e := range entries {
		path := filepath.Join(env.CacheDir, filepath.FromSlash(e.rel))
		if !Exists(path) {
			continue
		}
		size, files := DirSize(ctx, path)
		if size == 0 {
			continue
		}
		emit(&Target{
			ID: tool + ":" + e.rel, Tool: tool, ToolTitle: title,
			Category: e.cat, Title: e.name,
			Path: path, AllowedRoot: env.CacheDir,
			Description: e.desc, Risk: e.risk, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
}
