package scan

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// WorkspaceProvider walks project roots (~/Workspace, ~/PycharmProjects...)
// looking for well-known build outputs and dependency directories.
type WorkspaceProvider struct{}

func (p *WorkspaceProvider) Key() string   { return "workspace" }
func (p *WorkspaceProvider) Title() string { return "工作区" }

type artifactRule struct {
	cat  string
	risk RiskLevel
	desc string
}

var artifactRules = map[string]artifactRule{
	"node_modules": {
		"Node.js 依赖", RiskHigh,
		"项目依赖目录,删除后执行 npm/pnpm/yarn install 即可恢复(需要网络)",
	},
	"__pycache__": {
		"Python 字节码", RiskSafe,
		"Python 字节码缓存,删除后运行时自动重建",
	},
	".pytest_cache": {
		"测试缓存", RiskSafe, "pytest 缓存,删除后自动重建",
	},
	".mypy_cache": {
		"类型检查缓存", RiskSafe, "mypy 类型检查缓存,删除后自动重建",
	},
	".ruff_cache": {
		"Linter 缓存", RiskSafe, "ruff 缓存,删除后自动重建",
	},
	"target": {
		"构建产物", RiskCaution,
		"Rust (cargo) / Java (Maven) 的构建输出目录,删除后重新构建即可恢复",
	},
	"build": {
		"构建产物", RiskCaution,
		"通用构建输出目录 (Gradle 等),删除后重新构建即可恢复",
	},
	"dist": {
		"构建产物", RiskCaution,
		"前端打包产物,删除后重新构建即可恢复",
	},
	".next": {
		"构建产物", RiskCaution, "Next.js 构建产物,删除后重新构建即可恢复",
	},
	".nuxt": {
		"构建产物", RiskCaution, "Nuxt 构建产物,删除后重新构建即可恢复",
	},
	".output": {
		"构建产物", RiskCaution, "Nitro/Nuxt 输出目录,删除后重新构建即可恢复",
	},
	".turbo": {
		"构建缓存", RiskSafe, "Turborepo 任务缓存,删除后自动重建",
	},
	".gradle": {
		"构建缓存", RiskSafe, "项目级 Gradle 缓存,删除后自动重建",
	},
	"Pods": {
		"iOS 依赖", RiskHigh, "CocoaPods 依赖目录,删除后执行 pod install 恢复(需要网络)",
	},
	".build": {
		"构建产物", RiskCaution, "Swift Package Manager 构建产物,删除后 swift build 重新构建",
	},
	"venv": {
		"虚拟环境", RiskHigh, "Python 虚拟环境,删除后需要重新创建并安装依赖",
	},
	".venv": {
		"虚拟环境", RiskHigh, "Python 虚拟环境,删除后需要重新创建并安装依赖",
	},
}

// skipDirs are never entered while searching for artifacts.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"vendor": true, // go/php vendor dirs are intentional, not trash
}

const maxWalkDepth = 8

func (p *WorkspaceProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	for _, root := range env.Workspaces {
		if ctx.Err() != nil {
			return
		}
		children := collectArtifacts(ctx, root)
		if len(children) == 0 {
			continue
		}
		var total int64
		maxRisk := RiskSafe
		for _, c := range children {
			total += c.Size
			if RiskOrder(c.Risk) > RiskOrder(maxRisk) {
				maxRisk = c.Risk
			}
		}
		emit(&Target{
			ID: tool + ":" + root, Tool: tool, ToolTitle: title,
			Category: "依赖/构建产物", Title: "项目工作区 " + root,
			Description: fmt.Sprintf("在 %s 下发现 %d 个可清理目录(node_modules、构建产物、缓存等)", root, len(children)),
			Risk:        maxRisk, Size: total,
			Method: MethodGroup, Available: true,
			Items: children,
		})
	}
}

func collectArtifacts(ctx context.Context, root string) []*Target {
	var out []*Target
	seen := map[string]bool{}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil // skip unreadable and files
		}
		base := d.Name()
		if path != root && skipDirs[base] {
			return fs.SkipDir
		}
		if depth(root, path) > maxWalkDepth {
			return fs.SkipDir
		}
		rule, ok := artifactRules[base]
		if !ok || path == root {
			return nil
		}
		real, rerr := filepath.EvalSymlinks(path)
		if rerr != nil {
			real = path
		}
		if seen[real] {
			return fs.SkipDir
		}
		seen[real] = true

		size, files := DirSize(ctx, path)
		if size > 0 {
			rel, _ := filepath.Rel(root, path)
			out = append(out, &Target{
				ID: "workspace:artifact:" + real, Tool: "workspace", ToolTitle: "工作区",
				Category: rule.cat, Title: rel,
				Path: path, AllowedRoot: root,
				Description: rule.desc, Risk: rule.risk, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
		return fs.SkipDir // do not register nested artifacts inside
	})
	return out
}

func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator))
}
