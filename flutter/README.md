# flutter

Dagger module that runs Flutter CI tasks: unit/widget tests, static analysis, format checks, Android APK/App Bundle builds (with release signing) and integration tests on an Android emulator.

Images come from the [`flutter-container`](../flutter-container) module (Flutter SDK, Android SDK and emulator stages). Every function accepts an optional pre-built image (`--flutter-image`, `--android-image`, `--emulator-image`), e.g. one of the prebuilt `ghcr.io/daniel-naegele/flutter` images, so the images do not have to be built locally.

Pub packages and Gradle files are cached in the `flutter-pub-cache` and `flutter-gradle-cache` cache volumes.

## Functions

| Function | Command | Returns |
|----------|---------|---------|
| `test` | `flutter test` | reports directory: `test-results.json`, `junit.xml`, optional `lcov.info` + `coverage-html/` |
| `analyze` | `flutter analyze` | analyzer output |
| `format` | `dart format --set-exit-if-changed` | formatter output |
| `build-apk` | `flutter build apk` | the `.apk` |
| `build-app-bundle` | `flutter build appbundle` | the `.aab` |
| `integration-test` | `flutter test integration_test` on an emulator | reports directory: `integration-test-results.json`, `junit.xml`, `boot.txt`, `accel-check.txt`, `emulator.log` |

Pass the project with `--project .` (or a path to it). `build`, `.dart_tool`, `android/.gradle` and `ios/Pods` are ignored when uploading the project.

## Usage

```sh
M=github.com/daniel-naegele/daggerverse/flutter

# Unit and widget tests with coverage, export the reports
dagger call -m $M test --project . --coverage export --path reports

# Keep the reports even if tests fail (exit code is then not propagated)
dagger call -m $M test --project . --ignore-failures export --path reports

# Static analysis (warnings are fatal by default, infos optional)
dagger call -m $M analyze --project . --fatal-infos

# Format check
dagger call -m $M format --project .

# Debug APK
dagger call -m $M build-apk --project . --mode debug export --path app-debug.apk

# Signed release App Bundle
dagger call -m $M build-app-bundle --project . \
  --build-name 1.2.3 --build-number 42 \
  --dart-defines API_URL=https://example.com \
  --keystore ./upload-keystore.jks \
  --store-password env:STORE_PASSWORD \
  --key-password env:KEY_PASSWORD \
  --key-alias upload \
  export --path app-release.aab

# Integration tests on the Android emulator
dagger call -m $M integration-test --project . export --path reports

# Use a specific Flutter version / Android API level
dagger call -m $M with-flutter-version --version 3.47.6 with-android-version --version 35 test --project .

# Use a prebuilt image instead of building one
dagger call -m $M analyze --project . --flutter-image ghcr.io/daniel-naegele/flutter:3.47.6
```

Build options (`build-apk`, `build-app-bundle`): `--mode` (`debug`, `profile`, `release`; default `release`), `--flavor`, `--target`, `--build-name`, `--build-number`, `--dart-defines KEY=VALUE,...`.

## Release signing

When `--keystore` is passed (with `--store-password` and `--key-alias`; `--key-password` defaults to the store password), the module writes `android/key.properties` before building, following the [Flutter deployment docs](https://docs.flutter.dev/deployment/android#configure-signing-in-gradle):

```properties
storePassword=...
keyPassword=...
keyAlias=upload
storeFile=/run/flutter-signing/keystore
```

Passwords are Dagger secrets: they are mounted as secret files, and `key.properties` is written to a tmpfs (symlinked into `android/`), so they never end up in a container layer or in exec arguments.

Your `android/app/build.gradle.kts` must read that file and use it for the release build type, e.g.:

```kotlin
import java.io.FileInputStream
import java.util.Properties

val keystoreProperties = Properties()
val keystorePropertiesFile = rootProject.file("key.properties")
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(FileInputStream(keystorePropertiesFile))
}

android {
    signingConfigs {
        create("release") {
            if (keystorePropertiesFile.exists()) {
                keyAlias = keystoreProperties["keyAlias"] as String
                keyPassword = keystoreProperties["keyPassword"] as String
                storeFile = keystoreProperties["storeFile"]?.let { file(it) }
                storePassword = keystoreProperties["storePassword"] as String
            }
        }
    }
    buildTypes {
        release {
            signingConfig = if (keystorePropertiesFile.exists()) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }
}
```

See [`testdata/app/android/app/build.gradle.kts`](testdata/app/android/app/build.gradle.kts) for a complete example. Without a keystore, the project's own signing configuration applies (debug builds need nothing).

## Integration tests and KVM

`integration-test` boots the AVD from the emulator image in the background of the test container (`-no-window -gpu swiftshader_indirect -no-snapshot -noaudio -no-boot-anim`), waits for `sys.boot_completed` (`--boot-timeout`, default 300 s), disables animations and runs `flutter test integration_test -d emulator-5554`. The emulator runs in the same container as the tests because it binds its adb ports to `127.0.0.1`, so a separate Dagger service would not be reachable.

The emulator needs KVM. The exec runs with insecure root capabilities, but the Dagger engine itself must have `/dev/kvm`: a **rootless** engine (e.g. rootless Podman) runs in a user namespace and drops the device ([dagger/dagger#13827](https://github.com/dagger/dagger/issues/13827)). Use a rootful engine, e.g. with Podman:

```sh
sudo systemctl enable --now podman.socket
CONTAINER_HOST=unix:///run/podman/podman.sock dagger call -m $M integration-test --project .
```

Acceleration is checked with `emulator -accel-check` before booting; the function fails with a clear error when KVM is unusable, unless `--require-kvm=false` is passed (software emulation, very slow). The emulator image is `linux/amd64` only.

## Tests

`tests/` is a separate Dagger module that runs every function against the fixture app in `testdata/app`, including a release APK signed with a throwaway keystore (verified with `apksigner verify --print-certs`) and a signed App Bundle (verified with `jarsigner -verify`):

```sh
cd tests
CONTAINER_HOST=unix:///run/podman/podman.sock dagger call all
```
