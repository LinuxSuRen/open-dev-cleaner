package scan

import (
	"context"
	"path/filepath"
	"sort"
)

// MobileProvider scans mobile development toolchains: HarmonyOS
// (DevEco Studio / hvigor / ohpm), Android (SDK, NDK, AVD) and iOS
// (CocoaPods / SPM / Carthage / Xcode archives / simulators).
// Every target appears only when the corresponding directory exists.
type MobileProvider struct{}

func (p *MobileProvider) Key() string   { return "mobile" }
func (p *MobileProvider) Title() string { return "移动开发" }

func (p *MobileProvider) Scan(ctx context.Context, env *Env, emit func(*Target)) {
	tool, title := p.Key(), p.Title()

	// ================= 鸿蒙 HarmonyOS =================
	harmony := &Target{
		ID: tool + ":harmony", Tool: tool, ToolTitle: title,
		Category: "鸿蒙 DevEco", Title: "鸿蒙 DevEco Studio",
		Description: "DevEco Studio 索引缓存、Hvigor 构建缓存、ohpm 包缓存与 SDK,删除后按需重新下载/重建",
		Risk:        RiskCaution, Method: MethodGroup, Available: true,
	}
	// DevEco (JetBrains based) per-version caches
	for _, v := range globDirs(filepath.Join(env.CacheDir, "Huawei", "DevEco*")) {
		if size, files := DirSize(ctx, v); size > 0 {
			harmony.Items = append(harmony.Items, &Target{
				ID: tool + ":deveco:" + filepath.Base(v), Tool: tool, ToolTitle: title,
				Category: "鸿蒙 DevEco", Title: "DevEco Studio 缓存 (" + filepath.Base(v) + ")",
				Path: v, AllowedRoot: filepath.Dir(v),
				Description: "IDE 索引与缓存,删除后重新打开工程时重建索引(耗时)",
				Risk:        RiskCaution, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}
	// DevEco SDK: modern DevEco keeps it under command-line-tools/sdk,
	// older releases under ~/Library/Huawei/Sdk directly (the stub Sdk
	// dir only holds productConfig.json, hence the size threshold).
	sdkCandidates := []string{
		filepath.Join(env.Home, "Library", "Huawei", "command-line-tools", "sdk"),
		filepath.Join(env.Home, "Library", "Huawei", "Sdk"),
	}
	for _, sdkHuawei := range sdkCandidates {
		size, files := DirSize(ctx, sdkHuawei)
		if size <= 1<<20 {
			continue
		}
		note := ""
		if h := usageHint(sdkHuawei, 180); h != "" {
			note = "⚠ " + h
		}
		harmony.Items = append(harmony.Items, &Target{
			ID: tool + ":deveco-sdk", Tool: tool, ToolTitle: title,
			Category: "鸿蒙 DevEco", Title: "DevEco Studio SDK (" + sdkHuawei + ")",
			Path: sdkHuawei, AllowedRoot: filepath.Dir(sdkHuawei),
			Description: "鸿蒙 SDK 与工具链,删除后在 DevEco Studio 中重新下载;期间无法构建",
			Risk:        RiskHigh, Size: size, Count: files,
			Method: MethodDir, Available: true, Note: note,
		})
		break
	}
	// SDK installer downloads left after installation (~/Library/Huawei/dl)
	if dl := filepath.Join(env.Home, "Library", "Huawei", "dl"); isDir(dl) {
		if size, files := DirSize(ctx, dl); size > 0 {
			harmony.Items = append(harmony.Items, &Target{
				ID: tool + ":deveco-dl", Tool: tool, ToolTitle: title,
				Category: "鸿蒙 DevEco", Title: "SDK 安装包下载残留 (~/Library/Huawei/dl)",
				Path: dl, AllowedRoot: filepath.Dir(dl),
				Description: "DevEco 下载的 SDK 安装压缩包与解压残留,安装完成后即为垃圾,可安全删除",
				Risk:        RiskSafe, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}
	// Legacy OpenHarmony SDK
	oh := filepath.Join(env.Home, "Library", "OpenHarmony")
	if size, files := DirSize(ctx, oh); size > 0 {
		harmony.Items = append(harmony.Items, &Target{
			ID: tool + ":openharmony-sdk", Tool: tool, ToolTitle: title,
			Category: "鸿蒙 DevEco", Title: "OpenHarmony SDK (旧版路径)",
			Path: oh, AllowedRoot: filepath.Dir(oh),
			Description: "旧版 OpenHarmony SDK 目录,新 DevEco 使用 ~/Library/Huawei/Sdk 后可删除",
			Risk:        RiskCaution, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
	// hvigor build cache & ohpm package cache (all under home, cross OS)
	for _, d := range []struct {
		id, name, dir, desc string
		risk                RiskLevel
	}{
		{"hvigor", "Hvigor 构建缓存 (~/.hvigor)", filepath.Join(env.Home, ".hvigor"),
			"DevEco 构建工具的缓存与守护进程数据,删除后下次构建重建", RiskCaution},
		{"ohpm", "ohpm 包缓存 (~/.ohpm)", filepath.Join(env.Home, ".ohpm"),
			"鸿蒙包管理器下载的依赖包,删除后 ohpm install 重新下载", RiskCaution},
	} {
		if size, files := DirSize(ctx, d.dir); size > 0 {
			harmony.Items = append(harmony.Items, &Target{
				ID: tool + ":" + d.id, Tool: tool, ToolTitle: title,
				Category: "鸿蒙 DevEco", Title: d.name,
				Path: d.dir, AllowedRoot: env.Home,
				Description: d.desc, Risk: d.risk, Size: size, Count: files,
				Method: MethodDir, Available: true,
			})
		}
	}
	emitGroupIfAny(harmony, emit)

	// ================= Android =================
	android := &Target{
		ID: tool + ":android", Tool: tool, ToolTitle: title,
		Category: "安卓", Title: "Android SDK 与模拟器",
		Description: "SDK 镜像/NDK/平台与 AVD 模拟器设备,均可按需重新下载或重建",
		Risk:        RiskCaution, Method: MethodGroup, Available: true,
	}
	sdk := androidSDKDir(env)
	if sdk != "" && isDir(sdk) {
		// system images per API level (emulator images, re-downloadable)
		images := filepath.Join(sdk, "system-images")
		if levels := globDirs(filepath.Join(images, "android-*")); len(levels) > 0 {
			for _, lv := range levels {
				if size, files := DirSize(ctx, lv); size > 0 {
					android.Items = append(android.Items, &Target{
						ID: tool + ":sysimg:" + filepath.Base(lv), Tool: tool, ToolTitle: title,
						Category: "安卓", Title: "系统镜像 " + filepath.Base(lv),
						Path: lv, AllowedRoot: images,
						Description: "模拟器系统镜像,删除后 sdkmanager 重新下载",
						Risk:        RiskCaution, Size: size, Count: files,
						Method: MethodDir, Available: true,
					})
				}
			}
		}
		// NDK per version
		if ndks := globDirs(filepath.Join(sdk, "ndk", "*")); len(ndks) > 0 {
			for _, n := range ndks {
				if size, files := DirSize(ctx, n); size > 0 {
					android.Items = append(android.Items, &Target{
						ID: tool + ":ndk:" + filepath.Base(n), Tool: tool, ToolTitle: title,
						Category: "安卓", Title: "NDK " + filepath.Base(n),
						Path: n, AllowedRoot: filepath.Dir(n),
						Description: "Native 开发工具链,删除后 sdkmanager 重新下载",
						Risk:        RiskCaution, Size: size, Count: files,
						Method: MethodDir, Available: true,
					})
				}
			}
		}
		for _, d := range []struct{ id, name, rel, desc string }{
			{"platforms", "Android 平台组件 (platforms)", "platforms",
				"各版本平台 jar/框架,删除后 sdkmanager 重新下载"},
			{"build-tools", "Android 构建工具 (build-tools)", "build-tools",
				"aapt/d8 等构建工具,旧版本可清理,需要时重新下载"},
		} {
			dir := filepath.Join(sdk, filepath.FromSlash(d.rel))
			if size, files := DirSize(ctx, dir); size > 0 {
				android.Items = append(android.Items, &Target{
					ID: tool + ":" + d.id, Tool: tool, ToolTitle: title,
					Category: "安卓", Title: d.name,
					Path: dir, AllowedRoot: sdk,
					Description: d.desc, Risk: RiskCaution, Size: size, Count: files,
					Method: MethodDir, Available: true,
				})
			}
		}
	}
	// AVD emulator devices (user data!)
	avdRoot := filepath.Join(env.Home, ".android", "avd")
	if avds := globDirs(filepath.Join(avdRoot, "*.avd")); len(avds) > 0 {
		for _, a := range avds {
			if size, files := DirSize(ctx, a); size > 0 {
				name := filepath.Base(a)
				note := ""
				if h := usageHint(a, 180); h != "" {
					note = "⚠ " + h
				}
				android.Items = append(android.Items, &Target{
					ID: tool + ":avd:" + name, Tool: tool, ToolTitle: title,
					Category: "安卓", Title: "AVD 模拟器 " + name,
					Path: a, AllowedRoot: avdRoot,
					Description: "模拟器虚拟设备,含已安装应用与数据;删除后需在 Device Manager 重建",
					Risk:        RiskHigh, Size: size, Count: files,
					Method: MethodDir, Available: true, Note: note,
				})
			}
		}
	}
	if size, files := DirSize(ctx, filepath.Join(env.Home, ".android", "cache")); size > 0 {
		c := filepath.Join(env.Home, ".android", "cache")
		android.Items = append(android.Items, &Target{
			ID: tool + ":android-cache", Tool: tool, ToolTitle: title,
			Category: "安卓", Title: "Android 工具缓存 (~/.android/cache)",
			Path: c, AllowedRoot: filepath.Dir(c),
			Description: "adb/sdkmanager 等工具缓存,可安全删除",
			Risk:        RiskSafe, Size: size, Count: files,
			Method: MethodDir, Available: true,
		})
	}
	emitGroupIfAny(android, emit)
	// ================= iOS (macOS only) =================
	if env.GOOS == "darwin" {
		ios := &Target{
			ID: tool + ":ios", Tool: tool, ToolTitle: title,
			Category: "iOS", Title: "iOS 工具链",
			Description: "CocoaPods / Swift Package Manager / Carthage 缓存与 Xcode 打包归档",
			Risk:        RiskCaution, Method: MethodGroup, Available: true,
		}
		type iosDir struct {
			id, name, path, root, desc string
			risk                       RiskLevel
		}
		items := []iosDir{
			{"cocoapods-cache", "CocoaPods 缓存", filepath.Join(env.CacheDir, "CocoaPods"), env.CacheDir,
				"pod 下载缓存,删除后 pod install 重新下载", RiskSafe},
			{"cocoapods-repos", "CocoaPods Specs 镜像 (~/.cocoapods/repos)", filepath.Join(env.Home, ".cocoapods", "repos"), filepath.Join(env.Home, ".cocoapods"),
				"specs 仓库完整镜像,体积大;删除后 pod repo update 重建(耗时)", RiskCaution},
			{"spm-cache", "SwiftPM 下载缓存", filepath.Join(env.CacheDir, "org.swift.swiftpm"), env.CacheDir,
				"Swift Package Manager 依赖下载缓存,删除后 resolve 重新下载", RiskCaution},
			{"spm-repos", "SwiftPM 仓库缓存", filepath.Join(env.Home, "Library", "org.swift.swiftpm"), filepath.Join(env.Home, "Library"),
				"SPM checkout 的依赖仓库,删除后自动重新获取", RiskCaution},
			{"carthage", "Carthage 缓存", filepath.Join(env.CacheDir, "org.carthage.CarthageKit"), env.CacheDir,
				"Carthage 构建缓存,删除后 bootstrap 重建", RiskSafe},
			{"archives", "⚠ Xcode 打包归档 (Archives)", filepath.Join(env.Home, "Library", "Developer", "Xcode", "Archives"), filepath.Join(env.Home, "Library", "Developer", "Xcode"),
				"历史打包归档(ipa/dsym),删除后不可恢复,请确认无需留存", RiskHigh},
		}
		for _, it := range items {
			if size, files := DirSize(ctx, it.path); size > 0 {
				ios.Items = append(ios.Items, &Target{
					ID: tool + ":" + it.id, Tool: tool, ToolTitle: title,
					Category: "iOS", Title: it.name,
					Path: it.path, AllowedRoot: it.root,
					Description: it.desc, Risk: it.risk, Size: size, Count: files,
					Method: MethodDir, Available: true,
				})
			}
		}
		// unavailable simulators via simctl (safe command)
		if devices := filepath.Join(env.Home, "Library", "Developer", "CoreSimulator", "Devices"); isDir(devices) {
			ios.Items = append(ios.Items, &Target{
				ID: tool + ":simctl", Tool: tool, ToolTitle: title,
				Category: "iOS", Title: "失效的模拟器设备 (simctl)",
				Path:        devices,
				Description: "清理已不可用模拟器的残留数据 (xcrun simctl delete unavailable);可用设备不受影响",
				Risk:        RiskSafe,
				Method:      MethodCommand, Cmd: []string{"xcrun", "simctl", "delete", "unavailable"},
				Available: true,
			})
		}
		emitGroupIfAny(ios, emit)
	}
}

// androidSDKDir resolves the Android SDK location per OS.
func androidSDKDir(env *Env) string {
	switch env.GOOS {
	case "darwin":
		return filepath.Join(env.Home, "Library", "Android", "sdk")
	case "windows":
		return filepath.Join(env.CacheDir, "Android", "Sdk")
	default:
		return filepath.Join(env.Home, "Android", "Sdk")
	}
}

func emitGroupIfAny(group *Target, emit func(*Target)) {
	if len(group.Items) == 0 {
		return
	}
	group.Risk = group.Items[0].Risk
	for _, c := range group.Items {
		if RiskOrder(c.Risk) > RiskOrder(group.Risk) {
			group.Risk = c.Risk
		}
	}
	sort.SliceStable(group.Items, func(i, j int) bool { return group.Items[i].Size > group.Items[j].Size })
	emit(group)
}
