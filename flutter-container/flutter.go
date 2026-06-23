package main

import "dagger/flutter-container/internal/dagger"

// flutterBase builds the "flutter" stage: ubuntu:24.04 + system deps + Flutter SDK.
// When platform is empty, Dagger auto-detects the engine's native platform.
func flutterBase(platform dagger.Platform, flutterVersion string) *dagger.Container {
	var ctr *dagger.Container
	if platform == "" {
		ctr = dag.Container()
	} else {
		ctr = dag.Container(dagger.ContainerOpts{Platform: platform})
	}
	return ctr.
		From("ubuntu:24.04").
		WithEnvVariable("DEBIAN_FRONTEND", "noninteractive").
		WithEnvVariable("HOME", "/root").
		WithEnvVariable("LANG", "en_US.UTF-8").
		WithEnvVariable("LC_ALL", "en_US.UTF-8").
		WithEnvVariable("LANGUAGE", "en_US:en").
		WithEnvVariable("TAR_OPTIONS", "--no-same-owner").
		WithEnvVariable("FLUTTER_HOME", flutterHome).
		WithEnvVariable("FLUTTER_ROOT", flutterHome).
		WithEnvVariable("FLUTTER_VERSION", flutterVersion).
		WithEnvVariable("PATH", flutterHome+"/bin:"+flutterHome+"/bin/cache/dart-sdk/bin:/root/.pub-cache/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		WithExec([]string{"sh", "-c",
			"touch /.dockerenv" +
				" && apt-get update" +
				" && apt-get install -y --no-install-recommends" +
				" build-essential clang cmake curl git gnupg jq lcov" +
				" libgtk-3-dev libstdc++-12-dev locales ninja-build" +
				" openjdk-21-jdk openssh-client pkg-config python3" +
				" ruby-full ruby-bundler sudo unzip wget zip" +
				" && rm -rf /var/lib/apt/lists/*" +
				" && echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen" +
				" && locale-gen" +
				" && update-locale LANG=en_US.UTF-8" +
				" && echo '%sudo ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers",
		}).
		WithExec([]string{
			"git", "clone", "--depth", "1",
			"--branch", flutterVersion,
			"https://github.com/flutter/flutter.git",
			flutterHome,
		}).
		WithExec([]string{"flutter", "--version"}).
		WithExec([]string{"chown", "-R", "root:root", flutterHome})
}

// Flutter returns a container with the Flutter SDK installed (ubuntu:24.04 base).
// When platform is not specified, the engine's native platform is used.
func (m *FlutterContainer) Flutter(
	// +optional
	platform dagger.Platform,
) *dagger.Container {
	return flutterBase(platform, m.FlutterVersion)
}
