package scan

import (
	"context"
	"path/filepath"
)

// DotNetProvider scans the .NET ecosystem: NuGet global packages, HTTP
// download cache, and Visual Studio component caches (Windows).
type DotNetProvider struct{}

func (p *DotNetProvider) Key() string   { return "dotnet" }
func (p *DotNetProvider) Title() string { return ".NET" }

func (p *DotNetProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	// NuGet global packages folder: ~/.nuget/packages (all platforms).
	pkgs := filepath.Join(env.Home, ".nuget", "packages")
	if size, files := DirSize(ctx, pkgs); size > 0 {
		emit(&Target{
			ID: tool + ":global-packages", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "NuGet 全局包 (~/.nuget/packages)",
			Path: pkgs, AllowedRoot: filepath.Dir(pkgs),
			Description: "所有 .NET 项目还原的包,删除后构建时自动重新还原 (等效 dotnet nuget locals global-packages --clear)",
			Risk: RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// NuGet HTTP download cache:
	//   mac: ~/Library/Caches/NuGet   linux: ~/.local/share/NuGet/http-cache
	//   win: %LocalAppData%\NuGet\http-cache
	var httpCache string
	switch env.GOOS {
	case "darwin":
		httpCache = filepath.Join(env.CacheDir, "NuGet")
	case "windows":
		httpCache = filepath.Join(env.CacheDir, "NuGet", "http-cache")
	default:
		httpCache = filepath.Join(env.AppData, "NuGet", "http-cache")
	}
	if size, files := DirSize(ctx, httpCache); size > 0 {
		emit(&Target{
			ID: tool + ":http-cache", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "NuGet HTTP 下载缓存",
			Path: httpCache, AllowedRoot: filepath.Dir(httpCache),
			Description: "NuGet 直接下载缓存,删除后需要时重新下载 (等效 dotnet nuget locals http-cache --clear)",
			Risk: RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// Visual Studio component caches (Windows only):
	// %LocalAppData%\Microsoft\VisualStudio\<ver>\ComponentModelCache
	if env.GOOS == "windows" {
		for _, p := range globDirs(filepath.Join(env.CacheDir, "Microsoft", "VisualStudio", "*", "ComponentModelCache")) {
			size, files := DirSize(ctx, p)
			if size == 0 {
				continue
			}
			emit(&Target{
				ID: tool + ":vs-component-cache:" + filepath.Base(filepath.Dir(p)),
				Tool: tool, ToolTitle: title,
				Category: "IDE 缓存", Title: "Visual Studio 组件缓存 (" + filepath.Base(filepath.Dir(p)) + ")",
				Path: p, AllowedRoot: filepath.Dir(filepath.Dir(p)),
				Description: "VS 组件模型缓存,删除后下次启动重建",
				Risk: RiskCaution, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}
}
