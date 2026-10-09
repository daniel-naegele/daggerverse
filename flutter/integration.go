package main

import (
	"context"
	"strconv"

	"dagger/flutter/internal/dagger"
)

// integrationTestScript prebuilds the debug APK, boots the AVD with
// flutter-container's android-start-emulator in the background of the same
// container and runs the integration tests against it.
//
// Emulator startup (KVM detection via -accel-check, boot and package manager
// wait, animation settings, debug.emulator.accel prop) is entirely handled by
// the emulator image's scripts and ANDROID_EMULATOR_* env; this script only
// waits for its "emulator ready" line, enforces requireKvm and collects reports.
//
// The emulator runs in the same exec as the tests (rather than as an
// EmulatorService) so adb, the VM service forwarding and the emulator log stay
// local, and every IntegrationTest call gets its own emulator.
const integrationTestScript = `set -u
require_kvm="$1"; target="$2"; test_timeout="$3"; attempts="$4"; verbose="$5"; shift 5

port="${ANDROID_EMULATOR_PORT:-5554}"
serial="emulator-${port}"
log=` + reportsDir + `/emulator.log

diagnose() {
  echo "--- adb devices" >&2
  adb devices -l >&2 || true
  echo "--- emulator log (tail)" >&2
  tail -n 60 "$log" >&2 || true
}

# Build before booting so Gradle does not compete with the emulator for CPU and
# memory; flutter test then only recompiles the Dart test entry point.
echo "--- prebuilding debug apk"
if ! flutter build apk --debug "$@"; then
  echo "ERROR: debug build failed" >&2
  exit 1
fi

start=$(date +%s)
android-start-emulator > "$log" 2>&1 &
starter=$!
trap 'kill "$starter" 2>/dev/null; wait "$starter" 2>/dev/null' EXIT

while ! grep -q "emulator ready" "$log"; do
  if [ "$require_kvm" = "true" ] && grep -q "hardware acceleration off" "$log"; then
    echo "ERROR: KVM hardware acceleration is not usable in this container." >&2
    grep "accel-check" "$log" >&2 || true
    echo "The Dagger engine must expose /dev/kvm (e.g. a rootful engine); pass requireKvm=false to use the (very slow) software emulator." >&2
    exit 2
  fi
  if ! kill -0 "$starter" 2>/dev/null; then
    echo "ERROR: emulator failed to start or boot (ANDROID_EMULATOR_BOOT_TIMEOUT=${ANDROID_EMULATOR_BOOT_TIMEOUT:-600}s)." >&2
    diagnose
    exit 3
  fi
  sleep 1
done

grep "accel-check" "$log" > ` + reportsDir + `/accel-check.txt || true
accel="$(adb -s "$serial" shell getprop debug.emulator.accel | tr -d '\r')"
sdk="$(adb -s "$serial" shell getprop ro.build.version.sdk | tr -d '\r')"
echo "emulator booted in $(( $(date +%s) - start ))s (accel=${accel}, sdk=${sdk})" | tee ` + reportsDir + `/boot.txt
if [ "$require_kvm" = "true" ] && [ "$accel" != "on" ]; then
  echo "ERROR: emulator is not hardware accelerated (debug.emulator.accel=${accel})" >&2
  exit 2
fi

vflag=""
if [ "$verbose" = "true" ]; then vflag="-v"; fi

# Infrastructure failures (VM service/DDS connection lost, device offline, a
# hang while waiting for the app's VM service) are retried; test failures are not.
attempt=1
while :; do
  tlog=` + reportsDir + `/flutter-test-attempt-${attempt}.log
  echo "--- flutter test (attempt ${attempt}/${attempts}, timeout ${test_timeout}s)"
  rm -f ` + reportsDir + `/integration-test-results.json
  timeout -k 30 "$test_timeout" flutter test "$target" -d "$serial" $vflag --reporter expanded \
    --file-reporter "json:` + reportsDir + `/integration-test-results.json" "$@" > "$tlog" 2>&1
  status=$?
  cat "$tlog"
  [ "$status" -eq 0 ] && break

  infra=false
  if [ "$status" -eq 124 ] || [ "$status" -eq 137 ]; then
    echo "flutter test timed out after ${test_timeout}s" >&2
    infra=true
  elif grep -qE "Failed to start Dart Development Service|Service has disappeared|device offline|Lost connection to device|Unable to connect to VM service|ProcessException" "$tlog"; then
    infra=true
  fi
  diagnose
  if [ "$infra" != "true" ] || [ "$attempt" -ge "$attempts" ]; then
    break
  fi
  attempt=$((attempt + 1))
  echo "--- infrastructure failure, retrying after the device is ready again"
  android-wait-for-emulator "$port" 300 || break
done

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
// The debug APK is built first, then the emulator image's android-start-emulator
// boots the AVD (configured through the image's ANDROID_EMULATOR_* env) and the
// tests run against emulator-5554.
//
// The emulator needs KVM: the Dagger engine must be able to expose /dev/kvm to
// privileged execs (e.g. a rootful engine). The exec runs with insecure root
// capabilities so the emulator can use it. android-start-emulator checks
// acceleration with `emulator -accel-check`; without KVM this function fails
// unless requireKvm is false.
//
// Returns a reports directory with integration-test-results.json, junit.xml,
// accel-check.txt, boot.txt (boot time, accel on/off, API level) and emulator.log.
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
	// Seconds after which one flutter test attempt is aborted.
	// +optional
	// +default=1200
	testTimeout int,
	// Total flutter test attempts; only infrastructure failures (lost VM service,
	// device offline, timeout) are retried, never failing tests.
	// +optional
	// +default=2
	attempts int,
	// Run flutter test with -v (verbose tool logs).
	// +optional
	flutterVerbose bool,
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
	if testTimeout <= 0 {
		testTimeout = 1200
	}
	if attempts <= 0 {
		attempts = 2
	}
	args := []string{strconv.FormatBool(requireKvm), target, strconv.Itoa(testTimeout), strconv.Itoa(attempts), strconv.FormatBool(flutterVerbose)}
	if flavor != "" {
		args = append(args, "--flavor", flavor)
	}
	args = append(args, dartDefineArgs(dartDefines)...)

	ctr := withJunitReport(m.emulatorCtr(project, emulatorImage)).
		WithEnvVariable("ANDROID_EMULATOR_BOOT_TIMEOUT", strconv.Itoa(bootTimeout))
	return runReporting(ctx, ctr, "flutter integration test", integrationTestScript, args, ignoreFailures,
		dagger.ContainerWithExecOpts{InsecureRootCapabilities: true})
}
