# Open Dev Cleaner 🧹

**开发者磁盘清理工具** — 专为开发者打造的跨平台磁盘清理开源工具,扫描并清理各类开发工具的缓存、镜像、构建产物,按**危险等级分级展示**,内置 Web 界面,**全部资源嵌入单个二进制文件**。

A cross-platform (Windows / macOS / Linux) open-source disk cleaner for developers. It scans caches, container images and build outputs of common dev tools, grades every item by **risk level**, ships a built-in web UI, and embeds **everything into a single binary**.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)](.goreleaser.yml)

## ✨ 特性 / Features

- **一键扫描**:并发扫描十几个开发工具生态的磁盘占用(见下表)
- **磁盘概览**:按平台展示全部卷的总空间/剩余空间——Linux 遍历 `/proc/mounts` 挂载点、macOS 识别 APFS 卷(`/` 与 `/Volumes/*`)、Windows 枚举盘符并区分本地/可移动/网络驱动器;清理完成后自动刷新
- **危险等级分级**:每个清理项都标注 `安全 / 谨慎 / 高风险 / 危险` 四级,默认只勾选安全项,危险项需输入确认
- **Web 界面**:Vue 3 驱动的响应式界面,支持暗色主题、搜索、风险过滤、实时清理日志(SSE 流式)
- **单二进制**:前端(Vue 运行时 + 组件)全部 `go:embed` 嵌入,编译产物一个文件,零依赖、离线可用
- **API**:`GET /api/disks` 各卷用量、`GET /api/scan` 扫描、`POST /api/clean` 清理,便于二次集成
- **跨平台**:同一份代码支持 Windows / macOS / Linux(amd64 / arm64)
- **CLI + Web 双模式**:既能开网页点选,也能在终端 `scan` / `clean` / `--dry-run`
- **安全护栏**:目录删除强制校验"安全根",绝不越界;`--dry-run` 全链路预览

## 📦 安装 / Install

从 [Releases](../../releases) 下载对应平台的压缩包(`open-dev-cleaner_<os>_<arch>.tar.gz` / `.zip`),或自行编译:

```bash
go install github.com/LinuxSuRen/open-dev-cleaner@latest
# 或
git clone https://github.com/LinuxSuRen/open-dev-cleaner.git
cd open-dev-cleaner && make build
./bin/open-dev-cleaner
```

### 发布流程 / Release flow

推送代码后,在 GitHub 上创建并 **Publish 一个 Release**(打 `v*` tag)即可:

1. `build` 工作流对每次 push / PR 运行 `go vet` + `go test`
2. Release 发布后,`release` 工作流自动交叉编译 **linux / darwin / windows × amd64 / arm64** 六个平台,生成压缩包与 `checksums.txt`,并上传挂载到该 Release

也支持在 Actions 页面手动 `workflow_dispatch` 指定 tag 补传二进制。本地可用 `make snapshot` 快速构建全平台产物。

## 🚀 使用 / Usage

```bash
# 启动 Web 界面(默认 http://127.0.0.1:8420,自动打开浏览器)
open-dev-cleaner web

open-dev-cleaner web --addr 0.0.0.0:8420   # 监听所有网卡(局域网访问)
open-dev-cleaner web --no-open             # 不自动打开浏览器

# 命令行模式
open-dev-cleaner scan                       # 表格展示全部可清理项
open-dev-cleaner scan --risk safe           # 只看安全项
open-dev-cleaner scan --format json         # JSON 输出,便于脚本处理
open-dev-cleaner clean --all --risk safe --yes   # 清理全部安全项
open-dev-cleaner clean --ids go:build-cache,docker:dangling --dry-run
```

环境变量:`ODC_WORKSPACE`(追加要扫描的工作区根目录,`:` 分隔)。

## 🗂 支持的工具 / Supported tools

