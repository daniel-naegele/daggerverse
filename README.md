# daggerverse

My personal collection of [Dagger](https://dagger.io) modules.

## Modules

### [flutter-container](./flutter-container/)

Builds Flutter Docker images — flutter base, Android SDK, and emulator — and publishes them as multi-arch images to `ghcr.io/daniel-naegele/flutter`. Prebuilt images are available for immediate use without having to build locally.

### [flutter](./flutter/)

Dagger CI tasks for Flutter projects: static analysis, unit tests with coverage, Dart Code Metrics, license checks, and Android release builds via Fastlane. Each task accepts an optional prebuilt image from `flutter-container` to skip the local build step.
