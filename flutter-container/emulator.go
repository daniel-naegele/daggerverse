package main

import (
	"context"
	"fmt"
	"strconv"

	"dagger/flutter-container/internal/dagger"
)

// Emulator returns a container with Flutter, Android tools, and a pre-created AVD.
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
		WithEnvVariable("PATH",
			androidHome+"/emulator:"+androidHome+"/cmdline-tools/latest/bin:"+androidHome+"/platform-tools:"+
				flutterHome+"/bin:"+flutterHome+"/bin/cache/dart-sdk/bin:/root/.pub-cache/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		// Runtime deps required even for headless emulator runs
		WithExec([]string{"sh", "-c",
			"for i in 1 2 3; do apt-get update && apt-get install -y --no-install-recommends" +
				" libpulse0 libxtst6 libnss3 libnspr4 libxss1 libasound2t64 libxkbfile1" +
				" libatk-bridge2.0-0 libgtk-3-0 libgdk-pixbuf2.0-0" +
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
		WithExec([]string{"sh", "-c",
			`cat <<'EOF' > /usr/local/bin/android-wait-for-emulator
#!/usr/bin/env sh
set -eu

port="${1:-${ANDROID_EMULATOR_PORT:-5554}}"
timeout="${2:-${ANDROID_EMULATOR_BOOT_TIMEOUT:-600}}"
serial="emulator-${port}"

adb -s "${serial}" wait-for-device

elapsed=0
while [ "$(adb -s "${serial}" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" != "1" ]; do
  elapsed=$((elapsed + 1))
  if [ "${elapsed}" -ge "${timeout}" ]; then
    echo "timeout waiting for emulator boot on ${serial}" >&2
    exit 1
  fi
  sleep 1
done

adb -s "${serial}" shell input keyevent 82 >/dev/null 2>&1 || true
EOF
chmod +x /usr/local/bin/android-wait-for-emulator`,
		}), nil
}

// EmulatorService returns a service with a booted Android emulator ready for adb clients.
//
// Hardware acceleration behavior matches android-emulator-runner defaults:
//   - ANDROID_EMULATOR_DISABLE_LINUX_HW_ACCEL=auto: use KVM when /dev/kvm is accessible
//   - ANDROID_EMULATOR_OPTIONS defaults to headless CI-friendly options
func (m *FlutterContainer) EmulatorService(
	ctx context.Context,
	// +optional
	platform dagger.Platform,
	// +optional
	// +default="emulator"
	avdName string,
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

	portString := strconv.Itoa(port)
	return emu.
		WithEnvVariable("ANDROID_EMULATOR_NAME", avdName).
		WithEnvVariable("ANDROID_EMULATOR_PORT", portString).
		WithExec([]string{"sh", "-c",
			`cat <<'EOF' > /usr/local/bin/android-stop-emulator
#!/usr/bin/env sh
set -eu

port="${1:-${ANDROID_EMULATOR_PORT:-5554}}"
adb -s "emulator-${port}" emu kill >/dev/null 2>&1 || true
EOF
chmod +x /usr/local/bin/android-stop-emulator`,
		}).
		WithExec([]string{"sh", "-c",
			`cat <<'EOF' > /usr/local/bin/android-start-emulator
#!/usr/bin/env sh
set -eu

name="${ANDROID_EMULATOR_NAME:-emulator}"
port="${ANDROID_EMULATOR_PORT:-5554}"
disable_linux_hw_accel="${ANDROID_EMULATOR_DISABLE_LINUX_HW_ACCEL:-auto}"
disable_animations="${ANDROID_EMULATOR_DISABLE_ANIMATIONS:-true}"
disable_spellchecker="${ANDROID_EMULATOR_DISABLE_SPELLCHECKER:-false}"
enable_hw_keyboard="${ANDROID_EMULATOR_ENABLE_HW_KEYBOARD:-false}"
emulator_options="${ANDROID_EMULATOR_OPTIONS:--no-window -gpu swiftshader_indirect -no-snapshot -noaudio -no-boot-anim -camera-back none}"

if [ "${disable_linux_hw_accel}" = "auto" ]; then
  if [ -r /dev/kvm ] && [ -w /dev/kvm ]; then
    disable_linux_hw_accel="false"
  else
    disable_linux_hw_accel="true"
  fi
fi

if [ "${disable_linux_hw_accel}" = "true" ]; then
  emulator_options="${emulator_options} -accel off"
fi

"${ANDROID_HOME}/emulator/emulator" -port "${port}" -avd "${name}" ${emulator_options} &
emulator_pid="$!"
serial="emulator-${port}"

trap 'android-stop-emulator "${port}"; wait "${emulator_pid}" 2>/dev/null || true' INT TERM EXIT

android-wait-for-emulator "${port}"

if [ "${disable_animations}" = "true" ]; then
  adb -s "${serial}" shell settings put global window_animation_scale 0.0
  adb -s "${serial}" shell settings put global transition_animation_scale 0.0
  adb -s "${serial}" shell settings put global animator_duration_scale 0.0
fi

if [ "${disable_spellchecker}" = "true" ]; then
  adb -s "${serial}" shell settings put secure spell_checker_enabled 0
fi

if [ "${enable_hw_keyboard}" = "true" ]; then
  adb -s "${serial}" shell settings put secure show_ime_with_hard_keyboard 0
fi

wait "${emulator_pid}"
EOF
chmod +x /usr/local/bin/android-start-emulator`,
		}).
		WithEntrypoint([]string{"android-start-emulator"}).
		WithExposedPort(port).
		WithExposedPort(port + 1).
		AsService(), nil
}