| 工具 | 清理项 | 危险等级 |
|---|---|---|
| **Go** | 构建缓存 `GOCACHE`、模块缓存 `GOMODCACHE`、goimports/gopls 索引 | 安全 / 谨慎 |
| **Node.js** | npm `_cacache`、pnpm store、Yarn 缓存、node-gyp 头文件 | 安全 / 谨慎 |
| **Java** | Gradle caches / wrapper / daemon / `.tmp`、Maven 本地仓库、Maven wrapper | 安全 / 谨慎 |
| **Python** | pip / uv / poetry / pipenv / conda 缓存 | 安全 / 谨慎 |
| **Rust** | Cargo registry / git 依赖缓存(rustup 工具链不动) | 谨慎 |
| **Docker** | 悬空镜像、未使用镜像(可逐个勾选)、停止的容器、构建缓存、**数据卷** | 谨慎 → **危险** |
| **Kubernetes** | kubectl 发现缓存、krew 插件缓存/插件、Helm 缓存与插件、**minikube** 集群与缓存、colima/lima 虚拟机、Rancher Desktop、Podman(镜像/容器/卷,未使用时提示)、nerdctl rootless 存储、k9s、Skaffold、Okteto | 安全 → **危险** |
| **研发/运维** | **OpenTofu (~/.tofu.d)** 与 Terraform (~/.terraform.d) 插件缓存与数据目录、Packer 插件、Pulumi 插件、Vagrant boxes/tmp/数据目录、Ansible tmp/collections/roles、AWS CLI 凭证缓存/SAM/CDK、Bazel 输出缓存(Windows 自动识别 `~/_bazel*`)、tflint 插件 | 安全 → 高风险 |
| **Ollama** | 本地大模型(逐个列出,可单独删除);服务未运行时自动临时启动 `ollama serve` 完成扫描后关闭(设置 `OLLAMA_HOST` 时不自动启动) | 高风险 |
| **AI 编程工具** | OpenCode、DeepSeek Harness (dsh)、Claude Code、Codex、Gemini CLI、Aider 的日志/快照/会话缓存,以及"删除全部本地数据" | 安全 → 高风险 |
| **版本管理器** | nvm / sdkman / pyenv / ~/sdk 的各版本工具链,逐个列出可单独删除,标注当前版本与"长期未使用"提示;含 nvm、sdkman 下载缓存 | 安全 / 高风险 |
| **工作区** | `node_modules`、`target`、`dist`、`build`、`.next`、`__pycache__`、`venv`、`.pytest_cache` 等 | 安全 → 高风险 |
| **编辑器** | VS Code / Cursor / Trae 的 Cache/CachedData/日志/扩展、vscode-server | 安全 / 谨慎 |
| **JetBrains 系 IDE** | IntelliJ / PyCharm / GoLand / WebStorm / Rider / Fleet 按 IDE 细分(长期未用的单独标注)、IDE 日志、**Android Studio** 缓存、Android 构建缓存 | 安全 / 谨慎 |
| **Eclipse** | p2 下载缓存、p2 共享组件池(整体删除)、用户数据 | 安全 → 高风险 |
| **.NET** | NuGet 全局包、HTTP 下载缓存、Visual Studio 组件缓存(Windows) | 安全 / 谨慎 |
| **开发应用** | Electron 下载缓存、Playwright/Puppeteer 浏览器、Hugo、Cypress、TypeScript 缓存 | 安全 / 谨慎 |
| **浏览器** | Chrome / Edge / Firefox 缓存(不碰书签密码) | 谨慎 |
| **系统** | Homebrew 缓存与旧版本、Xcode DerivedData / DeviceSupport、临时目录 | 安全 / 谨慎 |

## ⚠️ 危险等级说明 / Risk levels

| 等级 | 含义 | 示例 |
|---|---|---|
| 🟢 安全 | 纯缓存,工具自动重建,无需联网 | go-build、`__pycache__`、Daemon 日志 |
| 🟡 谨慎 | 需要重新下载或重建,耗费时间/流量 | GOMODCACHE、Gradle caches、Playwright 浏览器 |
| 🟠 高风险 | 依赖目录/镜像/模型,删除后需重新安装或拉取 | `node_modules`、Docker 镜像、Ollama 模型 |
| 🔴 危险 | **数据类,删除后不可恢复**,界面需输入 `DELETE` 确认 | Docker 数据卷 |

## 🔒 安全设计 / Safety

- 每个目录型清理目标都声明 `allowedRoot`(安全根),删除前强制校验路径必须在安全根**内部**,且不允许删除安全根本身
- 危险级目标在 API 层要求 `dangerConfirm`,前端额外要求输入 `DELETE`
- Web 服务默认只监听 `127.0.0.1`,不会暴露到局域网
- `--dry-run` / API `dryRun` 全链路只打印不执行

## 🧑‍💻 开发 / Development

```bash
make test      # 单元测试
make build     # 本地构建
make snapshot  # goreleaser 多平台构建
```

结构:

```
main.go                     # 入口 + go:embed web
internal/scan/              # 扫描引擎:类型、Provider、清理执行器、测试
  provider_*.go             # 各工具扫描器(Go/Node/Java/Python/Rust/Docker/Ollama/...)
internal/server/            # HTTP + SSE API + 静态资源
internal/cli/               # scan / clean / web 子命令
web/                        # Vue 3 前端(无需构建链,vendor 运行时已提交)
```

新增一个工具的扫描器只需实现 `Provider` 接口并在 `DefaultProviders()` 注册:
`Key() / Title() / Scan(ctx, env, emit)`。

## 📄 License

[MIT](LICENSE) © LinuxSuRen
