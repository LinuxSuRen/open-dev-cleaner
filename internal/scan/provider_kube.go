package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// isCommandNotFound reports whether an exec error means the binary is
// missing (as opposed to running and failing).
func isCommandNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, exec.ErrNotFound) ||
		strings.Contains(err.Error(), "executable file not found") ||
		strings.Contains(err.Error(), "command not found")
}

// KubeProvider scans the Kubernetes ecosystem: kubectl/krew, Helm,
// minikube, colima/lima, Rancher Desktop, Podman, nerdctl, k9s,
// Skaffold and Okteto.
type KubeProvider struct{}

func (p *KubeProvider) Key() string   { return "kube" }
func (p *KubeProvider) Title() string { return "Kubernetes" }

func (p *KubeProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	type kubeDir struct {
		id, name, path, root, cat, desc string
		risk                            RiskLevel
		contentsOnly                    bool
	}
	var ts []kubeDir

	// --- kubectl discovery cache ---
	ts = append(ts, kubeDir{"kubectl-cache", "kubectl API 发现缓存 (~/.kube/cache)",
		filepath.Join(env.Home, ".kube", "cache"), filepath.Join(env.Home, ".kube"),
		"缓存", "kubectl 的 API discovery 缓存,删除后首次执行自动重建", RiskSafe, false})

	// --- krew (kubectl plugin manager) ---
	ts = append(ts, kubeDir{"krew-cache", "krew 插件下载缓存 (~/.krew/cache)",
		filepath.Join(env.Home, ".krew", "cache"), filepath.Join(env.Home, ".krew"),
		"包缓存", "krew 下载的插件安装包,删除后需要时重新下载", RiskSafe, false})
	ts = append(ts, kubeDir{"krew-all", "krew 插件目录 (~/.krew)",
		filepath.Join(env.Home, ".krew"), env.Home,
		"工具数据", "已安装的全部 kubectl 插件与索引,删除后需重新 krew install", RiskCaution, false})

	// --- Helm ---
	ts = append(ts, kubeDir{"helm-cache", "Helm 缓存 (repo 索引/图表/registry)",
		filepath.Join(env.CacheDir, "helm"), env.CacheDir,
		"包缓存", "Helm 仓库索引、图表与 registry 层缓存,删除后需要时重新下载", RiskSafe, false})
	helmData := filepath.Join(env.Home, ".local", "share", "helm")
	if env.GOOS == "darwin" {
		helmData = filepath.Join(env.AppData, "helm")
	} else if env.GOOS == "windows" {
		helmData = filepath.Join(env.Home, "AppData", "Roaming", "helm")
	}
	ts = append(ts, kubeDir{"helm-plugins", "Helm 插件",
		filepath.Join(helmData, "plugins"), helmData,
		"插件", "已安装的 Helm 插件,删除后需重新 helm plugin install", RiskCaution, false})

	// --- minikube ---
	ts = append(ts, kubeDir{"minikube-cache", "minikube 下载缓存 (镜像/二进制)",
		filepath.Join(env.Home, ".minikube", "cache"), filepath.Join(env.Home, ".minikube"),
		"包缓存", "minikube 下载的 VM 镜像与 kubectl/启动镜像缓存,删除后重新拉取", RiskCaution, false})
	ts = append(ts, kubeDir{"minikube-all", "⚠ minikube 本地集群 (~/.minikube)",
		filepath.Join(env.Home, ".minikube"), env.Home,
		"集群数据", "整个本地集群:VM 磁盘、证书与配置。删除后需 minikube start 重建集群(里面的数据会丢失)", RiskHigh, false})

	// --- colima / lima ---
	ts = append(ts, kubeDir{"colima", "⚠ Colima 虚拟机 (~/.colima)",
		filepath.Join(env.Home, ".colima"), env.Home,
		"集群数据", "Colima 的容器/虚拟机数据。删除将移除虚拟机与其中容器,需 colima start 重建", RiskHigh, false})
	ts = append(ts, kubeDir{"lima-cache", "Lima VM 镜像缓存",
		filepath.Join(env.CacheDir, "lima"), env.CacheDir,
		"包缓存", "Lima 下载的 VM 发行版镜像,删除后需要时重新下载", RiskCaution, false})
	ts = append(ts, kubeDir{"lima-instances", "⚠ Lima 虚拟机实例 (~/.lima)",
		filepath.Join(env.Home, ".lima"), env.Home,
		"集群数据", "Lima 虚拟机实例数据。删除将移除虚拟机,需重新 limactl create", RiskHigh, false})

	// --- Rancher Desktop (mac/linux) ---
	if env.GOOS != "windows" {
		ts = append(ts, kubeDir{"rancher-desktop", "⚠ Rancher Desktop 数据",
			filepath.Join(env.AppData, "rancher-desktop"), filepath.Dir(env.AppData),
			"集群数据", "Rancher Desktop 的虚拟机磁盘与配置。删除将移除容器与 k8s 集群,需重新初始化", RiskHigh, false})
	}

	// --- k9s ---
	ts = append(ts, kubeDir{"k9s", "k9s 缓存与日志 (~/.k9s)",
		filepath.Join(env.Home, ".k9s"), env.Home,
		"缓存", "k9s 终端 UI 的缓存、日志与皮肤配置(配置文件很小,大头是缓存)", RiskSafe, false})

	// --- Skaffold / Okteto ---
	ts = append(ts, kubeDir{"skaffold-cache", "Skaffold 缓存 (~/.skaffold/cache)",
		filepath.Join(env.Home, ".skaffold", "cache"), filepath.Join(env.Home, ".skaffold"),
		"缓存", "Skaffold 构建缓存,删除后自动重建", RiskSafe, false})
	ts = append(ts, kubeDir{"okteto", "Okteto 数据 (~/.okteto)",
		filepath.Join(env.Home, ".okteto"), env.Home,
		"工具数据", "Okteto CLI、上下文与登录凭证,删除后需重新 okteto login", RiskCaution, false})

	// --- nerdctl rootless image store ---
	ts = append(ts, kubeDir{"containerd-rootless", "containerd rootless 镜像存储",
		filepath.Join(env.Home, ".local", "share", "containerd"), filepath.Join(env.Home, ".local", "share"),
		"镜像存储", "nerdctl/containerd rootless 模式拉取的镜像,删除后需重新拉取", RiskCaution, false})

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

	p.scanPodman(ctx, env, tool, title, emit)
}

