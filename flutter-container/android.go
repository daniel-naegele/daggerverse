package main

import "dagger/flutter-container/internal/dagger"

// androidBase builds the "android" stage on top of flutterBase: adds Android cmdline-tools,
// SDK packages from Flutter's packages.txt, and runs flutter precache --android.
// When platform is empty, Dagger auto-detects the engine's native platform.
func androidBase(platform dagger.Platform, flutterVersion string) *dagger.Container {
	return flutterBase(platform, flutterVersion).
		WithEnvVariable("ANDROID_HOME", androidHome).
		WithEnvVariable("ANDROID_SDK_ROOT", androidHome).
		WithEnvVariable("PATH", androidHome+"/cmdline-tools/latest/bin:"+androidHome+"/platform-tools:"+
			flutterHome+"/bin:"+flutterHome+"/bin/cache/dart-sdk/bin:/root/.pub-cache/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		WithExec([]string{"sh", "-c",
			// Download Android cmdline-tools by scraping the current URL from developer.android.com
			`command_line_tools_url="$(curl -s https://developer.android.com/studio/ | grep -o 'https://dl.google.com/android/repository/commandlinetools-linux-[0-9]*_latest.zip')"` +
				` && wget -q "$command_line_tools_url" -O android-cmdline-tools.zip` +
				` && mkdir -p ` + androidHome + `/cmdline-tools/` +
				` && unzip -q android-cmdline-tools.zip -d ` + androidHome + `/cmdline-tools/` +
				` && mv ` + androidHome + `/cmdline-tools/cmdline-tools ` + androidHome + `/cmdline-tools/latest` +
				` && rm android-cmdline-tools.zip`,
		}).
		WithExec([]string{"sh", "-c",
			` yes | sdkmanager --licenses` +
				` && mkdir -p /root/.android` +
				` && touch /root/.android/repositories.cfg`,
		}).
		WithExec([]string{"sh", "-c",
			`sdkmanager --update` +
				// Install packages listed in Flutter's android_sdk/packages.txt for this version
				` && packages=$(curl -s https://raw.githubusercontent.com/flutter/flutter/refs/tags/` + flutterVersion + `/engine/src/flutter/tools/android_sdk/packages.txt | grep -E '(platforms|build-tools|platform-tools|ndk)$' | cut -d: -f1 | cut -d, -f1)` +
				` && yes | sdkmanager $packages`,
		}).
		WithExec([]string{"sh", "-c",
			`yes | flutter doctor --android-licenses` +
				` && flutter doctor` +
				` && flutter precache --android`,
		})
}

// Android returns a container with the Flutter SDK and Android tools installed.
// When platform is not specified, the engine's native platform is used.
func (m *FlutterContainer) Android(
	// +optional
	platform dagger.Platform,
) *dagger.Container {
	return androidBase(platform, m.FlutterVersion)
}
