package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestGitProvider builds a fake workspace containing a repo with a stale
// worktree record, a live worktree, LFS objects, merged and stale
// branches — driven by a fake `git` script on PATH.
func TestGitProvider(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script based test")
	}
	home := t.TempDir()
	bin := t.TempDir()
	ws := filepath.Join(home, "Workspace")
	repo := filepath.Join(ws, "demo")
	gitDir := filepath.Join(repo, ".git")

	// repo skeleton
	mkFile(t, filepath.Join(gitDir, "objects", "pack", "p.pack"), 4096)
	mkFile(t, filepath.Join(gitDir, "HEAD"), 16)
	// LFS objects
	mkFile(t, filepath.Join(gitDir, "lfs", "objects", "ab", "oid"), 8192)
	// stale worktree record: gitdir points to a removed directory
	stale := filepath.Join(gitDir, "worktrees", "old-wt")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "gitdir"),
		[]byte(filepath.Join(home, "gone", "wt", ".git")), 0o644); err != nil {
		t.Fatal(err)
	}
	// live worktree
	liveWT := filepath.Join(home, "live-wt")
	mkFile(t, filepath.Join(liveWT, ".git"), 40)
	mkFile(t, filepath.Join(liveWT, "src", "main.go"), 2048)
	liveAdmin := filepath.Join(gitDir, "worktrees", "live-wt")
	if err := os.MkdirAll(liveAdmin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveAdmin, "gitdir"),
		[]byte(filepath.Join(liveWT, ".git")), 0o644); err != nil {
		t.Fatal(err)
	}
	// second repo without anything interesting → must not be reported
	other := filepath.Join(ws, "plain")
	mkFile(t, filepath.Join(other, ".git", "HEAD"), 16)

	// fake git CLI: rich answers only for the "demo" repo
	script := "#!/bin/sh\n" +
		"repo=\"$2\"; shift 2\n" +
		"case \"$repo\" in\n" +
		"  *demo)\n" +
		"    case \"$1\" in\n" +
		"      count-objects) printf 'count: 900\\nsize: 40\\nin-pack: 10\\npacks: 1\\nsize-pack: 900\\ngarbage: 4\\nsize-garbage: 256\\n' ;;\n" +
		"      for-each-ref) printf 'main\\t2026-09-01\\nfeat/merged-x\\t2026-08-01\\nfeat/old-stale\\t2024-01-01\\nfeat/active\\t2026-09-20\\n' ;;\n" +
		"      branch) printf '* main\\n  feat/merged-x\\n' ;;\n" +
		"    esac ;;\n" +
		"  *)\n" +
		"    case \"$1\" in\n" +
		"      count-objects) printf 'count: 3\\nsize: 1\\nin-pack: 5\\npacks: 1\\nsize-pack: 10\\n' ;;\n" +
		"    esac ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	env := &Env{GOOS: runtime.GOOS, Home: home, CacheDir: filepath.Join(home, ".cache"),
		AppData: filepath.Join(home, ".local", "share"), Workspaces: []string{ws}}

	var got []*Target
	(&GitProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	if len(got) != 1 {
		t.Fatalf("want one workspace group, got %+v", ids(got))
	}
	// engine convention: groups carry the summed size of children
	for _, g := range got {
		if len(g.Items) == 0 {
			continue
		}
		g.Size = g.SumSize()
	}
	group := got[0]
	if len(group.Items) != 1 {
		t.Fatalf("plain repo should be skipped, want 1 repo child, got %+v", ids(group.Items))
	}
	repoGroup := group.Items[0]
	if repoGroup.Title != "demo" {
		t.Fatalf("unexpected repo group: %+v", repoGroup)
	}

	byID := map[string]*Target{}
	for _, c := range repoGroup.Items {
		byID[c.ID] = c
	}
	checks := []struct {
		id   string
		risk RiskLevel
		cmd  string
	}{
		{"git:gc:" + repo, RiskCaution, "git -C " + repo + " gc --prune=now"},
		{"git:lfs:" + repo, RiskCaution, "git -C " + repo + " lfs prune"},
		{"git:wt-prune:" + repo, RiskSafe, "git -C " + repo + " worktree prune"},
		{"git:worktree:" + liveWT, RiskHigh, "git -C " + repo + " worktree remove " + liveWT},
		{"git:branch-d:" + repo + ":feat/merged-x", RiskCaution, "git -C " + repo + " branch -d feat/merged-x"},
		{"git:branch-D:" + repo + ":feat/old-stale", RiskHigh, "git -C " + repo + " branch -D feat/old-stale"},
	}
	for _, c := range checks {
		got := byID[c.id]
		if got == nil {
			t.Errorf("missing %s in %+v", c.id, ids(repoGroup.Items))
			continue
		}
		if got.Risk != c.risk {
			t.Errorf("%s risk = %s, want %s", c.id, got.Risk, c.risk)
		}
		if strings.Join(got.Cmd, " ") != c.cmd {
			t.Errorf("%s cmd = %v, want %q", c.id, got.Cmd, c.cmd)
		}
	}
	// protected/current/active branches must NOT appear
	for _, c := range repoGroup.Items {
		if strings.Contains(c.ID, ":main") || strings.Contains(c.ID, "feat/active") {
			t.Errorf("protected or active branch offered: %s", c.ID)
		}
	}
	// stale worktree prune target counts exactly one record
	if w := byID["git:wt-prune:"+repo]; w != nil && !strings.Contains(w.Title, "1 个") {
		t.Errorf("stale worktree count wrong: %s", w.Title)
	}
}

func TestFindGitReposSkipsWorktreeFiles(t *testing.T) {
	ws := t.TempDir()
	// real repo
	mkFile(t, filepath.Join(ws, "real", ".git", "HEAD"), 4)
	// submodule-style checkout: .git is a FILE
	if err := os.MkdirAll(filepath.Join(ws, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "sub", ".git"), []byte("gitdir: ../real/.git"), 0o644); err != nil {
		t.Fatal(err)
	}
	repos := findGitRepos(ws, 10)
	if len(repos) != 1 || filepath.Base(repos[0]) != "real" {
		t.Fatalf("want only the real repo, got %+v", repos)
	}
}
