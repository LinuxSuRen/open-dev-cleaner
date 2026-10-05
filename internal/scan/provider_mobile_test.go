package scan

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMobileProviderHarmony(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, "Library", "Caches")
	env := &Env{GOOS: "darwin", Home: home, CacheDir: cache,
		AppData: filepath.Join(home, "Library", "Application Support")}

	// DevEco caches (JetBrains style), SDK, hvigor, ohpm
	mkFile(t, filepath.Join(cache, "Huawei", "DevEcoStudio6.0", "index"), 300)
	// SDK candidate must exceed the 1MB stub threshold
	mkFile(t, filepath.Join(home, "Library", "Huawei", "Sdk", "default", "openharmony", "toolchain"), 2<<20)
	mkFile(t, filepath.Join(home, ".hvigor", "cache", "module"), 100)
	mkFile(t, filepath.Join(home, ".ohpm", "cache", "p"), 50)

	var got []*Target
	(&MobileProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	var harmony *Target
	for _, g := range got {
		if g.ID == "mobile:harmony" {
			harmony = g
		}
	}
	if harmony == nil {
		t.Fatalf("harmony group missing: %+v", ids(got))
	}
	byID := map[string]*Target{}
	for _, c := range harmony.Items {
		byID[c.ID] = c
	}
	for id, risk := range map[string]RiskLevel{
		"mobile:deveco:DevEcoStudio6.0": RiskCaution,
		"mobile:deveco-sdk":             RiskHigh,
		"mobile:hvigor":                 RiskCaution,
		"mobile:ohpm":                   RiskCaution,
	} {
		if byID[id] == nil {
			t.Errorf("missing %s in %+v", id, ids(harmony.Items))
			continue
		}
		if byID[id].Risk != risk {
			t.Errorf("%s risk = %s, want %s", id, byID[id].Risk, risk)
		}
	}
	// children sorted by size desc: SDK(900) > deveco cache(300) > hvigor(100) > ohpm(50)
	sizes := make([]int64, 0, len(harmony.Items))
	for _, c := range harmony.Items {
		sizes = append(sizes, c.Size)
	}
	for i := 1; i < len(sizes); i++ {
		if sizes[i] > sizes[i-1] {
			t.Errorf("harmony children not size-desc: %v", sizes)
		}
	}
}

func TestMobileProviderAndroidAndIOS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("darwin layout in test")
	}
	home := t.TempDir()
	env := &Env{GOOS: "darwin", Home: home, CacheDir: filepath.Join(home, "Library", "Caches"),
		AppData: filepath.Join(home, "Library", "Application Support")}

	sdk := filepath.Join(home, "Library", "Android", "sdk")
	mkFile(t, filepath.Join(sdk, "system-images", "android-34", "google_apis", "arm64", "sys.img"), 800)
	mkFile(t, filepath.Join(sdk, "ndk", "27.0.1", "toolchain"), 400)
	mkFile(t, filepath.Join(sdk, "platforms", "android-34", "android.jar"), 120)
	mkFile(t, filepath.Join(home, ".android", "avd", "Pixel_7.avd", "userdata.img"), 260)
	mkFile(t, filepath.Join(home, ".android", "cache", "x"), 10)
	// iOS items
	mkFile(t, filepath.Join(home, "Library", "Caches", "CocoaPods", "pods"), 200)
	mkFile(t, filepath.Join(home, ".cocoapods", "repos", "trunk"), 500)
	mkFile(t, filepath.Join(home, "Library", "Developer", "CoreSimulator", "Devices", "set"), 30)

	var got []*Target
	(&MobileProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })

	found := map[string]*Target{}
	for _, g := range got {
		found[g.ID] = g
	}
	android, ios := found["mobile:android"], found["mobile:ios"]
	if android == nil {
		t.Fatalf("android group missing: %+v", ids(got))
	}
	abyID := map[string]*Target{}
	for _, c := range android.Items {
		abyID[c.ID] = c
	}
	for _, want := range []string{
		"mobile:sysimg:android-34", "mobile:ndk:27.0.1", "mobile:platforms",
		"mobile:avd:Pixel_7.avd", "mobile:android-cache",
	} {
		if abyID[want] == nil {
			t.Errorf("missing %s in %+v", want, ids(android.Items))
		}
	}
	if v := abyID["mobile:avd:Pixel_7.avd"]; v != nil && v.Risk != RiskHigh {
		t.Errorf("AVD must be high risk: %s", v.Risk)
	}
	if ios == nil {
		t.Fatalf("ios group missing: %+v", ids(got))
	}
	ibyID := map[string]*Target{}
	for _, c := range ios.Items {
		ibyID[c.ID] = c
	}
	if ibyID["mobile:cocoapods-cache"] == nil || ibyID["mobile:cocoapods-repos"] == nil {
		t.Errorf("cocoapods targets missing: %+v", ids(ios.Items))
	}
	if v := ibyID["mobile:simctl"]; v == nil || v.Risk != RiskSafe || v.Method != MethodCommand {
		t.Errorf("simctl target wrong: %+v", v)
	}
}

func TestWorkspacePodsRule(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "iOSApp", "Pods", "Alamofire", "lib"), 100)
	env := &Env{GOOS: "darwin", Home: root, CacheDir: filepath.Join(root, ".cache"),
		AppData: filepath.Join(root, ".local", "share"), Workspaces: []string{root}, NoExternal: true}

	var got []*Target
	(&WorkspaceProvider{}).Scan(context.Background(), env, func(t *Target) { got = append(got, t) })
	if len(got) != 1 || len(got[0].Items) != 1 {
		t.Fatalf("want Pods artifact, got %+v", got)
	}
	c := got[0].Items[0]
	if c.Risk != RiskHigh || filepath.Base(c.Path) != "Pods" {
		t.Errorf("Pods rule wrong: %+v", c)
	}
}
