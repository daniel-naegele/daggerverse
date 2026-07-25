# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository structure

A monorepo of [Dagger](https://dagger.io) modules, each in its own subdirectory with its own `dagger.json`, Go module, and generated SDK code.

| Directory | Module name | Purpose |
|-----------|-------------|---------|
| `flutter-container/` | `flutter-container` | Main module — builds Flutter Docker images and runs Flutter CI tasks |
| `flutter/` | `flutter` | Placeholder scaffold (not yet implemented) |
| `quarto/` | `quarto` | Renders Quarto documentation projects; `BuildDocs` is the sensible-default entry point for direct `-m` invocation from consumers with no local Dagger module |

## Common commands

All Dagger CLI commands must be run from inside the module directory (e.g. `flutter-container/`).

```bash
# List all callable functions
dagger call --help

# Call a function (example)
dagger call flutter-image --flutter-version=3.29.3

# Regenerate dagger.gen.go after changing the module's exported API
dagger develop
```

Go is only used as the Dagger SDK language — there is no standalone Go binary to build or test directly.

## flutter-container module architecture

Module type: `FlutterContainer`. Default versions: Flutter `3.41.9`, Android `36`. Use `WithFlutterVersion` / `WithAndroidVersion` to override (Dagger `WithSomething` chaining pattern).

**File-per-image-stage layout:**
- `flutter.go` — `Flutter(platform?)` method + internal `flutterBase(platform, version)`: ubuntu:24.04 + system deps + Flutter SDK cloned from GitHub
- `android.go` — `Android(platform?)` method + internal `androidBase(platform, version)`: extends flutter, adds Android cmdline-tools + SDK packages from Flutter's `packages.txt`
- `emulator.go` — `Emulator(ctx, platform?)` method: extends android, installs emulator + AVD; ABI auto-selected (`x86_64` for amd64, `arm64-v8a` for arm64)
- `publish.go` — `Publish(ctx, registry, username, password, platforms?)`: pushes multi-arch manifests for flutter, android, and emulator tags
- `constants.go` — `PlatformAMD64`, `PlatformARM64`, `abiAMD64`, `abiARM64`, `emulatorABI(platform)` helper
- `main.go` — struct, `New()`, `WithFlutterVersion`, `WithAndroidVersion`, `parsePlatforms`, `imageDigest`

When `platform == ""`, `flutterBase`/`androidBase` call `dag.Container()` without a `Platform` option — Dagger uses the engine host's native platform. `Emulator` calls `dag.DefaultPlatform(ctx)` to resolve the ABI.

## flutter module architecture

Module type: `Flutter`. Same default versions and `With*` pattern as flutter-container.

**File-per-CI-task layout:**
- `analyze.go` — `Analyze(ctx, project, netrcToken?, flutterImage?)`: validates analysis preset, returns JUnit XML
- `test.go` — `Test(ctx, project, netrcToken?, flutterImage?)`: runs coverage, returns JUnit XML + lcov HTML
- `dcm.go` — `Dcm(ctx, project, dcmEmail, dcmCiKey, dcmVersion?, dcmFatalLevel?, netrcToken?, flutterImage?)`: dart code metrics, returns GitLab Code Quality JSON
- `license.go` — `LicenseCheck(ctx, project, netrcToken?, flutterImage?)`: runs `license_checker` (advisory if config absent)
- `build.go` — `BuildAndroid(ctx, project, keystoreFile, gradleProperties, pipelineIid, netrcToken?, androidImage?)`: Fastlane build, returns `.aab`
- `main.go` — struct, `New()`, `With*`, internal helpers: `flutterBase`, `androidBase`, `flutterCtr`, `androidCtr`, `withFlutterSetup`

The optional `flutterImage`/`androidImage` parameters let you supply a pre-built image from the `flutter-container` module instead of building locally.

**Key constants (defined in both modules' `main.go`):**
```go
flutterHome  = "/opt/flutter"
androidHome  = "/opt/android-sdk-linux"
workspaceDir = "/workspace"
```

## quarto module architecture

Module type: `Quarto`, wrapping a `Ctr *dagger.Container` built from the official `ghcr.io/quarto-dev/quarto` image (override via `New`'s `version`/`image`/`container` params) plus `tinytex` for PDF output.

- `Render(ctx, source, input?, siteUrl?) *Renderer` — runs `quarto render` against an arbitrary source directory; chainable `Renderer.Directory()`/`Renderer.File(name)` extract the output.
- `BuildDocs(ctx, source, docsDir?) *dagger.Directory` — thin wrapper around `Render(source.Directory(docsDir)).Directory()`. `docsDir` defaults to `"docs"`. `source` is `+defaultPath="."`, which only auto-resolves to the caller's working directory when this module is the one loaded directly (no `-m`, `dagger.json` in cwd); when loaded via `-m` (local path or remote ref), the default-path context resolves relative to the *loaded* module instead, so callers must pass `--source .` explicitly (verified empirically — omitting it fails with "stat docs: no such file or directory" against the quarto module's own tree). This is the function meant to be called directly by consumers via `dagger call -m github.com/daniel-naegele/daggerverse/quarto build-docs --source .`, without adding this module as a dependency — for consumers that need custom composition beyond "render one directory" (e.g. merging in separately-generated content), install it as a real dependency instead (`dagger install github.com/daniel-naegele/daggerverse/quarto`) and call `Render`/`BuildDocs` directly from Go.

`quarto/tests/` is a separate Dagger module (depends on `quarto` via a local path, `"source": ".."`) that exercises the module against the `testdata/` fixture — run `dagger call all` from `quarto/tests/` after changing `quarto/main.go` (and running `dagger develop` in both `quarto/` and `quarto/tests/` to regenerate bindings).

## Generated files — do not edit

`dagger.gen.go` and `internal/dagger/dagger.gen.go` in each module are auto-generated. After changing the public API (struct fields, method signatures), run `dagger develop` inside that module directory to regenerate them.
