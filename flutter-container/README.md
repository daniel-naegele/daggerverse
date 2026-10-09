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

- `android-start-emulator` (boot + wait + optional animation/spellchecker/keyboard tweaks)
- `android-wait-for-emulator` (waits on `sys.boot_completed`)
- `android-stop-emulator` (graceful `adb emu kill`)

Default env values mirror the action's CI defaults:

- `ANDROID_EMULATOR_OPTIONS="-no-window -gpu swiftshader_indirect -no-snapshot -noaudio -no-boot-anim -camera-back none"`
- `ANDROID_EMULATOR_DISABLE_LINUX_HW_ACCEL=auto` (`/dev/kvm` available → accel on; otherwise `-accel off`)
- `ANDROID_EMULATOR_DISABLE_ANIMATIONS=true`
- `ANDROID_EMULATOR_DISABLE_SPELLCHECKER=false`
- `ANDROID_EMULATOR_ENABLE_HW_KEYBOARD=false`

You can also run a booted emulator as a Dagger service via `emulator-service`.

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

Override versions with `--flutter-version` and `--android-version` (Android API level):

```sh
dagger call --flutter-version=3.29.3 --android-version=35 android
```

Defaults: Flutter `3.41.9`, Android API `36`.
