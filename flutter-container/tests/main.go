// Tests for the flutter-container module.

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	// Older stable Flutter release used to verify WithFlutterVersion.
	olderFlutterVersion = "3.41.9"
	// Non-default Android API level used to verify WithAndroidVersion.
	olderAndroidVersion = "35"
)

type Tests struct{}

// All executes all tests.
func (m *Tests) All(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error { return m.EmulatorScripts(ctx) })
	eg.Go(func() error { return m.EmulatorService(ctx) })
	eg.Go(func() error { return m.VersionOverride(ctx) })
	eg.Go(func() error { return m.AndroidToolchain(ctx) })

	return eg.Wait()
}

// EmulatorScripts checks that the emulator image ships the helper scripts and
// socat, and that android-wait-for-emulator times out after wall-clock seconds.
func (m *Tests) EmulatorScripts(ctx context.Context) error {
	_, err := dag.FlutterContainer().Emulator().
		WithEnvVariable("CACHE_BUSTER", time.Now().String()).
		WithExec([]string{"sh", "-euc", `
for bin in android-start-emulator android-stop-emulator android-wait-for-emulator socat; do
  path="$(command -v "$bin")" || { echo "missing $bin" >&2; exit 1; }
  [ -x "$path" ] || { echo "$path is not executable" >&2; exit 1; }
  echo "found $path"
done

# No emulator is running: waiting with a 3 second timeout must fail quickly.
start="$(date +%s)"
if android-wait-for-emulator 5600 3; then
  echo "android-wait-for-emulator unexpectedly succeeded" >&2
  exit 1
fi
elapsed=$(($(date +%s) - start))
echo "android-wait-for-emulator failed after ${elapsed}s"
[ "$elapsed" -lt 30 ] || { echo "timeout is not measured in seconds" >&2; exit 1; }
`}).
		Sync(ctx)
	return err
}

// EmulatorService boots the emulator service, connects to it over the network
// with adb and checks that it booted with hardware acceleration.
func (m *Tests) EmulatorService(ctx context.Context) error {
	svc := dag.FlutterContainer().EmulatorService()

	// Note: the alias must not be "emu", since adb parses "emu:..." in
	// `adb connect` as a <console port>,<adb port> pair.

	out, err := dag.FlutterContainer().Android().
		WithServiceBinding("emulator", svc).
		WithEnvVariable("CACHE_BUSTER", time.Now().String()).
		WithExec([]string{"sh", "-euc", `
adb start-server
for i in $(seq 1 30); do
  if adb connect emulator:5555 | tee /dev/stderr | grep -q "connected to"; then
    break
  fi
  sleep 2
done
timeout 120 adb -s emulator:5555 wait-for-device
adb devices -l
echo "boot_completed=$(adb -s emulator:5555 shell getprop sys.boot_completed | tr -d '\r')"
echo "accel=$(adb -s emulator:5555 shell getprop debug.emulator.accel | tr -d '\r')"
echo "sdk=$(adb -s emulator:5555 shell getprop ro.build.version.sdk | tr -d '\r')"
flutter devices
`}).
		Stdout(ctx)
	if err != nil {
		return err
	}

	for _, want := range []string{"boot_completed=1", "accel=on", "• emulator:5555"} {
		if !strings.Contains(out, want) {
			return fmt.Errorf("expected %q in output:\n%s", want, out)
		}
	}
	return nil
}

// VersionOverride checks that WithFlutterVersion and WithAndroidVersion are
// honored.
func (m *Tests) VersionOverride(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		out, err := dag.FlutterContainer().
			WithFlutterVersion(olderFlutterVersion).
			Flutter().
			WithExec([]string{"flutter", "--version"}).
			Stdout(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(out, "Flutter "+olderFlutterVersion) {
			return fmt.Errorf("expected Flutter %s, got:\n%s", olderFlutterVersion, out)
		}
		return nil
	})

	eg.Go(func() error {
		emu := dag.FlutterContainer().
			WithAndroidVersion(olderAndroidVersion).
			Emulator()

		env, err := emu.EnvVariable(ctx, "ANDROID_PLATFORM_VERSION")
		if err != nil {
			return err
		}
		if env != olderAndroidVersion {
			return fmt.Errorf("expected ANDROID_PLATFORM_VERSION=%s, got %q", olderAndroidVersion, env)
		}

		cfg, err := emu.File("/root/.android/avd/emulator.avd/config.ini").Contents(ctx)
		if err != nil {
			return err
		}
		want := "system-images/android-" + olderAndroidVersion + "/"
		if !strings.Contains(cfg, want) {
			return fmt.Errorf("expected AVD config to reference %q, got:\n%s", want, cfg)
		}
		return nil
	})

	return eg.Wait()
}

// AndroidToolchain checks that the android image ships the Android CLI, keeps
// sdkmanager on PATH for Flutter and Gradle, has the SDK licenses accepted and
// that flutter doctor reports a working Android toolchain.
func (m *Tests) AndroidToolchain(ctx context.Context) error {
	out, err := dag.FlutterContainer().Android().
		WithEnvVariable("CACHE_BUSTER", time.Now().String()).
		WithExec([]string{"sh", "-euc", `
android --no-metrics --version
command -v sdkmanager
command -v avdmanager
test -s "$ANDROID_HOME/licenses/android-sdk-license"
android --no-metrics --sdk="$ANDROID_HOME" sdk list
flutter doctor -v
`}).
		Stdout(ctx)
	if err != nil {
		return err
	}

	for _, want := range []string{
		"platform-tools",
		"build-tools/",
		"ndk/",
		"[✓] Android toolchain",
		"All Android licenses accepted.",
	} {
		if !strings.Contains(out, want) {
			return fmt.Errorf("expected %q in output:\n%s", want, out)
		}
	}
	return nil
}
