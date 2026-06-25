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

## Dagger usage

```sh
# Build and return the flutter base container
dagger call flutter

# Build and return the android container
dagger call android

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