// scanPodman mirrors the Docker provider using `podman system df`, which
// shares the same JSON shape. Podman is the most common Docker
// alternative in the Kubernetes/kind toolchains.
func (p *KubeProvider) scanPodman(ctx context.Context, env *Env, tool, title string, emit func(*Target)) {
	if env == nil || env.NoExternal {
		return // external commands disabled (tests / --no-external)
	}
	out, err := runCmd(ctx, env, 20*time.Second, "podman", "system", "df", "--format", "{{json .}}")
	if err != nil {
		if isCommandNotFound(err) {
			return // podman not installed
		}
		note := "Podman 未运行或不可用:" + firstLine(err.Error()) + ",启动后重新扫描可统计镜像与容器"
		emit(&Target{
			ID: tool + ":podman-unavailable", Tool: tool, ToolTitle: title,
			Category: "镜像", Title: "Podman 未运行",
			Description: note, Note: note,
			Risk: RiskCaution, Available: false,
		})
		return
	}

	rows := map[string]dockerDFRow{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var row dockerDFRow
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &row); err == nil && row.Type != "" {
			rows[row.Type] = row
		}
	}

	if row, ok := rows["Images"]; ok && row.TotalCount > 0 {
		emit(&Target{
			ID: tool + ":podman-image-prune", Tool: tool, ToolTitle: title,
			Category: "镜像", Title: "Podman 悬空镜像 (docker.io 残留层)",
			Description: "未被使用的悬空镜像层 (等效 podman image prune),删除后按需重新拉取",
			Risk:        RiskCaution, Size: reclaimableBytes(row),
			Method: MethodCommand, Cmd: []string{"podman", "image", "prune", "-f"},
			Available: true,
		})
		emit(&Target{
			ID: tool + ":podman-image-prune-all", Tool: tool, ToolTitle: title,
			Category: "镜像", Title: fmt.Sprintf("Podman 未使用镜像 (%d 个)", row.TotalCount-row.Active),
			Description: "没有任何容器使用的镜像 (等效 podman image prune -a),删除后需重新拉取",
			Risk:        RiskHigh, Size: reclaimableBytes(row),
			Method: MethodCommand, Cmd: []string{"podman", "image", "prune", "-a", "-f"},
			Available: true,
		})
	}
	if row, ok := rows["Containers"]; ok && row.TotalCount-row.Active > 0 {
		emit(&Target{
			ID: tool + ":podman-container-prune", Tool: tool, ToolTitle: title,
			Category: "容器", Title: fmt.Sprintf("Podman 已停止的容器 (%d 个)", row.TotalCount-row.Active),
			Description: "退出状态的容器及其可写层 (等效 podman container prune)",
			Risk:        RiskCaution, Size: reclaimableBytes(row),
			Method: MethodCommand, Cmd: []string{"podman", "container", "prune", "-f"},
			Available: true,
		})
	}
	if row, ok := rows["Local Volumes"]; ok && row.TotalCount-row.Active > 0 {
		emit(&Target{
			ID: tool + ":podman-volume-prune", Tool: tool, ToolTitle: title,
			Category: "数据卷", Title: fmt.Sprintf("Podman 未使用的数据卷 (%d 个)", row.TotalCount-row.Active),
			Description: "数据卷通常含数据库等持久化数据,删除后不可恢复 (等效 podman volume prune)",
			Risk:        RiskDangerous, Size: reclaimableBytes(row),
			Method: MethodCommand, Cmd: []string{"podman", "volume", "prune", "-f"},
			Available: true,
		})
	}
}
