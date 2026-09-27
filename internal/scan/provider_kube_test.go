package scan

import (
	"context"
	"path/filepath"
	"testing"
)

func TestKubeProviderDirs(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".cache")
	env := &Env{GOOS: "linux", Home: home, CacheDir: cache,
		AppData: filepath.Join(home, ".local", "share"), NoExternal: true}

	// kubectl discovery cache (safe)
	mkFile(t, filepath.Join(home, ".kube", "cache", "discovery", "v1.json"), 50)
	// krew: cache + plugin
	mkFile(t, filepath.Join(home, ".krew", "cache", "ns.tar.gz"), 500)
	mkFile(t, filepath.Join(home, ".krew", "bin", "kubectl-ns"), 100)
	// helm cache + plugin
	mkFile(t, filepath.Join(cache, "helm", "repository", "index.yaml"), 200)
	mkFile(t, filepath.Join(home, ".local", "share", "helm", "plugins", "helm-diff"), 80)
	// minikube: cache + cluster data
	mkFile(t, filepath.Join(home, ".minikube", "cache", "v1.30.0", "kubectl"), 1000)
	mkFile(t, filepath.Join(home, ".minikube", "machines", "minikube", "disk.img"), 4000)
	// k9s
	mkFile(t, filepath.Join(home, ".k9s", "log", "k9s.log"), 30)
	// colima (high risk whole dir)
	mkFile(t, filepath.Join(home, ".colima", "default", "disk.img"), 2000)

	var got []*Target
	(&KubeProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	byID := map[string]*Target{}
	for _, g := range got {
		byID[g.ID] = g
	}
	expect := map[string]RiskLevel{
		"kube:kubectl-cache":  RiskSafe,
		"kube:krew-cache":     RiskSafe,
		"kube:krew-all":       RiskCaution,
		"kube:helm-cache":     RiskSafe,
		"kube:helm-plugins":   RiskCaution,
		"kube:minikube-cache": RiskCaution,
		"kube:minikube-all":   RiskHigh,
		"kube:k9s":            RiskSafe,
		"kube:colima":         RiskHigh,
	}
	for id, risk := range expect {
		if byID[id] == nil {
			t.Errorf("missing %s in %+v", id, ids(got))
			continue
		}
		if byID[id].Risk != risk {
			t.Errorf("%s risk = %s, want %s", id, byID[id].Risk, risk)
		}
	}
	// minikube whole-dir must stay inside home root
	if m := byID["kube:minikube-all"]; m != nil && m.AllowedRoot != home {
		t.Errorf("minikube-all allowedRoot = %s, want %s", m.AllowedRoot, home)
	}
	// podman section must be skipped entirely with NoExternal
	for _, g := range got {
		if stringsContains(g.ID, "podman") {
			t.Errorf("podman targets must not appear with NoExternal: %s", g.ID)
		}
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
