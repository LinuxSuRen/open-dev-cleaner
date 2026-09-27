package scan

import (
	"context"
	"path/filepath"
	"sort"
)

// EditorProvider scans IDE/editor caches: VS Code, Cursor, Trae, JetBrains
// and the remote vscode-server.
type EditorProvider struct{}

func (p *EditorProvider) Key() string   { return "editor" }
func (p *EditorProvider) Title() string { return "编辑器" }

type editorApp struct {
	key  string
	name string
	base string // app support dir
	ext  string // extensions dir (optional)
}

func (p *EditorProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	apps := []editorApp{
		{key: "vscode", name: "VS Code", base: filepath.Join(env.AppData, "Code"),
			ext: filepath.Join(env.Home, ".vscode", "extensions")},
		{key: "cursor", name: "Cursor", base: filepath.Join(env.AppData, "Cursor"),
			ext: filepath.Join(env.Home, ".cursor", "extensions")},
		{key: "trae", name: "Trae", base: filepath.Join(env.AppData, "Trae")},
		{key: "vscodium", name: "VSCodium", base: filepath.Join(env.AppData, "VSCodium")},
	}

	// Cache sub-directories inside an editor profile dir; all safe to drop.
	cacheSubs := []struct{ sub, name string }{
		{"Cache", "缓存 (Cache)"},
		{"CachedData", "脚本缓存 (CachedData)"},
		{"CachedExtensionVSIXs", "扩展安装包缓存 (VSIX)"},
		{"Code Cache", "网页缓存 (Code Cache)"},
		{"GPUCache", "GPU 缓存"},
		{"ShaderCache", "着色器缓存"},
		{"DawnCache", "Dawn 缓存"},
		{"GrShaderCache", "GrShader 缓存"},
		{"Service Worker/CacheStorage", "Service Worker 缓存"},
		{"Service Worker/ScriptCache", "Service Worker 脚本缓存"},
		{"logs", "运行日志"},
	}

	for _, app := range apps {
		if !Exists(app.base) && app.ext == "" {
			continue
		}
		group := &Target{
			ID: tool + ":" + app.key, Tool: tool, ToolTitle: title,
			Category: "IDE 缓存", Title: app.name,
			Path:        app.base,
			Description: app.name + " 的缓存/索引/日志(设置、登录状态与工作区数据不受影响)",
			Risk:        RiskSafe, Method: MethodGroup, Available: true,
		}
		any := false

		for _, cs := range cacheSubs {
			path := filepath.Join(app.base, filepath.FromSlash(cs.sub))
			if !Exists(path) {
				continue
			}
			size, files := DirSize(ctx, path)
			if size == 0 {
				continue
			}
			group.Items = append(group.Items, &Target{
				ID: tool + ":" + app.key + ":" + cs.name, Tool: tool, ToolTitle: title,
				Category: "IDE 缓存", Title: app.name + " " + cs.name,
				Path: path, AllowedRoot: app.base,
				Description: cs.name + ",删除后自动重建", Risk: RiskSafe,
				Size: size, Count: files, Method: MethodDir, Available: true,
			})
			any = true
		}

		if app.ext != "" && Exists(app.ext) {
			size, files := DirSize(ctx, app.ext)
			if size > 0 {
				group.Items = append(group.Items, &Target{
					ID: tool + ":" + app.key + ":ext", Tool: tool, ToolTitle: title,
					Category: "扩展", Title: app.name + " 扩展目录",
					Path: app.ext, AllowedRoot: filepath.Dir(app.ext),
					Description: "已安装的编辑器扩展,删除后需重新安装(配置与快捷键保留)",
					Risk:        RiskCaution, Size: size, Count: files,
					Method: MethodDir, Available: true,
				})
				group.Risk = RiskCaution
				any = true
			}
		}

		if any {
			sort.SliceStable(group.Items, func(i, j int) bool { return group.Items[i].Size > group.Items[j].Size })
			emit(group)
		}
	}

	// JetBrains IDE caches, broken down per product so frequently used
	// IDEs can be kept while stale ones are cleaned:
	//   mac:   ~/Library/Caches/JetBrains/<Product><ver>
	//   linux: ~/.cache/JetBrains/<Product><ver>
	//   win:   %LocalAppData%\JetBrains\<Product><ver>
	jetbrainsRoot := filepath.Join(env.CacheDir, "JetBrains")
	if products := listDirs(jetbrainsRoot); len(products) > 0 {
		group := &Target{
			ID: tool + ":jetbrains", Tool: tool, ToolTitle: title,
			Category: "IDE 缓存", Title: "JetBrains 系 IDE 缓存(按 IDE 细分)",
			Path: jetbrainsRoot, AllowedRoot: env.CacheDir,
			Description: "IntelliJ IDEA / PyCharm / GoLand / WebStorm / Rider / Fleet 等各 IDE 的索引与缓存,删除后重新打开项目时重建索引(耗时);长期未用的 IDE 会单独标注",
			Risk:        RiskCaution, Method: MethodGroup, Available: true,
		}
		for _, p := range products {
			size, files := DirSize(ctx, p)
			if size == 0 {
				continue
			}
			note := ""
			if h := usageHint(p, 180); h != "" {
				note = "⚠ " + h
			}
			group.Items = append(group.Items, &Target{
				ID: tool + ":jetbrains:" + filepath.Base(p), Tool: tool, ToolTitle: title,
				Category: "IDE 缓存", Title: filepath.Base(p),
				Path: p, AllowedRoot: jetbrainsRoot,
				Description: "该 IDE 的索引、缓存与本地历史,删除后重建",
				Risk:        RiskCaution, Size: size, Count: files,
				Method: MethodDir, Available: true, Note: note,
			})
		}
		if len(group.Items) > 0 {
			sort.SliceStable(group.Items, func(i, j int) bool { return group.Items[i].Size > group.Items[j].Size })
			emit(group)
		}
	}

	// JetBrains logs live next to the caches only on macOS.
	if env.GOOS == "darwin" {
		jbLogs := filepath.Join(env.Home, "Library", "Logs", "JetBrains")
		if size, files := DirSize(ctx, jbLogs); size > 0 {
			emit(&Target{
				ID: tool + ":jetbrains-logs", Tool: tool, ToolTitle: title,
				Category: "日志", Title: "JetBrains 系 IDE 日志",
				Path: jbLogs, AllowedRoot: filepath.Dir(jbLogs),
				Description: "各 IDE 的运行日志(idea.log 等),可安全删除",
				Risk:        RiskSafe, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}

	// Android Studio is JetBrains based but keeps caches under Google/.
	//   mac/linux: <cache>/Google/AndroidStudio*   win: %LocalAppData%\Google\AndroidStudio*
	for _, p := range globDirs(filepath.Join(env.CacheDir, "Google", "AndroidStudio*")) {
		size, files := DirSize(ctx, p)
		if size == 0 {
			continue
		}
		note := ""
		if h := usageHint(p, 180); h != "" {
			note = "⚠ " + h
		}
		emit(&Target{
			ID: tool + ":android-studio:" + filepath.Base(p), Tool: tool, ToolTitle: title,
			Category: "IDE 缓存", Title: "Android Studio 缓存 (" + filepath.Base(p) + ")",
			Path: p, AllowedRoot: filepath.Dir(p),
			Description: "Android Studio 的索引与构建缓存,删除后重新打开项目时重建",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true, Note: note,
		})
	}

	// Android Gradle build cache (~/.android/build-cache).
	if size, files := DirSize(ctx, filepath.Join(env.Home, ".android", "build-cache")); size > 0 {
		emit(&Target{
			ID: tool + ":android-build-cache", Tool: tool, ToolTitle: title,
			Category: "构建缓存", Title: "Android 构建缓存 (~/.android/build-cache)",
			Path:        filepath.Join(env.Home, ".android", "build-cache"),
			AllowedRoot: filepath.Join(env.Home, ".android"),
			Description: "Android Gradle 插件的构建缓存,删除后自动重建(AVD 虚拟设备不受影响)",
			Risk:        RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// Eclipse: p2 shared bundle pool + download cache + per-user data.
	p2Cache := filepath.Join(env.Home, ".p2", "org.eclipse.equinox.p2.core", "cache")
	if size, files := DirSize(ctx, p2Cache); size > 0 {
		emit(&Target{
			ID: tool + ":eclipse-p2-cache", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "Eclipse p2 下载缓存",
			Path: p2Cache, AllowedRoot: filepath.Dir(filepath.Dir(p2Cache)),
			Description: "p2 配置器下载缓存,删除后需要时重新下载",
			Risk:        RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
	p2Pool := filepath.Join(env.Home, ".p2")
	if size, files := DirSize(ctx, p2Pool); size > 0 {
		note := ""
		if h := usageHint(p2Pool, 180); h != "" {
			note = "⚠ " + h
		}
		emit(&Target{
			ID: tool + ":eclipse-p2-pool", Tool: tool, ToolTitle: title,
			Category: "工具数据", Title: "Eclipse p2 共享组件池 (~/.p2)",
			Path: p2Pool, AllowedRoot: env.Home,
			Description: "Eclipse 安装器共享的插件/组件池,删除后相关 Eclipse 安装需重新安装组件;不再使用 Eclipse 时可整体删除",
			Risk:        RiskHigh, Size: size, Count: files,
			Method: MethodDir, Available: true, Note: note,
		})
	}
	if size, files := DirSize(ctx, filepath.Join(env.Home, ".eclipse")); size > 0 {
		emit(&Target{
			ID: tool + ":eclipse-data", Tool: tool, ToolTitle: title,
			Category: "工具数据", Title: "Eclipse 用户数据 (~/.eclipse)",
			Path: filepath.Join(env.Home, ".eclipse"), AllowedRoot: env.Home,
			Description: "Eclipse 的用户级配置与缓存数据",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	// Remote development server artifacts.
	for _, d := range []string{
		filepath.Join(env.Home, ".vscode-server"),
		filepath.Join(env.Home, ".vscode-insiders-server"),
	} {
		if size, files := DirSize(ctx, d); size > 0 {
			emit(&Target{
				ID: tool + ":" + filepath.Base(d), Tool: tool, ToolTitle: title,
				Category: "远程开发", Title: filepath.Base(d) + " (远程开发)",
				Path: d, AllowedRoot: env.Home,
				Description: "SSH 远程开发时安装的服务端与扩展,删除后下次连接自动重装",
				Risk:        RiskCaution, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}
}
