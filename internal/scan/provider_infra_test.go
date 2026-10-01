package scan

import (
	"context"
	"path/filepath"
	"testing"
)

func TestInfraProvider(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".cache")
	env := &Env{GOOS: "linux", Home: home, CacheDir: cache,
		AppData: filepath.Join(home, ".local", "share"), NoExternal: true}

	// OpenTofu: plugin cache + data dir
	mkFile(t, filepath.Join(home, ".tofu.d", "plugin-cache", "provider"), 300)
	mkFile(t, filepath.Join(home, ".tofu.d", "tofurc"), 5)
	// Terraform: plugin cache + credentials
	mkFile(t, filepath.Join(home, ".terraform.d", "plugin-cache", "provider"), 400)
	mkFile(t, filepath.Join(home, ".terraform.d", "credentials.tfrc.json"), 10)
	// Packer / Pulumi
	mkFile(t, filepath.Join(home, ".packer.d", "plugins", "builder"), 100)
	mkFile(t, filepath.Join(home, ".pulumi", "plugins", "aws"), 200)
	mkFile(t, filepath.Join(home, ".pulumi", "credentials.json"), 8)
	// Vagrant: boxes + tmp
	mkFile(t, filepath.Join(home, ".vagrant.d", "boxes", "ubuntu"), 900)
	mkFile(t, filepath.Join(home, ".vagrant.d", "tmp", "pkg"), 50)
	// Ansible
	mkFile(t, filepath.Join(home, ".ansible", "tmp", "mod"), 20)
	mkFile(t, filepath.Join(home, ".ansible", "collections", "c"), 120)
	// AWS
	mkFile(t, filepath.Join(home, ".aws", "cli", "cache", "tok.json"), 30)
	mkFile(t, filepath.Join(home, ".aws-sam", "build", "dep"), 150)
	mkFile(t, filepath.Join(home, ".cdk", "cache", "jsii"), 60)
	// Bazel (under cache dir)
	mkFile(t, filepath.Join(cache, "bazel", "_bazel_r", "out.bin"), 5000)
	// tflint
	mkFile(t, filepath.Join(home, ".tflint.d", "plugins", "p"), 40)

	var got []*Target
	(&InfraProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	byID := map[string]*Target{}
	for _, g := range got {
		byID[g.ID] = g
	}
	expect := map[string]RiskLevel{
		"infra:tofu-plugin-cache":   RiskCaution,
		"infra:tofu-all":            RiskHigh,
		"infra:tf-plugin-cache":     RiskCaution,
		"infra:tf-all":              RiskHigh,
		"infra:packer-plugins":      RiskCaution,
		"infra:pulumi-plugins":      RiskCaution,
		"infra:pulumi-all":          RiskHigh,
		"infra:vagrant-boxes":       RiskCaution,
		"infra:vagrant-tmp":         RiskSafe,
		"infra:vagrant-all":         RiskHigh,
		"infra:ansible-tmp":         RiskSafe,
		"infra:ansible-collections": RiskCaution,
		"infra:aws-cli-cache":       RiskCaution,
		"infra:aws-sam":             RiskCaution,
		"infra:aws-cdk":             RiskSafe,
		"infra:bazel-cache":         RiskCaution,
		"infra:tflint-plugins":      RiskCaution,
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
	// whole-data dirs stay inside home; sub-caches inside their tool dir
	if v := byID["infra:tofu-all"]; v.AllowedRoot != home {
		t.Errorf("tofu-all allowedRoot = %s, want %s", v.AllowedRoot, home)
	}
	if v := byID["infra:tofu-plugin-cache"]; v.AllowedRoot != filepath.Join(home, ".tofu.d") {
		t.Errorf("tofu-plugin-cache allowedRoot = %s", v.AllowedRoot)
	}
	if v := byID["infra:aws-cli-cache"]; v.Method != MethodDir {
		t.Errorf("aws cli cache should be MethodDir: %s", v.Method)
	}
	if v := byID["infra:ansible-tmp"]; v.Method != MethodDirContents {
		t.Errorf("ansible tmp should keep the dir itself: %s", v.Method)
	}
}
