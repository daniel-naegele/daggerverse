package main

import (
	"context"
	"embed"
	"fmt"

	"dagger/flutter-container/internal/dagger"
)

// Helper scripts installed into /usr/local/bin of the emulator image.
//
//go:embed scripts/*
var emulatorScripts embed.FS

var emulatorScriptNames = []string{
	"android-start-emulator",
	"android-stop-emulator",
	"android-wait-for-emulator",
}

// withEmulatorScripts installs the emulator helper scripts as executables.
func withEmulatorScripts(ctr *dagger.Container) *dagger.Container {
	for _, name := range emulatorScriptNames {
		content, err := emulatorScripts.ReadFile("scripts/" + name)
		if err != nil {
			// Embedded at compile time; a missing file is a programming error.
			panic(err)
		}
		ctr = ctr.WithNewFile("/usr/local/bin/"+name, string(content),
			dagger.ContainerWithNewFileOpts{Permissions: 0o755})
	}
	return ctr
}

// Emulator returns a container with Flutter, Android tools, a pre-created AVD and
// the android-start-emulator / android-stop-emulator / android-wait-for-emulator
// helper scripts.
//
// The ABI is automatically selected based on platform:
//   - linux/amd64 → x86_64
//   - linux/arm64 → arm64-v8a
//
// When platform is not specified, the engine's native platform is used.
func (m *FlutterContainer) Emulator(
	ctx context.Context,
	// +optional
	platform dagger.Platform,
) (*dagger.Container, error) {
	if platform == "" {
		var err error
		platform, err = dag.DefaultPlatform(ctx)
		if err != nil {
			return nil, fmt.Errorf("detecting native platform: %w", err)
		}
	}
	if platform == PlatformARM64 {
		return nil, fmt.Errorf("the android emulator image does not yet support %s", platform)
	}
	abi := emulatorABI(platform)

	return androidBase(platform, m.FlutterVersion).
		WithEnvVariable("ANDROID_PLATFORM_VERSION", m.AndroidVersion).
		WithEnvVariable("ANDROID_EMULATOR_NAME", "emulator").
		WithEnvVariable("ANDROID_EMULATOR_PORT", "5554").
		WithEnvVariable("ANDROID_EMULATOR_BOOT_TIMEOUT", "600").
		WithEnvVariable("ANDROID_EMULATOR_OPTIONS", "-no-window -gpu swiftshader_indirect -no-snapshot -noaudio -no-boot-anim -camera-back none").
		WithEnvVariable("ANDROID_EMULATOR_DISABLE_ANIMATIONS", "true").
		WithEnvVariable("ANDROID_EMULATOR_DISABLE_SPELLCHECKER", "false").
		WithEnvVariable("ANDROID_EMULATOR_DISABLE_LINUX_HW_ACCEL", "auto").
		WithEnvVariable("ANDROID_EMULATOR_ENABLE_HW_KEYBOARD", "false").
		WithEnvVariable("ANDROID_EMULATOR_EXPOSE_ADB", "false").
		WithEnvVariable("PATH",
			androidHome+"/emulator:"+androidHome+"/cmdline-tools/latest/bin:"+androidHome+"/platform-tools:"+
				flutterHome+"/bin:"+flutterHome+"/bin/cache/dart-sdk/bin:/root/.pub-cache/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		// Runtime deps required even for headless emulator runs
		WithExec([]string{"sh", "-c",
			"for i in 1 2 3; do apt-get update && apt-get install -y --no-install-recommends" +
				" libpulse0 libxtst6 libnss3 libnspr4 libxss1 libasound2t64 libxkbfile1" +
				" libatk-bridge2.0-0 libgtk-3-0 libgdk-pixbuf2.0-0" +
				// socat forwards the loopback-only emulator ports (see android-start-emulator)
				" socat" +
				" && break || { [ $i -lt 3 ] && sleep 5; }; done" +
				" && rm -rf /var/lib/apt/lists/*",
		}).
		WithExec([]string{"sh", "-c",
			`sdkmanager "emulator"` +
				` && yes | sdkmanager "system-images;android-` + m.AndroidVersion + `;google_apis;` + abi + `"` +
				` && echo "no" | avdmanager create avd --force --name emulator` +
				` --abi "google_apis/` + abi + `"` +
				` --package "system-images;android-` + m.AndroidVersion + `;google_apis;` + abi + `"`,
		}).
		With(withEmulatorScripts), nil
}

// EmulatorService returns a service running a booted Android emulator that adb
// clients can connect to over the network.
//
// The service exposes the emulator console port (port) and adb port (port+1).
// Both only become reachable once the emulator finished booting, so Dagger's
// port health check implies a booted device. Bind it to a client container and
// connect with adb:
//
//	ctr.WithServiceBinding("emulator", svc).
//		WithExec([]string{"adb", "connect", "emulator:5555"})
//
// The service runs with insecure root capabilities so /dev/kvm is available
// for hardware acceleration (ANDROID_EMULATOR_DISABLE_LINUX_HW_ACCEL=auto).
// Without KVM the x86_64 emulator falls back to software emulation, which is
// very slow.
func (m *FlutterContainer) EmulatorService(
	ctx context.Context,
	// +optional
	platform dagger.Platform,
	// +optional
	// +default="emulator"
	avdName string,
	// Emulator console port; the adb port is the console port plus one.
	// +optional
	// +default=5554
	port int,
) (*dagger.Service, error) {
	emu, err := m.Emulator(ctx, platform)
	if err != nil {
		return nil, err
	}
	if avdName == "" {
		avdName = "emulator"
	}
	if port <= 0 {
		port = 5554
	}

	return emu.
		WithEnvVariable("ANDROID_EMULATOR_NAME", avdName).
		WithEnvVariable("ANDROID_EMULATOR_PORT", fmt.Sprint(port)).
		WithEnvVariable("ANDROID_EMULATOR_EXPOSE_ADB", "true").
		WithExposedPort(port, dagger.ContainerWithExposedPortOpts{Description: "emulator console"}).
		WithExposedPort(port+1, dagger.ContainerWithExposedPortOpts{Description: "adb"}).
		AsService(dagger.ContainerAsServiceOpts{
			Args:                     []string{"android-start-emulator"},
			InsecureRootCapabilities: true,
		}), nil
}
