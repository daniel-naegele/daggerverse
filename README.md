# daggerverse

My personal collection of [Dagger](https://dagger.io) modules.

## Modules

### [flutter-container](./flutter-container/)

Builds Flutter Docker images — flutter base, Android SDK, and emulator — and publishes them as multi-arch images to `ghcr.io/daniel-naegele/flutter`. Prebuilt images are available for immediate use without having to build locally.

### [flutter](./flutter/)

Dagger CI tasks for Flutter projects: unit/widget tests (JUnit + coverage), `flutter analyze`, format checks, Android APK/App Bundle builds with release signing, and integration tests on a KVM-accelerated Android emulator. Each task accepts an optional prebuilt image from `flutter-container` to skip the local build step.

### [quarto](./quarto/)

Renders [Quarto](https://quarto.org) documentation projects. `Render` wraps `quarto render` directly for full control; `BuildDocs` is a sensible-default convenience — render a project's `docs/` subdirectory (or another via `docsDir`) and get the output back — callable directly from CI with no local Dagger module required:

```bash
dagger call -m github.com/daniel-naegele/daggerverse/quarto build-docs --source . export --path public
```

### [sops](./sops/)

Minimal multi-arch image with [sops](https://github.com/getsops/sops), [age](https://github.com/FiloSottile/age) and [ssh-to-age](https://github.com/Mic92/ssh-to-age) (published as `ghcr.io/daniel-naegele/sops`), plus Dagger functions to encrypt with public keys only, decrypt with age or SSH ed25519 keys, and run a sops keyservice so other containers can decrypt without holding the key:

```bash
dagger call -m github.com/daniel-naegele/daggerverse/sops decrypt --file secrets.enc.yaml --age-key env:SOPS_AGE_KEY export --path secrets.yaml
```

### [codemagic-cli-tools](./codemagic-cli-tools/)

Minimal multi-arch image with [Codemagic CLI tools](https://github.com/codemagic-ci-cd/cli-tools), a jlink'ed Java runtime, bundletool and git, published to `ghcr.io/daniel-naegele/codemagic-cli-tools`. Typed Dagger functions for Google Play publishing, App Bundle inspection/signing/APK generation, keystores and changelogs:

```bash
dagger call -m github.com/daniel-naegele/daggerverse/codemagic-cli-tools \
  google-play --credentials env:GOOGLE_PLAY_CREDENTIALS publish-bundle --bundle app.aab --track internal
```
