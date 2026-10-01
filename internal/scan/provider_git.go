package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitProvider analyses git repositories inside the workspace roots:
// garbage/loose objects, prunable worktree records, unused branches and
// Git LFS objects. Every clean action runs as `git -C <repo> ...` so no
// working-directory assumptions are needed.
type GitProvider struct{}

func (p *GitProvider) Key() string   { return "git" }
func (p *GitProvider) Title() string { return "Git" }

const (
	gitMaxReposPerRoot = 60
	gitMaxWalkDepth    = 6
	gitStaleBranchDays = 180
)

// protectedBranches are never offered for deletion.
var protectedBranches = map[string]bool{
	"main": true, "master": true, "develop": true, "trunk": true, "release": true,
}

func (p *GitProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	if env.NoExternal {
		return // git analysis needs the git CLI
	}
	tool, title := p.Key(), p.Title()

	for _, root := range env.Workspaces {
		if ctx.Err() != nil {
			return
		}
		repos := findGitRepos(root, gitMaxReposPerRoot)
		if len(repos) == 0 {
			continue
		}
		group := &Target{
			ID: tool + ":" + root, Tool: tool, ToolTitle: title,
			Category: "代码仓库", Title: "Git 仓库 (" + root + ")",
			Path: root, AllowedRoot: root,
			Description: fmt.Sprintf("在 %s 下发现 %d 个 Git 仓库,分析垃圾对象、worktree、已合并分支与 LFS 对象", root, len(repos)),
			Risk:        RiskCaution, Method: MethodGroup, Available: true,
		}
		for _, repo := range repos {
			if ctx.Err() != nil {
				break
			}
			if child := analyzeRepo(ctx, env, repo, tool, title); child != nil {
				group.Items = append(group.Items, child)
			}
		}
		if len(group.Items) == 0 {
			continue
		}
		sort.SliceStable(group.Items, func(i, j int) bool { return group.Items[i].Size > group.Items[j].Size })
		emit(group)
	}
}

// findGitRepos walks a workspace root collecting directories that contain
// a `.git` directory (real repos; submodule/worktree checkouts hold a
// `.git` *file* and are skipped — they belong to their parent repo).
func findGitRepos(root string, limit int) []string {
	var repos []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && name != ".git" && (skipDirs[name] || name == "node_modules") {
			return filepath.SkipDir
		}
		if depth(root, path) > gitMaxWalkDepth {
			return filepath.SkipDir
		}
		if name == ".git" {
			// only a real .git directory marks a standalone repo
			repos = append(repos, filepath.Dir(path))
			if len(repos) >= limit {
				return filepath.SkipAll
			}
			return filepath.SkipDir
		}
		return nil
	})
	return repos
}

// analyzeRepo inspects one repository and returns a group target, nil
// when nothing interesting was found.
func analyzeRepo(ctx context.Context, env *Env, repo, tool, title string) *Target {
	gitDir := filepath.Join(repo, ".git")
	dotGitSize, _ := DirSize(ctx, gitDir)

	group := &Target{
		ID: tool + ":repo:" + repo, Tool: tool, ToolTitle: title,
		Category: "代码仓库", Title: filepath.Base(repo),
		Path: repo, AllowedRoot: filepath.Dir(repo),
		Description: fmt.Sprintf(".git 占用 %s;最近活动: %s", FormatSize(dotGitSize), lastActivityDesc(gitDir)),
		Risk:        RiskCaution, Method: MethodGroup, Available: true,
	}

	// --- garbage & loose objects (git count-objects -v) ---
	if out, err := runCmd(ctx, env, 8*time.Second, "git", "-C", repo, "count-objects", "-v"); err == nil {
		fields := parseKeyValueLines(out)
		garbageKB := toInt(fields["size-garbage"])
		looseKB := toInt(fields["size"])
		nLoose := toInt(fields["count"])
		if garbageKB > 0 || nLoose > 200 {
			desc := "重打包并清理不可达对象 (git gc --prune=now)"
			if looseKB > 0 {
				desc += fmt.Sprintf(";当前松散对象 %d 个(约 %s,gc 后多数转为打包而非删除)", nLoose, FormatSize(looseKB*1024))
			}
			group.Items = append(group.Items, &Target{
				ID: tool + ":gc:" + repo, Tool: tool, ToolTitle: title,
				Category: "对象库", Title: "垃圾对象清理 (git gc)",
				Path:        filepath.Join(gitDir, "objects"),
				Description: desc, Risk: RiskCaution,
				Size:   garbageKB * 1024,
				Method: MethodCommand, Cmd: []string{"git", "-C", repo, "gc", "--prune=now"},
				Available: true,
			})
		}
	}

	// --- worktrees: stale admin records + linked working copies ---
	wtRoot := filepath.Join(gitDir, "worktrees")
	if entries, err := os.ReadDir(wtRoot); err == nil {
		var staleSize int64
		staleCount := 0
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			entryDir := filepath.Join(wtRoot, e.Name())
			gitdirFile := filepath.Join(entryDir, "gitdir")
			data, err := os.ReadFile(gitdirFile)
			if err != nil {
				continue
			}
			target := strings.TrimSpace(string(data))
			if Exists(target) {
				// live linked worktree: offer removal (HIGH, git refuses
				// when it holds uncommitted changes)
				wtDir := filepath.Dir(target)
				size, _ := DirSize(ctx, wtDir)
				note := ""
				if h := usageHint(wtDir, gitStaleBranchDays); h != "" {
					note = "⚠ " + h
				}
				group.Items = append(group.Items, &Target{
					ID: tool + ":worktree:" + wtDir, Tool: tool, ToolTitle: title,
					Category: "worktree", Title: "worktree " + filepath.Base(wtDir),
					Path:        wtDir,
					Description: "独立的 worktree 检出目录;git worktree remove 在含未提交更改时会拒绝,保护你的现场",
					Risk:        RiskHigh, Size: size,
					Method: MethodCommand, Cmd: []string{"git", "-C", repo, "worktree", "remove", wtDir},
					Available: true, Note: note,
				})
			} else {
				staleCount++
				size, _ := DirSize(ctx, entryDir)
				staleSize += size
			}
		}
		if staleCount > 0 {
			group.Items = append(group.Items, &Target{
				ID: tool + ":wt-prune:" + repo, Tool: tool, ToolTitle: title,
				Category: "worktree", Title: fmt.Sprintf("失效的 worktree 记录 (%d 个)", staleCount),
				Path:        wtRoot,
				Description: "工作目录已被手工删除的 worktree 残留管理数据 (git worktree prune)",
				Risk:        RiskSafe, Size: staleSize,
				Method: MethodCommand, Cmd: []string{"git", "-C", repo, "worktree", "prune"},
				Available: true,
			})
		}
	}

	// --- Git LFS objects ---
	if lfsDir := filepath.Join(gitDir, "lfs"); isDir(lfsDir) {
		if size, files := DirSize(ctx, lfsDir); size > 0 {
			group.Items = append(group.Items, &Target{
				ID: tool + ":lfs:" + repo, Tool: tool, ToolTitle: title,
				Category: "LFS", Title: "Git LFS 旧对象 (git lfs prune)",
				Path: lfsDir, AllowedRoot: gitDir,
				Description: "未被当前检出引用且超出保留期的 LFS 对象会被清理;此处显示 LFS 目录总体积(上限)",
				Risk:        RiskCaution, Size: size, Count: files,
				Method: MethodCommand, Cmd: []string{"git", "-C", repo, "lfs", "prune"},
				Available: true,
				Note:      "体积为上限估算,实际回收以 lfs prune 输出为准",
			})
		}
	}

	// --- branches: merged (safe -d) and stale unmerged (-D) ---
	pBranches(ctx, env, repo, tool, title, group)

	if len(group.Items) == 0 {
		return nil
	}
	sort.SliceStable(group.Items, func(i, j int) bool { return group.Items[i].Size > group.Items[j].Size })
	return group
}

