package scan

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// BrowserProvider scans browser caches. Browser caches are not dev-only,
// but developers usually run heavyweight Electron/Chromium apps whose
// caches grow fast. Profiles, passwords and history are never touched.
type BrowserProvider struct{}

func (p *BrowserProvider) Key() string   { return "browser" }
func (p *BrowserProvider) Title() string { return "浏览器" }

type browserTarget struct {
	name string
	path string
}

func (p *BrowserProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	var targets []browserTarget
	switch env.GOOS {
	case "darwin":
		targets = []browserTarget{
			{"Google Chrome", filepath.Join(env.CacheDir, "Google", "Chrome")},
			{"Microsoft Edge", filepath.Join(env.CacheDir, "Microsoft Edge")},
			{"Firefox", filepath.Join(env.CacheDir, "Firefox")},
			{"Chromium", filepath.Join(env.CacheDir, "Chromium")},
			{"Brave", filepath.Join(env.CacheDir, "BraveSoftware", "Brave-Browser")},
		}
	case "windows":
		// Only the cache sub-folders of the default profile.
		profiles := filepath.Join(env.CacheDir, "Google", "Chrome", "User Data")
		targets = []browserTarget{
			{"Google Chrome 缓存", filepath.Join(profiles, "Default", "Cache")},
			{"Google Chrome GPU 缓存", filepath.Join(profiles, "GrShaderCache")},
			{"Microsoft Edge 缓存", filepath.Join(env.CacheDir, "Microsoft", "Edge", "User Data", "Default", "Cache")},
		}
		// Firefox per-profile cache2
		ffRoot := filepath.Join(env.Home, "AppData", "Local", "Mozilla", "Firefox", "Profiles")
		if matches, _ := filepath.Glob(filepath.Join(ffRoot, "*", "cache2")); len(matches) > 0 {
			targets = append(targets, browserTarget{"Firefox 缓存", matches[0]})
		}
	default:
		targets = []browserTarget{
			{"Google Chrome", filepath.Join(env.CacheDir, "google-chrome")},
			{"Chromium", filepath.Join(env.CacheDir, "chromium")},
			{"Firefox", filepath.Join(env.CacheDir, "mozilla", "firefox")},
			{"Microsoft Edge", filepath.Join(env.CacheDir, "microsoft-edge")},
			{"Brave", filepath.Join(env.CacheDir, "BraveSoftware")},
		}
	}

	for _, bt := range targets {
		if !Exists(bt.path) {
			continue
		}
		size, files := DirSize(ctx, bt.path)
		if size == 0 {
			continue
		}
		emit(&Target{
			ID: tool + ":" + sanitizeID(bt.name), Tool: tool, ToolTitle: title,
			Category: "浏览器缓存", Title: bt.name + " 缓存",
			Path: bt.path, AllowedRoot: env.CacheDir,
			Description: "网页资源缓存,请先关闭对应浏览器再清理;书签/密码/历史记录不受影响",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
}

func sanitizeID(s string) string {
	r := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	return strings.ToLower(r.ReplaceAllString(s, "-"))
}

// OSProvider covers OS-level items: Homebrew, Xcode and temp directories.
type OSProvider struct{}

func (p *OSProvider) Key() string   { return "os" }
func (p *OSProvider) Title() string { return "系统" }

func (p *OSProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	// --- Homebrew (macOS & Linuxbrew) ---
	if _, err := runCmd(ctx, env, 5*time.Second, "brew", "--version"); err == nil {
		cacheDir := filepath.Join(env.CacheDir, "Homebrew")
		if size, files := DirSize(ctx, cacheDir); size > 0 {
			emit(&Target{
				ID: tool + ":brew-cache", Tool: tool, ToolTitle: title,
				Category: "包缓存", Title: "Homebrew 下载缓存",
				Path: cacheDir, AllowedRoot: env.CacheDir,
				Description: "brew 下载的 bottle 压缩包 (等效 brew cleanup --cache)",
				Risk:        RiskSafe, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
		if est, detail := estimateBrewCleanup(ctx, env); est >= 0 {
			t := &Target{
				ID: tool + ":brew-cleanup", Tool: tool, ToolTitle: title,
				Category: "旧版本", Title: "Homebrew 旧版本清理",
				Description: "已安装软件的旧版本与死链 (等效 brew cleanup --prune=all)",
				Risk:        RiskCaution, Size: est,
				Method: MethodCommand, Cmd: []string{"brew", "cleanup", "--prune=all"},
				Available: true,
			}
			if est == 0 {
				t.Note = detail
			}
			emit(t)
		}
	}

	// --- Xcode (macOS only) ---
	if env.GOOS == "darwin" {
		xcode := []struct {
			id, name, path string
			risk           RiskLevel
			desc           string
		}{
			{"xcode-derived", "Xcode DerivedData",
				filepath.Join(env.Home, "Library", "Developer", "Xcode", "DerivedData"), RiskCaution,
				"Xcode 构建的中间产物与索引,删除后下次构建重新生成"},
			{"xcode-devsupport", "Xcode 设备支持文件",
				filepath.Join(env.Home, "Library", "Developer", "Xcode", "iOS DeviceSupport"), RiskCaution,
				"旧版本设备的符号文件,删除后连接设备时可能重新生成"},
			{"xcode-simcache", "模拟器缓存",
				filepath.Join(env.Home, "Library", "Developer", "CoreSimulator", "Caches"), RiskSafe,
				"模拟器临时缓存,可安全删除"},
		}
		for _, x := range xcode {
			if size, files := DirSize(ctx, x.path); size > 0 {
				emit(&Target{
					ID: tool + ":" + x.id, Tool: tool, ToolTitle: title,
					Category: "构建缓存", Title: x.name,
					Path: x.path, AllowedRoot: filepath.Dir(x.path),
					Description: x.desc, Risk: x.risk, Size: size, Count: files,
					Method: MethodDir, Available: true,
				})
			}
		}
	}

	// --- Temp directories (all OS) ---
	tmp := os.TempDir()
	if size, files := DirSize(ctx, tmp); size > 0 {
		emit(&Target{
			ID: tool + ":tmp", Tool: tool, ToolTitle: title,
			Category: "临时文件", Title: "系统临时目录 (" + tmp + ")",
			Path: tmp, AllowedRoot: filepath.Dir(tmp),
			Description: "临时文件,保留目录本身仅清空内容;被占用的文件会自动跳过",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDirContents, Available: true,
		})
	}
}

// estimateBrewCleanup runs `brew cleanup --dry-run` and sums the printed
// sizes. Returns -1 when the command is unavailable.
func estimateBrewCleanup(ctx context.Context, env *Env) (int64, string) {
	out, err := runCmd(ctx, env, 40*time.Second, "brew", "cleanup", "--dry-run")
	if err != nil {
		return -1, ""
	}
	var total int64
	re := regexp.MustCompile(`\(([\d.,]+\s*[KMGT]?i?B)`)
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		total += ParseSize(m[1])
	}
	return total, ""
}
