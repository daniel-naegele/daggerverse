package main

import "dagger/flutter-container/internal/dagger"

// androidSdkSetup installs the Android cmdline-tools and the SDK packages listed in
// Flutter's packages.txt, accepting their licenses.
//
// On x86_64 it uses the Android CLI (`android sdk install`), which replaces the
// deprecated sdkmanager and accepts the licenses of the packages it installs
// (written to $ANDROID_HOME/licenses).
//
// The Android CLI only ships for linux x86_64 (the `android` launcher in
// cmdline-tools is an x86_64 binary that downloads the x86_64-only CLI). On
// aarch64 it cannot run, and the sdkmanager of cmdline-tools 23.0+ is a shim
// that calls it. So aarch64 keeps the Java-based sdkmanager of cmdline-tools
// 22.0 (legacyCmdlineToolsVersion), the last release that ships it. It prints a
// deprecation warning there.
//
// $1 = cmdline-tools build for x86_64, $2 = Flutter version.
const androidSdkSetup = `set -eu
case "$(uname -m)" in
  x86_64|amd64) use_android_cli=true; cmdline_tools_build="$1" ;;
  *) use_android_cli=false; cmdline_tools_build="` + legacyCmdlineToolsVersion + `" ;;
esac

wget -q "https://dl.google.com/android/repository/commandlinetools-linux-${cmdline_tools_build}_latest.zip" -O /tmp/android-cmdline-tools.zip
mkdir -p "$ANDROID_HOME/cmdline-tools"
unzip -q /tmp/android-cmdline-tools.zip -d "$ANDROID_HOME/cmdline-tools"
mv "$ANDROID_HOME/cmdline-tools/cmdline-tools" "$ANDROID_HOME/cmdline-tools/latest"
rm /tmp/android-cmdline-tools.zip
mkdir -p /root/.android
touch /root/.android/repositories.cfg

# Install packages listed in Flutter's android_sdk/packages.txt for this version
packages=$(curl -fsS "https://raw.githubusercontent.com/flutter/flutter/refs/tags/$2/engine/src/flutter/tools/android_sdk/packages.txt" | grep -E '(platforms|build-tools|platform-tools|ndk)$' | cut -d: -f1 | cut -d, -f1)

if [ "$use_android_cli" = true ]; then
  android --no-metrics --sdk="$ANDROID_HOME" sdk install $packages
else
  echo "Android CLI is not available for $(uname -m); falling back to the deprecated sdkmanager." >&2
  yes | sdkmanager --licenses
  sdkmanager --update
  yes | sdkmanager $packages
fi
`

// androidBase builds the "android" stage on top of flutterBase: adds Android cmdline-tools,
// SDK packages from Flutter's packages.txt, and runs flutter precache --android.
// When platform is empty, Dagger auto-detects the engine's native platform.
func androidBase(platform dagger.Platform, flutterVersion, cmdlineToolsVersion string) *dagger.Container {
	return flutterBase(platform, flutterVersion).
		WithEnvVariable("ANDROID_HOME", androidHome).
		WithEnvVariable("ANDROID_SDK_ROOT", androidHome).
		WithEnvVariable("PATH", androidHome+"/cmdline-tools/latest/bin:"+androidHome+"/platform-tools:"+
			flutterHome+"/bin:"+flutterHome+"/bin/cache/dart-sdk/bin:/root/.pub-cache/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		WithExec([]string{"sh", "-c", androidSdkSetup, "sh", cmdlineToolsVersion, flutterVersion}).
		// Licenses were accepted while installing the packages above. No
		// `flutter doctor --android-licenses`: it runs sdkmanager.
		WithExec([]string{"sh", "-c",
			`flutter doctor` +
				` && flutter precache --android`,
		})
}

// Android returns a container with the Flutter SDK and Android tools installed.
// When platform is not specified, the engine's native platform is used.
func (m *FlutterContainer) Android(
	// +optional
	platform dagger.Platform,
) *dagger.Container {
	return androidBase(platform, m.FlutterVersion, m.CmdlineToolsVersion)
}