func pBranches(ctx context.Context, env *Env, repo, tool, title string, group *Target) {
	// all heads with last commit date
	refs := map[string]string{} // name -> last commit date
	if out, err := runCmd(ctx, env, 6*time.Second, "git", "-C", repo, "for-each-ref",
		"refs/heads", "--format=%(refname:short)%09%(committerdate:short)"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
			if len(parts) == 2 && parts[0] != "" {
				refs[parts[0]] = parts[1]
			}
		}
	}
	if len(refs) == 0 {
		return
	}

	merged := map[string]bool{}
	current := ""
	if out, err := runCmd(ctx, env, 6*time.Second, "git", "-C", repo, "branch", "--merged"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			name := strings.TrimSpace(line)
			if name == "" {
				continue
			}
			if strings.HasPrefix(line, "*") {
				current = strings.TrimSpace(strings.TrimPrefix(line, "*"))
			}
			merged[strings.TrimSpace(strings.TrimPrefix(line, "*"))] = true
		}
	}

	staleCutoff := time.Now().AddDate(0, 0, -gitStaleBranchDays).Format("2006-01-02")

	var mergedClean, staleUnmerged []*Target
	for name := range refs {
		if protectedBranches[name] || name == current {
			continue
		}
		if merged[name] {
			mergedClean = append(mergedClean, &Target{
				ID: tool + ":branch-d:" + repo + ":" + name, Tool: tool, ToolTitle: title,
				Category: "分支", Title: "已合并分支 " + name,
				Description: "已合并进当前分支,git branch -d 删除(未合并时 git 会拒绝)",
				Risk:        RiskCaution,
				Method:      MethodCommand, Cmd: []string{"git", "-C", repo, "branch", "-d", name},
				Available: true,
			})
		} else if refs[name] < staleCutoff { // date compare works for YYYY-MM-DD
			staleUnmerged = append(staleUnmerged, &Target{
				ID: tool + ":branch-D:" + repo + ":" + name, Tool: tool, ToolTitle: title,
				Category: "分支", Title: "长期未合并分支 " + name,
				Description: fmt.Sprintf("最后提交 %s 且未合并,-D 强删后该分支上的提交将不可恢复", refs[name]),
				Risk:        RiskHigh,
				Method:      MethodCommand, Cmd: []string{"git", "-C", repo, "branch", "-D", name},
				Available: true, Note: "⚠ 已超过 " + strconv.Itoa(gitStaleBranchDays) + " 天无提交",
			})
		}
	}
	sortTargetsByID(mergedClean)
	sortTargetsByID(staleUnmerged)
	if len(mergedClean) > 15 {
		mergedClean = mergedClean[:15]
	}
	if len(staleUnmerged) > 10 {
		staleUnmerged = staleUnmerged[:10]
	}
	group.Items = append(group.Items, mergedClean...)
	group.Items = append(group.Items, staleUnmerged...)
}

func sortTargetsByID(ts []*Target) {
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].ID < ts[j].ID })
}

// parseKeyValueLines parses "key: value" output (count-objects -v).
func parseKeyValueLines(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if i := strings.IndexByte(line, ':'); i > 0 {
			m[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
		}
	}
	return m
}

func toInt(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}
