package scan

import (
	"context"
	"path/filepath"
)

// PythonProvider scans Python package manager caches.
type PythonProvider struct{}

func (p *PythonProvider) Key() string   { return "python" }
func (p *PythonProvider) Title() string { return "Python" }

func (p *PythonProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	type pyTarget struct {
		id, cat, name string
		path          string
		desc          string
		risk          RiskLevel
	}
	var ts []pyTarget

	// pip: ~/Library/Caches/pip | ~/.cache/pip | %LocalAppData%\pip\cache
	pipCache := filepath.Join(env.CacheDir, "pip")
	if env.GOOS == "windows" {
		pipCache = filepath.Join(env.CacheDir, "pip", "cache")
	}
	ts = append(ts, pyTarget{"pip-cache", "包缓存", "pip 下载缓存", pipCache,
		"pip 安装包的下载缓存,删除后安装需重新联网 (等效 pip cache purge)", RiskSafe})

	// uv: ~/Library/Caches/uv | ~/.cache/uv
	uv := filepath.Join(env.CacheDir, "uv")
	ts = append(ts, pyTarget{"uv-cache", "包缓存", "uv 缓存", uv,
		"uv 的包与 wheel 缓存,删除后安装需重新联网下载 (等效 uv cache clean)", RiskCaution})

	// poetry: ~/Library/Caches/pypoetry | ~/.cache/pypoetry
	poetry := filepath.Join(env.CacheDir, "pypoetry")
	ts = append(ts, pyTarget{"poetry-cache", "包缓存", "Poetry 缓存", poetry,
		"Poetry 的虚拟环境与包缓存,删除后需重建 (等效 poetry cache clear --all .)", RiskCaution})

	// conda pkgs
	for _, root := range []string{
		filepath.Join(env.Home, "miniconda3"),
		filepath.Join(env.Home, "anaconda3"),
		filepath.Join(env.Home, "miniforge3"),
	} {
		pkgs := filepath.Join(root, "pkgs")
		ts = append(ts, pyTarget{"conda-pkgs:" + root, "包缓存", "Conda 包缓存 (" + pkgs + ")", pkgs,
			"Conda 已下载的包归档与解压目录,删除后需重新下载", RiskCaution})
	}

	// pipenv: ~/.cache/pipenv | ~/Library/Caches/pipenv
	pipenv := filepath.Join(env.CacheDir, "pipenv")
	ts = append(ts, pyTarget{"pipenv-cache", "包缓存", "Pipenv 缓存", pipenv,
		"Pipenv 的虚拟环境与缓存,删除后需重建", RiskCaution})

	for _, t := range ts {
		if !Exists(t.path) {
			continue
		}
		size, files := DirSize(ctx, t.path)
		if size == 0 {
			continue
		}
		root := t.path
		if t.id == "pip-cache" {
			root = env.CacheDir
		}
		emit(&Target{
			ID: tool + ":" + t.id, Tool: tool, ToolTitle: title,
			Category: t.cat, Title: t.name,
			Path: t.path, AllowedRoot: parentOf(root),
			Description: t.desc, Risk: t.risk, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
}
