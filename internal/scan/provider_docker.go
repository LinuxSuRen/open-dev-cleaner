package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DockerProvider inspects the local Docker daemon through the docker CLI,
// which works on Linux sockets, macOS/Windows Docker Desktop alike.
type DockerProvider struct{}

func (p *DockerProvider) Key() string   { return "docker" }
func (p *DockerProvider) Title() string { return "Docker" }

type dockerDFRow struct {
	Type        string `json:"Type"`
	TotalCount  int    `json:"TotalCount"`
	Active      int    `json:"Active"`
	Size        string `json:"Size"`
	Reclaimable string `json:"Reclaimable"`
}

type dockerImage struct {
	Containers string `json:"Containers"`
	Created    string `json:"Created"`
	ID         string `json:"ID"`
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	Size       string `json:"Size"`
}

func (p *DockerProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	out, err := runCmd(ctx, env, 20*time.Second, "docker", "system", "df", "--format", "{{json .}}")
	if err != nil {
		emit(p.daemonDown(ctx, env, err))
		return
	}

	// Reclaimable numbers from `docker system df`.
	imagesReclaim := int64(0)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row dockerDFRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if row.Type == "Images" {
			imagesReclaim = reclaimableBytes(row)
		}
	}

	// Dangling images: untagged leftovers, prime cleaning candidates.
	dangling := p.listImages(ctx, env, "-f", "dangling=true")
	var danglingSize int64
	for _, img := range dangling {
		danglingSize += ParseSize(img.Size)
	}

	if len(dangling) > 0 {
		emit(&Target{
			ID: tool + ":dangling", Tool: tool, ToolTitle: title,
			Category: "镜像", Title: fmt.Sprintf("悬空镜像 (dangling, %d 个)", len(dangling)),
			Path: "", Description: "无标签的中间层/残留镜像 (等效 docker image prune),删除后按需重新拉取或构建",
			Risk: RiskCaution, Size: danglingSize,
			Method: MethodCommand, Cmd: []string{"docker", "image", "prune", "-f"},
			Available: true,
			Items:     imageChildren(ctx, tool, title, dangling),
		})
	}

	// Per-image list: every image not referenced by a container.
	all := p.listImages(ctx, env)
	var unused []dockerImage
	var unusedSize int64
	for _, img := range all {
		if n, _ := strconv.Atoi(img.Containers); n == 0 {
			unused = append(unused, img)
			unusedSize += ParseSize(img.Size)
		}
	}
	if len(unused) > 0 {
		emit(&Target{
			ID: tool + ":images-unused", Tool: tool, ToolTitle: title,
			Category: "镜像", Title: fmt.Sprintf("未被容器使用的镜像 (%d 个)", len(unused)),
			Description: "当前没有任何容器引用的镜像 (等效 docker image prune -a),删除后需重新拉取/构建",
			Risk:        RiskHigh, Size: unusedSize,
			Method: MethodCommand, Cmd: []string{"docker", "image", "prune", "-a", "-f"},
			Available: true,
			Items:     imageChildren(ctx, tool, title, unused),
		})
	}

	// Stopped containers.
	if out2, err := runCmd(ctx, env, 10*time.Second, "docker", "ps", "-a", "--filter", "status=exited",
		"--filter", "status=created", "--filter", "status=dead", "--format", "{{.ID}}"); err == nil {
		if n := len(nonEmptyLines(out2)); n > 0 {
			emit(&Target{
				ID: tool + ":containers", Tool: tool, ToolTitle: title,
				Category: "容器", Title: fmt.Sprintf("已停止的容器 (%d 个)", n),
				Description: "退出/创建失败的容器及其可写层 (等效 docker container prune),数据卷不会被删除",
				Risk:        RiskCaution,
				Method:      MethodCommand, Cmd: []string{"docker", "container", "prune", "-f"},
				Available: true,
			})
		}
	}

	// Build cache.
	if out3, err := runCmd(ctx, env, 10*time.Second, "docker", "builder", "du"); err == nil {
		_ = out3 // `builder du` output is not stable across versions; use df row instead
	}
	if row, ok := p.dfRow(ctx, env, "Build Cache"); ok {
		if row.TotalCount > 0 {
			emit(&Target{
				ID: tool + ":build-cache", Tool: tool, ToolTitle: title,
				Category: "构建缓存", Title: fmt.Sprintf("镜像构建缓存 (%d 项)", row.TotalCount),
				Description: "buildx/buildkit 的中间层缓存 (等效 docker builder prune),删除后构建变慢",
				Risk:        RiskCaution, Size: reclaimableBytes(row),
				Method: MethodCommand, Cmd: []string{"docker", "builder", "prune", "-f"},
				Available: true,
			})
		}
	}

	// Volumes: user data — the dangerous one.
	if row, ok := p.dfRow(ctx, env, "Local Volumes"); ok {
		if unused := row.TotalCount - row.Active; unused > 0 {
			emit(&Target{
				ID: tool + ":volumes", Tool: tool, ToolTitle: title,
				Category: "数据卷", Title: fmt.Sprintf("未被容器使用的数据卷 (%d 个)", unused),
				Description: "数据库、上传文件等持久化数据通常在卷里,删除后不可恢复 (等效 docker volume prune)",
				Risk:        RiskDangerous, Size: reclaimableBytes(row),
				Method: MethodCommand, Cmd: []string{"docker", "volume", "prune", "-f"},
				Available: true,
			})
		}
	}

	_ = imagesReclaim
}

