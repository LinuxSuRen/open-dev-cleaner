package scan

import (
	"context"
	"path/filepath"
)

// InfraProvider scans R&D / ops tooling: OpenTofu, Terraform, Packer,
// Pulumi, Vagrant, Ansible, AWS CLI/SAM/CDK, Bazel and tflint.
type InfraProvider struct{}

func (p *InfraProvider) Key() string   { return "infra" }
func (p *InfraProvider) Title() string { return "研发/运维" }

func (p *InfraProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	type infraDir struct {
		id, name, path, root, cat, desc string
		risk                            RiskLevel
		contentsOnly                    bool
	}
	var ts []infraDir

	// --- OpenTofu ---
	ts = append(ts, infraDir{"tofu-plugin-cache", "OpenTofu 插件缓存 (~/.tofu.d/plugin-cache)",
		filepath.Join(env.Home, ".tofu.d", "plugin-cache"), filepath.Join(env.Home, ".tofu.d"),
		"插件缓存", "已下载的 provider 插件,删除后下次 init 重新下载", RiskCaution, false})
	ts = append(ts, infraDir{"tofu-all", "⚠ OpenTofu 数据目录 (~/.tofu.d)",
		filepath.Join(env.Home, ".tofu.d"), env.Home,
		"工具数据", "全部本地数据(可能含 CLI 配置与凭证文件),不再使用时整体删除", RiskHigh, false})

	// --- Terraform ---
	ts = append(ts, infraDir{"tf-plugin-cache", "Terraform 插件缓存 (~/.terraform.d/plugin-cache)",
		filepath.Join(env.Home, ".terraform.d", "plugin-cache"), filepath.Join(env.Home, ".terraform.d"),
		"插件缓存", "已下载的 provider 插件,删除后下次 init 重新下载", RiskCaution, false})
	ts = append(ts, infraDir{"tf-checkpoint-cache", "Terraform checkpoint 缓存",
		filepath.Join(env.Home, ".terraform.d", "checkpoint-cache"), filepath.Join(env.Home, ".terraform.d"),
		"缓存", "版本检查缓存,可安全删除", RiskSafe, false})
	ts = append(ts, infraDir{"tf-all", "⚠ Terraform 数据目录 (~/.terraform.d)",
		filepath.Join(env.Home, ".terraform.d"), env.Home,
		"工具数据", "全部本地数据(常含 terraform.rc 与 credentials.tfrc.json 登录凭证),不再使用时整体删除", RiskHigh, false})

	// --- Packer ---
	ts = append(ts, infraDir{"packer-plugins", "Packer 插件 (~/.packer.d)",
		filepath.Join(env.Home, ".packer.d", "plugins"), filepath.Join(env.Home, ".packer.d"),
		"插件", "已安装的 builder/provisioner 插件,删除后 packer init 重新下载", RiskCaution, false})

	// --- Pulumi ---
	ts = append(ts, infraDir{"pulumi-plugins", "Pulumi 插件 (~/.pulumi/plugins)",
		filepath.Join(env.Home, ".pulumi", "plugins"), filepath.Join(env.Home, ".pulumi"),
		"插件", "已下载的语言/云插件,删除后按需自动重新下载", RiskCaution, false})
	ts = append(ts, infraDir{"pulumi-all", "⚠ Pulumi 数据目录 (~/.pulumi)",
		filepath.Join(env.Home, ".pulumi"), env.Home,
		"工具数据", "含登录凭证与本地缓存;若使用 file(local) backend,状态文件也在其中,删除前请确认", RiskHigh, false})

	// --- Vagrant ---
	ts = append(ts, infraDir{"vagrant-boxes", "Vagrant boxes (~/.vagrant.d/boxes)",
		filepath.Join(env.Home, ".vagrant.d", "boxes"), filepath.Join(env.Home, ".vagrant.d"),
		"镜像", "已下载的虚拟机镜像,删除后 vagrant box add 重新下载(通常体积很大)", RiskCaution, false})
	ts = append(ts, infraDir{"vagrant-tmp", "Vagrant 临时目录",
		filepath.Join(env.Home, ".vagrant.d", "tmp"), filepath.Join(env.Home, ".vagrant.d"),
		"临时文件", "安装/打包过程残留,可安全清空", RiskSafe, true})
	ts = append(ts, infraDir{"vagrant-all", "⚠ Vagrant 数据目录 (~/.vagrant.d)",
		filepath.Join(env.Home, ".vagrant.d"), env.Home,
		"工具数据", "含 boxes、机器状态与全局配置,删除后已有虚拟机将无法被 vagrant 管理", RiskHigh, false})

	// --- Ansible ---
	ts = append(ts, infraDir{"ansible-tmp", "Ansible 临时目录",
		filepath.Join(env.Home, ".ansible", "tmp"), filepath.Join(env.Home, ".ansible"),
		"临时文件", "模块传输临时文件,可安全清空", RiskSafe, true})
	ts = append(ts, infraDir{"ansible-collections", "Ansible collections",
		filepath.Join(env.Home, ".ansible", "collections"), filepath.Join(env.Home, ".ansible"),
		"依赖", "已安装的 collection,删除后 ansible-galaxy collection install -r requirements.yml 重装", RiskCaution, false})
	ts = append(ts, infraDir{"ansible-roles", "Ansible roles",
		filepath.Join(env.Home, ".ansible", "roles"), filepath.Join(env.Home, ".ansible"),
		"依赖", "已安装的 role,删除后 ansible-galaxy install 重装", RiskCaution, false})

	// --- AWS CLI / SAM / CDK ---
	ts = append(ts, infraDir{"aws-cli-cache", "AWS CLI 凭证缓存 (~/.aws/cli/cache)",
		filepath.Join(env.Home, ".aws", "cli", "cache"), filepath.Join(env.Home, ".aws"),
		"缓存", "SSO/botocore 令牌缓存,删除后需重新 aws sso login 或重新取凭证", RiskCaution, false})
	ts = append(ts, infraDir{"aws-sam", "AWS SAM 构建缓存 (~/.aws-sam)",
		filepath.Join(env.Home, ".aws-sam"), env.Home,
		"构建缓存", "sam build 的依赖与产物缓存,删除后自动重建", RiskCaution, false})
	ts = append(ts, infraDir{"aws-cdk", "AWS CDK 缓存 (~/.cdk/cache)",
		filepath.Join(env.Home, ".cdk", "cache"), filepath.Join(env.Home, ".cdk"),
		"缓存", "CDK/jsii 构建缓存,删除后自动重建", RiskSafe, false})

	// --- tflint ---
	ts = append(ts, infraDir{"tflint-plugins", "tflint 插件 (~/.tflint.d/plugins)",
		filepath.Join(env.Home, ".tflint.d", "plugins"), filepath.Join(env.Home, ".tflint.d"),
		"插件", "已下载的 tflint 插件,删除后 tflint --init 重新安装", RiskCaution, false})

	for _, t := range ts {
		if !Exists(t.path) {
			continue
		}
		size, files := DirSize(ctx, t.path)
		if size == 0 {
			continue
		}
		note := ""
		if h := usageHint(t.path, 180); h != "" && t.risk != RiskSafe {
			note = "⚠ " + h
		}
		method := MethodDir
		if t.contentsOnly {
			method = MethodDirContents
		}
		emit(&Target{
			ID: tool + ":" + t.id, Tool: tool, ToolTitle: title,
			Category: t.cat, Title: t.name,
			Path: t.path, AllowedRoot: t.root,
			Description: t.desc, Risk: t.risk, Size: size, Count: files,
			Method: method, Available: true, Note: note,
		})
	}

	// --- Bazel ---
	// unix: ~/.cache/bazel | ~/Library/Caches/bazel
	// windows: %USERPROFILE%\_bazel_<user> output bases
	bazelDirs := []string{filepath.Join(env.CacheDir, "bazel")}
	if env.GOOS == "windows" {
		bazelDirs = globDirs(filepath.Join(env.Home, "_bazel*"))
	}
	for _, b := range bazelDirs {
		if !Exists(b) {
			continue
		}
		size, files := DirSize(ctx, b)
		if size == 0 {
			continue
		}
		id := tool + ":bazel-cache"
		name := "Bazel 输出与下载缓存 (" + b + ")"
		if len(bazelDirs) > 1 {
			id = tool + ":bazel-cache:" + filepath.Base(b)
		}
		emit(&Target{
			ID: id, Tool: tool, ToolTitle: title,
			Category: "构建缓存", Title: name,
			Path: b, AllowedRoot: filepath.Dir(b),
			Description: "Bazel 的输出库、仓库缓存与下载缓存,常达数十 GB;删除后下次构建重新下载并全量编译",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
}
