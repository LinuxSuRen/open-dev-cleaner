package scan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// VersionsProvider scans version managers (nvm, sdkman, pyenv, ~/sdk for
// Go) and lists every installed toolchain/SDK as an individually
// deletable target. Rarely used versions are flagged with a stale hint so
// they can be deleted to reclaim large amounts of space.
type VersionsProvider struct{}

func (p *VersionsProvider) Key() string   { return "versions" }
func (p *VersionsProvider) Title() string { return "版本管理器" }

const staleDays = 180

func (p *VersionsProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	// --- nvm: Node.js versions ---
	nvmVersions := filepath.Join(env.Home, ".nvm", "versions", "node")
	if isDir(nvmVersions) {
		current := readSmallFile(filepath.Join(env.Home, ".nvm", "alias", "default"))
		group := &Target{
			ID: tool + ":nvm", Tool: tool, ToolTitle: title,
			Category: "工具链", Title: "Node.js 版本 (nvm)",
			Path: nvmVersions, AllowedRoot: nvmVersions,
			Description: "nvm 安装的各个 Node.js 版本,可删除不再使用的版本;默认版本: " + current,
			Risk:        RiskHigh, Method: MethodGroup, Available: true,
		}
		for _, v := range listDirs(nvmVersions) {
			size, files := DirSize(ctx, v)
			if size == 0 {
				continue
			}
			isCur := versionMatches(current, filepath.Base(v))
			note := ""
			if isCur {
				note = "当前默认版本,删除前请先 nvm use 切换"
			} else if h := usageHint(v, staleDays); h != "" {
				note = "⚠ " + h
			}
			group.Items = append(group.Items, &Target{
				ID: tool + ":nvm:" + filepath.Base(v), Tool: tool, ToolTitle: title,
				Category: "工具链", Title: "Node.js " + filepath.Base(v),
				Path: v, AllowedRoot: nvmVersions,
				Description: "最后修改: " + lastActivityDesc(v),
				Risk:        RiskHigh, Size: size, Count: files,
				Method: MethodDir, Available: true, Note: note,
			})
		}
		emitIfAny(group, emit)

		// nvm download cache (bottled tarballs).
		nvmCache := filepath.Join(env.Home, ".nvm", ".cache")
		if size, files := DirSize(ctx, nvmCache); size > 0 {
			emit(&Target{
				ID: tool + ":nvm-cache", Tool: tool, ToolTitle: title,
				Category: "包缓存", Title: "nvm 下载缓存",
				Path: nvmCache, AllowedRoot: filepath.Join(env.Home, ".nvm"),
				Description: "已下载的 Node.js 发行版压缩包,删除后需要时重新下载",
				Risk:        RiskSafe, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}

	// --- sdkman: java/gradle/... candidates ---
	sdkman := filepath.Join(env.Home, ".sdkman")
	if isDir(sdkman) {
		cands := filepath.Join(sdkman, "candidates")
		group := &Target{
			ID: tool + ":sdkman", Tool: tool, ToolTitle: title,
			Category: "工具链", Title: "SDK 版本 (sdkman)",
			Path: cands, AllowedRoot: cands,
			Description: "sdkman 安装的各 SDK 版本(java、gradle 等),current 指向的版本已标注",
			Risk:        RiskHigh, Method: MethodGroup, Available: true,
		}
		if entries, err := os.ReadDir(cands); err == nil {
			for _, cand := range entries {
				if !cand.IsDir() {
					continue
				}
				current := resolveSymlink(filepath.Join(cands, cand.Name(), "current"))
				for _, v := range listDirs(filepath.Join(cands, cand.Name())) {
					base := filepath.Base(v)
					if base == "current" {
						continue
					}
					size, files := DirSize(ctx, v)
					if size == 0 {
						continue
					}
					note := ""
					if current != "" && filepath.Base(current) == base {
						note = "当前使用版本,删除后请重新 sdk use/default"
					} else if h := usageHint(v, staleDays); h != "" {
						note = "⚠ " + h
					}
					group.Items = append(group.Items, &Target{
						ID: tool + ":sdkman:" + cand.Name() + ":" + base, Tool: tool, ToolTitle: title,
						Category: "工具链", Title: cand.Name() + " " + base,
						Path: v, AllowedRoot: cands,
						Description: "最后修改: " + lastActivityDesc(v),
						Risk:        RiskHigh, Size: size, Count: files,
						Method: MethodDir, Available: true, Note: note,
					})
				}
			}
		}
		sort.SliceStable(group.Items, func(i, j int) bool { return group.Items[i].Size > group.Items[j].Size })
		emitIfAny(group, emit)

		// sdkman download temp dir — often huge after upgrades.
		tmp := filepath.Join(sdkman, "tmp")
		if size, files := DirSize(ctx, tmp); size > 0 {
			emit(&Target{
				ID: tool + ":sdkman-tmp", Tool: tool, ToolTitle: title,
				Category: "临时文件", Title: "sdkman 下载临时目录 (tmp)",
				Path: tmp, AllowedRoot: sdkman,
				Description: "安装/升级过程残留的下载文件,可安全清空",
				Risk:        RiskSafe, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
		archives := filepath.Join(sdkman, "archives")
		if size, files := DirSize(ctx, archives); size > 0 {
			emit(&Target{
				ID: tool + ":sdkman-archives", Tool: tool, ToolTitle: title,
				Category: "包缓存", Title: "sdkman 发行版缓存 (archives)",
				Path: archives, AllowedRoot: sdkman,
				Description: "已下载的 SDK 压缩包,删除后需要时重新下载",
				Risk:        RiskSafe, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}

	// --- pyenv: python versions ---
	pyenvVersions := filepath.Join(env.Home, ".pyenv", "versions")
	if isDir(pyenvVersions) {
		current := strings.TrimSpace(readSmallFile(filepath.Join(env.Home, ".pyenv", "version")))
		group := &Target{
			ID: tool + ":pyenv", Tool: tool, ToolTitle: title,
			Category: "工具链", Title: "Python 版本 (pyenv)",
			Path: pyenvVersions, AllowedRoot: pyenvVersions,
			Description: "pyenv 安装的 Python 版本;全局版本: " + current,
			Risk:        RiskHigh, Method: MethodGroup, Available: true,
		}
		for _, v := range listDirs(pyenvVersions) {
			size, files := DirSize(ctx, v)
			if size == 0 {
				continue
			}
			base := filepath.Base(v)
			note := ""
			if versionMatches(current, base) {
				note = "当前全局版本,删除前请先 pyenv global 切换"
			} else if h := usageHint(v, staleDays); h != "" {
				note = "⚠ " + h
			}
			group.Items = append(group.Items, &Target{
				ID: tool + ":pyenv:" + base, Tool: tool, ToolTitle: title,
				Category: "工具链", Title: "Python " + base,
				Path: v, AllowedRoot: pyenvVersions,
				Description: "最后修改: " + lastActivityDesc(v),
				Risk:        RiskHigh, Size: size, Count: files,
				Method: MethodDir, Available: true, Note: note,
			})
		}
		emitIfAny(group, emit)
	}

	// --- Go extra SDKs: ~/sdk/go1.* (go install golang.org/dl/...) ---
	sdkDir := filepath.Join(env.Home, "sdk")
	if isDir(sdkDir) {
		group := &Target{
			ID: tool + ":gosdk", Tool: tool, ToolTitle: title,
			Category: "工具链", Title: "Go 多版本 (~/sdk)",
			Path: sdkDir, AllowedRoot: sdkDir,
			Description: "通过 golang.org/dl 安装的额外 Go 工具链",
			Risk:        RiskHigh, Method: MethodGroup, Available: true,
		}
		for _, v := range listDirs(sdkDir) {
			base := filepath.Base(v)
			if !strings.HasPrefix(base, "go1") {
				continue
			}
			size, files := DirSize(ctx, v)
			if size == 0 {
				continue
			}
			note := ""
			if h := usageHint(v, staleDays); h != "" {
				note = "⚠ " + h
			}
			group.Items = append(group.Items, &Target{
				ID: tool + ":gosdk:" + base, Tool: tool, ToolTitle: title,
				Category: "工具链", Title: "Go " + base,
				Path: v, AllowedRoot: sdkDir,
				Description: "最后修改: " + lastActivityDesc(v),
				Risk:        RiskHigh, Size: size, Count: files,
				Method: MethodDir, Available: true, Note: note,
			})
		}
		emitIfAny(group, emit)
	}
}

func emitIfAny(group *Target, emit func(*Target)) {
	if len(group.Items) > 0 {
		emit(group)
	}
}

// listDirs returns the immediate sub-directories of dir.
func listDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// globDirs returns existing directories matching a glob pattern.
func globDirs(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range matches {
		if isDir(m) {
			out = append(out, m)
		}
	}
	return out
}

// readSmallFile returns the trimmed content of a small text file (nvm
// alias, pyenv version) or "" when unavailable.
func readSmallFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 64 {
		return ""
	}
	return s
}

// resolveSymlink returns the target of a symlink directory or "".
func resolveSymlink(path string) string {
	res, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return res
}

// versionMatches tolerates "v" prefixes and partial pins ("22" matches
// "v22.18.0").
func versionMatches(current, dirName string) bool {
	if current == "" {
		return false
	}
	c := strings.TrimPrefix(current, "v")
	d := strings.TrimPrefix(dirName, "v")
	return c == d || strings.HasPrefix(d, c)
}
