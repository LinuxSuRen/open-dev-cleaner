package scan

import (
	"context"
	"path/filepath"
)

// JavaProvider scans JVM build tool caches: Gradle and Maven.
type JavaProvider struct{}

func (p *JavaProvider) Key() string   { return "java" }
func (p *JavaProvider) Title() string { return "Java" }

func (p *JavaProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()
	gradleHome := filepath.Join(env.Home, ".gradle")

	// ~/.gradle/caches — dependencies & transform caches
	targets := []struct {
		id, cat, name, rel, desc string
		risk                     RiskLevel
		contentsOnly             bool
	}{
		{"gradle-caches", "包缓存", "Gradle 依赖与变换缓存", filepath.Join("caches"),
			"Gradle 下载的依赖与构建变换缓存,删除后构建需重新联网下载", RiskCaution, false},
		{"gradle-wrapper", "发行版缓存", "Gradle Wrapper 发行版", filepath.Join("wrapper", "dists"),
			"各项目 wrapper 下载的 Gradle 发行版压缩包与解压目录,删除后按需重新下载", RiskCaution, false},
		{"gradle-daemon", "日志", "Gradle Daemon 日志", filepath.Join("daemon"),
			"守护进程日志与临时状态,可安全删除", RiskSafe, false},
		{"gradle-native", "缓存", "Gradle native 支持库", filepath.Join("native"),
			"按平台解压的 native 库,删除后自动重建", RiskSafe, false},
		{"gradle-tmp", "临时文件", "Gradle 临时目录 (.tmp)", filepath.Join(".tmp"),
			"构建过程的临时文件,通常在构建结束后就是垃圾,可安全清空", RiskSafe, true},
		{"maven-repo", "包缓存", "Maven 本地仓库 (~/.m2/repository)", filepath.Join(".m2", "repository"),
			"Maven 下载的全部依赖,删除后构建需重新联网下载(体积通常很大)", RiskCaution, false},
		{"maven-wrapper", "发行版缓存", "Maven Wrapper 发行版", filepath.Join(".m2", "wrapper", "dists"),
			"各项目 wrapper 下载的 Maven 发行版,删除后按需重新下载", RiskCaution, false},
	}

	for _, t := range targets {
		var path string
		if t.id == "maven-repo" || t.id == "maven-wrapper" {
			path = filepath.Join(env.Home, filepath.FromSlash(t.rel))
		} else {
			path = filepath.Join(gradleHome, filepath.FromSlash(t.rel))
		}
		if !Exists(path) {
			continue
		}
		size, files := DirSize(ctx, path)
		if size == 0 {
			continue
		}
		method := MethodDir
		if t.contentsOnly {
			method = MethodDirContents
		}
		emit(&Target{
			ID: tool + ":" + t.id, Tool: tool, ToolTitle: title,
			Category: t.cat, Title: t.name,
			Path: path, AllowedRoot: filepath.Dir(path),
			Description: t.desc, Risk: t.risk, Size: size, Count: files,
			Method: method, Available: true,
		})
	}
}
