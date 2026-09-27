package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Env carries all filesystem locations the providers need. It exists as a
// struct so tests (and unusual setups) can override locations without
// touching the real home directory.
type Env struct {
	GOOS       string   `json:"goos"`
	Home       string   `json:"home"`
	CacheDir   string   `json:"cacheDir"`   // per-OS user cache dir
	AppData    string   `json:"appData"`    // app support / local share
	Workspaces []string `json:"workspaces"` // project roots to scan
	// NoExternal disables invoking external CLIs (docker, brew, ollama,
	// pnpm...). Used by tests and by the --no-external flag.
	NoExternal bool `json:"noExternal"`
}

// DetectEnv collects the standard locations of the current machine.
// Environment overrides (useful for testing / CI):
//
//	ODC_HOME   replace the home directory
//	ODC_CACHE  replace the user cache directory
//	ODC_APPDATA replace the app data directory
//	ODC_WORKSPACE add extra workspace roots, separated by ":"
func DetectEnv() *Env {
	home := firstNonEmpty(os.Getenv("ODC_HOME"), userHome())
	cache := firstNonEmpty(os.Getenv("ODC_CACHE"), userCache())
	appData := firstNonEmpty(os.Getenv("ODC_APPDATA"), appDataDir(home, cache))

	e := &Env{
		GOOS:     runtime.GOOS,
		Home:     home,
		CacheDir: cache,
		AppData:  appData,
	}
	e.Workspaces = detectWorkspaces(home)
	if extra := os.Getenv("ODC_WORKSPACE"); extra != "" {
		for _, p := range strings.Split(extra, string(os.PathListSeparator)) {
			if p != "" && isDir(p) {
				e.Workspaces = append(e.Workspaces, p)
			}
		}
		e.Workspaces = dedupeRoots(e.Workspaces)
	}
	return e
}

func userHome() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return workingDirFallback()
	}
	return h
}

func userCache() string {
	c, err := os.UserCacheDir()
	if err != nil || c == "" {
		h := userHome()
		if h == "" {
			return ""
		}
		return filepath.Join(h, ".cache")
	}
	return c
}

func appDataDir(home, cache string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	case "windows":
		// os.UserCacheDir() is %LocalAppData% on Windows which is the
		// closest equivalent for per-app data used here.
		if cache != "" {
			return cache
		}
		return filepath.Join(home, "AppData", "Local")
	default:
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return x
		}
		return filepath.Join(home, ".local", "share")
	}
}

func workingDirFallback() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// detectWorkspaces returns existing candidate project roots, most likely
// first. These are the folders scanned for node_modules / target / dist...
// Case-insensitive filesystems (macOS/Windows) can expose the same folder
// under several spellings, and roots nested in other roots would be
// scanned twice — both are de-duplicated here.
func detectWorkspaces(home string) []string {
	candidates := []string{
		"Workspace",
		"workspace",
		"Workspaces",
		"projects",
		"Projects",
		"PycharmProjects",
		"IdeaProjects",
		"go/src",
		"dev",
		"code",
		"src",
		"work",
	}
	var out []string
	for _, c := range candidates {
		p := filepath.Join(home, c)
		if isDir(p) {
			out = append(out, p)
		}
	}
	return dedupeRoots(out)
}

// dedupeRoots removes duplicated / nested roots. On case-insensitive
// systems (darwin, windows) two spellings may point at one directory.
func dedupeRoots(roots []string) []string {
	caseFold := runtime.GOOS == "darwin" || runtime.GOOS == "windows"
	norm := func(p string) string {
		p = filepath.Clean(p)
		if caseFold {
			p = strings.ToLower(p)
		}
		return p
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range roots {
		n := norm(r)
		if seen[n] {
			continue
		}
		dup := false
		for _, prev := range out {
			pn := norm(prev)
			if pn == n || strings.HasPrefix(n+string(filepath.Separator), pn+string(filepath.Separator)) {
				dup = true // r is inside (or equals) an already kept root
				break
			}
		}
		if !dup {
			out = append(out, r)
			seen[n] = true
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// IsDir reports whether path is an existing directory.
func IsDir(path string) bool { return isDir(path) }

func isDir(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// Exists reports whether path exists (file or directory).
func Exists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
