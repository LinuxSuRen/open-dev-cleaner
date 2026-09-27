package scan

import (
	"context"
	"path/filepath"
)

// RustProvider scans cargo caches. rustup toolchains are intentionally NOT
// offered for cleaning (they are managed by rustup itself).
type RustProvider struct{}

func (p *RustProvider) Key() string   { return "rust" }
func (p *RustProvider) Title() string { return "Rust" }

func (p *RustProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()
	cargoHome := filepath.Join(env.Home, ".cargo")

	registry := filepath.Join(cargoHome, "registry")
	if size, files := DirSize(ctx, registry); size > 0 {
		emit(&Target{
			ID: tool + ":registry", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "Cargo registry 缓存",
			Path: registry, AllowedRoot: cargoHome,
			Description: "crates.io 的源码与 .crate 归档缓存,删除后构建需重新联网下载",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}

	git := filepath.Join(cargoHome, "git")
	if size, files := DirSize(ctx, git); size > 0 {
		emit(&Target{
			ID: tool + ":git", Tool: tool, ToolTitle: title,
			Category: "包缓存", Title: "Cargo git 依赖缓存",
			Path: git, AllowedRoot: cargoHome,
			Description: "git 依赖的裸仓库与检出缓存,删除后按需重新克隆",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
}
