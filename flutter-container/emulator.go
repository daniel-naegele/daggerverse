package main

import (
	"context"
	"fmt"

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
		WithEnvVariable("PATH",
			androidHome+"/emulator:"+androidHome+"/cmdline-tools/latest/bin:"+androidHome+"/platform-tools:"+
				flutterHome+"/bin:"+flutterHome+"/bin/cache/dart-sdk/bin:/root/.pub-cache/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		// Runtime deps required even for headless emulator runs
		WithExec([]string{"sh", "-c",
			"apt-get update" +
				" && apt-get install -y --no-install-recommends" +
				" libpulse0 libxtst6 libnss3 libnspr4 libxss1 libasound2t64" +
				" libatk-bridge2.0-0 libgtk-3-0 libgdk-pixbuf2.0-0" +
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
			"wget -q -O /usr/local/bin/android-wait-for-emulator" +
				" https://raw.githubusercontent.com/travis-ci/travis-cookbooks/master/community-cookbooks/android-sdk/files/default/android-wait-for-emulator" +
				" && chmod +x /usr/local/bin/android-wait-for-emulator",
		}), nil
}