func (p *DockerProvider) dfRow(ctx context.Context, env *Env, typ string) (dockerDFRow, bool) {
	out, err := runCmd(ctx, env, 20*time.Second, "docker", "system", "df", "--format", "{{json .}}")
	if err != nil {
		return dockerDFRow{}, false
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var row dockerDFRow
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &row) == nil && row.Type == typ {
			return row, true
		}
	}
	return dockerDFRow{}, false
}

func (p *DockerProvider) listImages(ctx context.Context, env *Env, extra ...string) []dockerImage {
	args := append([]string{"images", "--format", "{{json .}}"}, extra...)
	out, err := runCmd(ctx, env, 15*time.Second, "docker", args...)
	if err != nil {
		return nil
	}
	var imgs []dockerImage
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var img dockerImage
		if json.Unmarshal([]byte(line), &img) == nil && img.ID != "" {
			imgs = append(imgs, img)
		}
	}
	return imgs
}

func imageChildren(ctx context.Context, tool, title string, imgs []dockerImage) []*Target {
	sorted := append([]dockerImage(nil), imgs...)
	// sort by size desc
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && ParseSize(sorted[j].Size) > ParseSize(sorted[j-1].Size); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	children := make([]*Target, 0, len(sorted))
	for _, img := range sorted {
		if n, _ := strconv.Atoi(img.Containers); n > 0 {
			continue // in use, cannot remove
		}
		name := img.Repository + ":" + img.Tag
		if img.Repository == "<none>" {
			name = "<none> (" + shortID(img.ID) + ")"
		}
		id := img.ID
		children = append(children, &Target{
			ID:   tool + ":image:" + strings.TrimPrefix(id, "sha256:"),
			Tool: tool, ToolTitle: title, Category: "镜像",
			Title: name, Description: "镜像 " + name,
			Risk: RiskHigh, Size: ParseSize(img.Size),
			Method: MethodCommand, Cmd: []string{"docker", "image", "rm", "-f", id},
			Available: true,
			Meta:      map[string]string{"id": id},
		})
	}
	return children
}

func (p *DockerProvider) daemonDown(ctx context.Context, env *Env, err error) *Target {
	note := "Docker 守护进程未运行 (" + firstLine(err.Error()) + ")。启动 Docker 后重新扫描可统计镜像、容器、缓存。"
	t := &Target{
		ID: toolUnavailable("docker"), Tool: "docker", ToolTitle: "Docker",
		Category: "镜像", Title: "Docker 未运行",
		Description: note, Risk: RiskCaution, Available: false, Note: note,
	}
	// On macOS give a hint about the Docker Desktop VM disk size.
	if env.GOOS == "darwin" {
		raw := filepath.Join(env.Home, "Library", "Containers", "com.docker.docker", "Data", "vms", "0", "data", "Docker.raw")
		if fi, statErr := statFile(raw); statErr == nil {
			t.Size = fi.Size()
			t.Note = fmt.Sprintf("Docker 未运行;虚拟磁盘 %s 占用约 %s(稀疏文件)。启动 Docker 后可通过镜像/缓存清理回收。", raw, FormatSize(fi.Size()))
			t.Description = t.Note
		}
	}
	return t
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func reclaimableBytes(row dockerDFRow) int64 {
	// Reclaimable like "4.1GB (73%)" or "0B (0%)".
	s := row.Reclaimable
	if i := strings.IndexByte(s, '('); i > 0 {
		s = s[:i]
	}
	if v := ParseSize(s); v > 0 {
		return v
	}
	return ParseSize(row.Size)
}

func toolUnavailable(key string) string { return key + ":unavailable" }
