# flutter-container

Dagger module that builds multi-arch Flutter Docker images — flutter base, Android SDK, and emulator — and publishes them to a container registry.

## Prebuilt images

Images are published to `ghcr.io/daniel-naegele/flutter` for every Flutter release.

| Tag | Contents |
|-----|----------|
| `<version>` | Ubuntu 24.04 + Flutter SDK |
| `<version>-android` | flutter + Android SDK + NDK |
| `<version>-emulator` | android + emulator + AVD (amd64 only) |

```sh
docker pull ghcr.io/daniel-naegele/flutter:3.41.9
docker pull ghcr.io/daniel-naegele/flutter:3.41.9-android
docker pull ghcr.io/daniel-naegele/flutter:3.41.9-emulator
```

Both `linux/amd64` and `linux/arm64` are supported (emulator: `linux/amd64` only).

## Emulator helpers and acceleration

The emulator image includes startup helpers modeled after `reactivecircus/android-emulator-runner` defaults:

- `android-start-emulator` (boot + wait + optional animation/spellchecker/keyboard tweaks, optional adb forwarding)
- `android-wait-for-emulator [port] [timeout-seconds]` (waits on `sys.boot_completed` and a responding package manager, timeout in wall-clock seconds)
- `android-stop-emulator` (graceful `adb emu kill`)

Default env values mirror the action's CI defaults:

- `ANDROID_EMULATOR_OPTIONS="-no-window -gpu swiftshader_indirect -no-snapshot -noaudio -no-boot-anim -camera-back none"`
- `ANDROID_EMULATOR_DISABLE_LINUX_HW_ACCEL=auto` (accel on when `/dev/kvm` is readable/writable and `emulator -accel-check` succeeds; otherwise `-accel off`)
- `ANDROID_EMULATOR_DISABLE_ANIMATIONS=true`
- `ANDROID_EMULATOR_DISABLE_SPELLCHECKER=false`
- `ANDROID_EMULATOR_ENABLE_HW_KEYBOARD=false`
- `ANDROID_EMULATOR_BOOT_TIMEOUT=600` (seconds)
- `ANDROID_EMULATOR_EXPOSE_ADB=false` (`true` → after boot, forward the console port and adb port from the container's non-loopback IPv4 addresses to the emulator's `127.0.0.1` listeners via `socat`)

`android-start-emulator` records the chosen acceleration mode on the device as `debug.emulator.accel` (`on`/`off`).

## Emulator service

`EmulatorService` runs a booted emulator as a Dagger service. It sets `ANDROID_EMULATOR_EXPOSE_ADB=true` and exposes the console port (`5554`) and adb port (`5555`). The ports only accept connections once the emulator has booted, so Dagger's health check implies a booted device. Clients connect with `adb connect <alias>:5555`.

The service runs with `InsecureRootCapabilities` so `/dev/kvm` is available for KVM acceleration (boot takes ~30-90 s with KVM; without it the x86_64 emulator is unusably slow). The Dagger engine itself must have access to `/dev/kvm`.

> adb is exposed without authentication to every container bound to the service. Use it for CI only.

```go
svc := dag.FlutterContainer().EmulatorService()

out, err := dag.FlutterContainer().Android().
	WithServiceBinding("emulator", svc).
	WithDirectory("/workspace", project).
	WithWorkdir("/workspace").
	WithExec([]string{"adb", "connect", "emulator:5555"}).
	WithExec([]string{"adb", "-s", "emulator:5555", "wait-for-device"}).
	WithExec([]string{"flutter", "test", "integration_test", "-d", "emulator:5555"}).
	Stdout(ctx)
```

Do not use `emu` as the service alias: `adb connect emu:...` is parsed as an emulator `<console port>,<adb port>` pair.

## Dagger usage

```sh
# Build and return the flutter base container
dagger call flutter

# Build and return the android container
dagger call android

# Build and return the emulator container (includes helper scripts)
dagger call emulator

# Start a long-running emulator service (ready for adb clients)
dagger call emulator-service

# Publish all three tags to a registry
dagger call publish \
  --registry=ghcr.io/your-org \
  --username=$GITHUB_ACTOR \
  --password=env:GITHUB_TOKEN
```

Override versions with `with-flutter-version` and `with-android-version` (Android API level):

```sh
dagger call with-flutter-version --version=3.41.9 with-android-version --version=35 emulator
```

Defaults: Flutter `3.47.6`, Android API `36`.

## Tests

`tests/` is a separate Dagger module that exercises this module (emulator scripts, emulator service via adb, version overrides):

```sh
cd tests && dagger call all
```
