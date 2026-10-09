package main

import (
	"context"
	"strconv"

	"dagger/flutter/internal/dagger"
)

// integrationTestScript boots the AVD from the emulator image in the
// background of the same container, waits for sys.boot_completed, disables
// animations and runs the integration tests against it.
//
// The emulator runs in the same container as the tests because it binds its
// adb/console ports to 127.0.0.1, which makes a separate Dagger service
// unreachable without extra port forwarding.
const integrationTestScript = `set -u
target="$1"; boot_timeout="$2"; require_kvm="$3"; shift 3

port="${ANDROID_EMULATOR_PORT:-5554}"
serial="emulator-${port}"
avd="${ANDROID_EMULATOR_NAME:-emulator}"
opts="${ANDROID_EMULATOR_OPTIONS:--no-window -gpu swiftshader_indirect -no-snapshot -noaudio -no-boot-anim}"
log=` + reportsDir + `/emulator.log

echo "--- emulator -accel-check"
if emulator -accel-check > ` + reportsDir + `/accel-check.txt 2>&1; then
  accel=kvm
  cat ` + reportsDir + `/accel-check.txt
else
  cat ` + reportsDir + `/accel-check.txt
  if [ "$require_kvm" = "true" ]; then
    echo "ERROR: KVM hardware acceleration is not usable in this container." >&2
    echo "Run a rootful Dagger engine that has /dev/kvm (the exec already uses insecure root capabilities)," >&2
    echo "or pass requireKvm=false to fall back to the (very slow) software emulator." >&2
    exit 2
  fi
  accel=off
  opts="$opts -accel off"
fi

adb start-server >/dev/null
start=$(date +%s)
echo "--- starting emulator: emulator -avd $avd -port $port $opts"
emulator -avd "$avd" -port "$port" $opts > "$log" 2>&1 &
emu_pid=$!

cleanup() {
  adb -s "$serial" emu kill >/dev/null 2>&1 || true
  sleep 1
  kill "$emu_pid" >/dev/null 2>&1 || true
}
trap cleanup EXIT

while :; do
  if ! kill -0 "$emu_pid" 2>/dev/null; then
    echo "ERROR: emulator process exited before boot completed. Last log lines:" >&2
    tail -n 50 "$log" >&2
    exit 3
  fi
  if [ "$(adb -s "$serial" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; then
    break
  fi
  elapsed=$(( $(date +%s) - start ))
  if [ "$elapsed" -ge "$boot_timeout" ]; then
    echo "ERROR: emulator did not finish booting within ${boot_timeout}s (accel=$accel). Last log lines:" >&2
    tail -n 50 "$log" >&2
    exit 4
  fi
  sleep 2
done
boot_seconds=$(( $(date +%s) - start ))
sdk="$(adb -s "$serial" shell getprop ro.build.version.sdk | tr -d '\r')"
echo "emulator booted in ${boot_seconds}s (accel=${accel}, sdk=${sdk})" | tee ` + reportsDir + `/boot.txt

adb -s "$serial" shell settings put global window_animation_scale 0
adb -s "$serial" shell settings put global transition_animation_scale 0
adb -s "$serial" shell settings put global animator_duration_scale 0
adb -s "$serial" shell input keyevent 82 >/dev/null 2>&1 || true

flutter test "$target" -d "$serial" --reporter expanded \
  --file-reporter "json:` + reportsDir + `/integration-test-results.json" "$@"
status=$?

if [ -s ` + reportsDir + `/integration-test-results.json ]; then
  dart pub global run junitreport:tojunit \
    --input ` + reportsDir + `/integration-test-results.json \
    --output ` + reportsDir + `/junit.xml \
    --base ` + workspaceDir + `/ || status=1
fi

exit $status
`

// IntegrationTest runs integration tests (flutter test integration_test) on an
// Android emulator booted inside the test container.
//
// The emulator needs KVM: the Dagger engine must be able to expose /dev/kvm to
// privileged execs (e.g. a rootful engine). The exec runs with insecure root
// capabilities so the emulator can use it. Acceleration is verified with
// `emulator -accel-check` before booting; without KVM the function fails unless
// requireKvm is false.
//
// Returns a reports directory with integration-test-results.json, junit.xml,
// accel-check.txt, boot.txt (boot time, acceleration, API level) and emulator.log.
func (m *Flutter) IntegrationTest(
	ctx context.Context,
	// Flutter project directory.
	// +ignore=["build", ".dart_tool", "android/.gradle", "ios/Pods"]
	project *dagger.Directory,
	// Integration test file or directory.
	// +optional
	// +default="integration_test"
	target string,
	// Product flavor to build.
	// +optional
	flavor string,
	// Compile-time constants as KEY=VALUE, passed as --dart-define.
	// +optional
	dartDefines []string,
	// Seconds to wait for the emulator to finish booting.
	// +optional
	// +default=300
	bootTimeout int,
	// Fail if KVM acceleration is unavailable instead of using the software emulator.
	// +optional
	// +default=true
	requireKvm bool,
	// Return the reports even when tests fail.
	// +optional
	ignoreFailures bool,
	// Pre-built emulator image to use instead of building one (e.g. ghcr.io/daniel-naegele/flutter:<version>-emulator).
	// +optional
	emulatorImage *dagger.Container,
) (*dagger.Directory, error) {
	if target == "" {
		target = "integration_test"
	}
	if bootTimeout <= 0 {
		bootTimeout = 300
	}
	args := []string{target, strconv.Itoa(bootTimeout), strconv.FormatBool(requireKvm)}
	if flavor != "" {
		args = append(args, "--flavor", flavor)
	}
	args = append(args, dartDefineArgs(dartDefines)...)

	ctr := withJunitReport(m.emulatorCtr(project, emulatorImage))
	return runReporting(ctx, ctr, "flutter integration test", integrationTestScript, args, ignoreFailures,
		dagger.ContainerWithExecOpts{InsecureRootCapabilities: true})
}
