# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository structure

A monorepo of [Dagger](https://dagger.io) 1.0 modules, each in its own subdirectory with its own `dagger-module.toml`, Go module, and committed generated SDK code. The root `dagger.toml` is the workspace: it registers every module as a Go SDK scope and installs the five `*/tests` modules as `<module>-tests` so `dagger check` finds their checks. `dagger.lock` pins resolved images/Git refs. CLI and `engineVersion` are pinned to `v1.0.0-beta.28` (the CLI comes from the `github:dagger/nix` flake input in `~/code/nix/config`).

| Directory | Module name | Purpose |
|-----------|-------------|---------|
| `flutter-container/` | `flutter-container` | Builds Flutter Docker images (flutter, android, emulator stages) |
| `flutter/` | `flutter` | Flutter CI tasks (test, analyze, format, Android builds, emulator integration tests) on top of `flutter-container` images |
| `quarto/` | `quarto` | Renders Quarto documentation projects; `BuildDocs` is the sensible-default entry point for direct `-m` invocation from consumers with no local Dagger module |
| `sops/` | `sops` | Minimal sops + age + ssh-to-age image (published multi-arch to `<registry>/sops`) and encrypt/decrypt/keyservice helpers |
| `codemagic-cli-tools/` | `codemagic-cli-tools` | Minimal multi-arch image with [Codemagic CLI tools](https://github.com/codemagic-ci-cd/cli-tools) + typed wrappers for google-play, android-app-bundle, android-keystore, universal-apk, git-changelog |

## Common commands

In 1.0, `dagger call` without `-m` loads the *workspace* (root `dagger.toml`), not the module in the cwd. Select a module with `-m` (`--load-module`).

```bash
# List checks / run all of them (in parallel, cached per check)
dagger check -l
dagger check
dagger check --module flutter-tests                  # one tests module
dagger check --module sops-tests --check=extract     # one check

# Call a module function (from the module dir, or -m <dir> from the root)
dagger call -m . --help
dagger call -m flutter-container flutter --help

# Regenerate committed SDK code after changing any module's API or dependencies
dagger generate -y
```

Secret arguments need a provider scheme in 1.0: `--password=env://REGISTRY_PASSWORD`, `file://PATH`, `cmd://COMMAND`. The 0.x form `env:NAME` fails with "secret address must name its provider with a scheme".

Check functions are the `// +check`-annotated methods in each `tests/main.go` (everything `All()` runs; `Arm64`/`LatestVersions` deliberately are not checks). Add `// +check` to every new test function and call it from `All()` too. `dagger check` also runs the Go SDK staleness check (`dagger-go-sdk/generate/stale`), so uncommitted codegen fails CI.

Go is only used as the Dagger SDK language — there is no standalone Go binary to build or test directly.

## flutter-container module architecture

Module type: `FlutterContainer`. Default versions: Flutter as set in `New()` in `main.go` (bumped by automated PRs), Android `36`. Use `WithFlutterVersion` / `WithAndroidVersion` to override (Dagger `WithSomething` chaining pattern).

**File-per-image-stage layout:**
- `flutter.go` — `Flutter(platform?)` method + internal `flutterBase(platform, version)`: ubuntu:24.04 + system deps + Flutter SDK cloned from GitHub
- `android.go` — `Android(platform?)` method + internal `androidBase(platform, version)`: extends flutter, adds Android cmdline-tools + SDK packages from Flutter's `packages.txt`
- `emulator.go` — `Emulator(ctx, platform?)` method: extends android, installs emulator + AVD + `socat` + helper scripts; ABI auto-selected (`x86_64` for amd64, `arm64-v8a` for arm64). `EmulatorService(ctx, platform?, avdName?, port?)`: runs `android-start-emulator` as a service with `InsecureRootCapabilities` (for `/dev/kvm`) and `ANDROID_EMULATOR_EXPOSE_ADB=true`
- `scripts/` — `android-start-emulator`, `android-stop-emulator`, `android-wait-for-emulator`, embedded via `go:embed` and installed into `/usr/local/bin` of the emulator image
- `publish.go` — `Publish(ctx, registry, username, password, platforms?)`: pushes multi-arch manifests for flutter, android, and emulator tags
- `constants.go` — `PlatformAMD64`, `PlatformARM64`, `abiAMD64`, `abiARM64`, `emulatorABI(platform)` helper
- `main.go` — struct, `New()`, `WithFlutterVersion`, `WithAndroidVersion`, `parsePlatforms`, `imageDigest`

**Precaching:** everything a CI run would otherwise download is baked in: `flutter config --no-analytics`, `junitreport` (activated with `PUB_CACHE=/opt/pub-global`, separate from the project pub cache that `flutter` mounts at `/cache/pub`), and in the android stage platform + build-tools for the default API level (what the Flutter Gradle plugin compiles against, whatever the emulator level) and for `AndroidVersion` (so Gradle does not install them during the first build). Published images always use the default level (`defaultAndroidVersion`, 36; `Publish` refuses others) and carry no Android tag; `prebuilt.go` (`imageRef`, `PrebuiltFlutter/Android/Emulator(registry?)`) is the single source of the tags, used by `Publish` and the flutter module.

When `platform == ""`, `flutterBase`/`androidBase` call `dag.Container()` without a `Platform` option — Dagger uses the engine host's native platform. `Emulator` calls `dag.DefaultPlatform(ctx)` to resolve the ABI.

**EmulatorService networking:** the emulator binds console (5554) and adb (5555) to `127.0.0.1` only. With `ANDROID_EMULATOR_EXPOSE_ADB=true`, `android-start-emulator` starts `socat` forwarders bound to the container's non-loopback IPv4 addresses *after* `sys.boot_completed=1`, so Dagger's port health check implies a booted device. Clients bind the service (e.g. `WithServiceBinding("emulator", svc)`), then `adb connect emulator:5555` and `flutter test -d emulator:5555`. Never use the alias `emu`: adb parses `emu:...` as a `<console port>,<adb port>` pair. KVM requires the Dagger engine itself to see `/dev/kvm`; the script records the chosen mode as device prop `debug.emulator.accel`.

`flutter-container/tests/` is a separate Dagger module (depends on `..`) with `EmulatorScripts`, `EmulatorService` (boots the service, connects via adb, asserts boot + KVM), `VersionOverride` and `All`. Run `dagger check --module flutter-container-tests` after changing the module (after `dagger generate -y`).

## flutter module architecture

Module type: `Flutter`. Same default versions and `With*` pattern as flutter-container. Depends on the local `flutter-container` module (`"source": "../flutter-container"`); images come from `dag.FlutterContainer().WithFlutterVersion(..).WithAndroidVersion(..).Flutter()/Android()/Emulator()`.

**File-per-CI-task layout:**
- `test.go` — `Test(ctx, project, targets?, coverage?, ignoreFailures?, flutterImage?) *Directory`: `flutter test` with a JSON file reporter; reports dir with `test-results.json`, `junit.xml` (via `junitreport`'s `tojunit`), optional `lcov.info` + `coverage-html/`
- `analyze.go` — `Analyze(ctx, project, fatalInfos?, fatalWarnings?=true, flutterImage?) string`: `flutter analyze`
- `format.go` — `Format(ctx, project, targets?, flutterImage?) string`: `dart format --output=none --set-exit-if-changed`
- `build.go` — `BuildApk` / `BuildAppBundle(ctx, project, mode?="release", flavor?, target?, buildName?, buildNumber?, dartDefines?, keystore?, storePassword?, keyPassword?, keyAlias?, androidImage?) *File`: with a keystore, writes `android/key.properties` (storePassword, keyPassword, keyAlias, storeFile) on a tmpfs symlinked into `android/`; secrets are mounted with `WithMountedSecret`. The consumer's `android/app/build.gradle(.kts)` must read it
- `integration.go` — `IntegrationTest(ctx, project, target?="integration_test", flavor?, dartDefines?, bootTimeout?=300, testTimeout?=600, attempts?=3, flutterVerbose?, requireKvm?=true, ignoreFailures?, emulatorImage?) *Directory`: prebuilds the debug APK (so Gradle does not compete with the running emulator), then runs flutter-container's `android-start-emulator` in the background of the *same* exec (`InsecureRootCapabilities: true` for `/dev/kvm`) and waits for its `emulator ready` log line. KVM detection, boot + package-manager wait, animation settings and the `debug.emulator.accel` prop all come from the emulator image's scripts/`ANDROID_EMULATOR_*` env — do not reimplement them here; add generic behaviour to `flutter-container/scripts/` instead. `requireKvm` fails fast on the script's `hardware acceleration off` line and re-checks `debug.emulator.accel`. `flutter test` attempts are wrapped in `timeout` and retried only on infrastructure failures (DDS/VM service lost, device offline, hang waiting for the VM service — seen intermittently in CI and locally), never on test failures. (Parameter is `flutterVerbose`, not `verbose`: `--verbose` clashes with a dagger CLI flag.) Same-exec instead of `EmulatorService`: adb/VM-service forwarding stays local (no socat hop), the emulator log is captured, and each call gets its own emulator (services are deduped by digest). Not named `*_test.go` on purpose (Go would treat it as a test file)
- `report.go` — `runReporting` (exec with `Expect: Any`, return reports dir, error unless `ignoreFailures`) and `runChecked` (puts command output into the error, since exec stdout is lost across module boundaries)
- `main.go` — struct, `New()`, `With*`, `FlutterImage()`/`AndroidImage()`/`EmulatorImage()` (the images used for the configured versions; `AndroidVersion` only affects the emulator system image), internal helpers: `flutterCtr`, `androidCtr`, `emulatorCtr`, `withProject` (pub cache volume at `/cache/pub`, copy to `/workspace`, `flutter pub get`), `withGradleCache` (locked cache volume as `GRADLE_USER_HOME`, daemon off)

`Registry` defaults to `ghcr.io/daniel-naegele`: `FlutterImage/AndroidImage/EmulatorImage` return flutter-container's `Prebuilt*` images, unless `WithoutPrebuiltImages()` was called or `AndroidVersion` is not the default (published images only exist for that level; `prebuilt()`). `WithPrebuiltImages(registry?)` changes the registry. `flutter/tests` uses `WithoutPrebuiltImages()` (helper `flutter()`) so image changes are tested. JUnit conversion runs `PUB_CACHE=/opt/pub-global dart pub global run junitreport:tojunit` (`junitCmd`); `withJunitReport` only activates it for images lacking it.

The optional `flutterImage`/`androidImage`/`emulatorImage` parameters let you supply a pre-built image (e.g. `ghcr.io/daniel-naegele/flutter:<version>[-android|-emulator]`) instead of building locally.

`flutter/testdata/app` is a trimmed `flutter create` fixture (unit + widget test, `integration_test/`, release signing wired to `key.properties`). `flutter/tests/` is a separate Dagger module (depends on `..` and `../../flutter-container`) with one function per feature plus `All`; it generates a throwaway keystore with `keytool` and verifies signatures with `apksigner`/`jarsigner`. `VersionOverride` checks `WithFlutterVersion` (3.41.9 image, `Test`/`Analyze` on a version-agnostic probe project) and `WithAndroidVersion` (35: AVD system image and `sdk=35` in the integration test's `boot.txt`).

**Running the emulator tests requires a rootful engine** with `/dev/kvm` (a rootless engine drops devices, dagger/dagger#13827). Locally:
```bash
CONTAINER_HOST=unix:///run/podman/podman.sock dagger check --module flutter-tests
```
This is also why CI stays on GitHub Actions (`.github/workflows/test.yml`: KVM enabled on the runner, engine started there, `dagger check --module <module>-tests`) instead of Dagger Cloud Checks/Cloud Engines.

**Key constants (defined in both modules' `main.go`):**
```go
flutterHome  = "/opt/flutter"
androidHome  = "/opt/android-sdk-linux"
workspaceDir = "/workspace"
```

## quarto module architecture

Module type: `Quarto`, wrapping a `Ctr *dagger.Container` built from the official `ghcr.io/quarto-dev/quarto` image (override via `New`'s `version`/`image`/`container` params) plus `tinytex` for PDF output.

- `Render(ctx, source, input?, siteUrl?) *Renderer` — runs `quarto render` against an arbitrary source directory; chainable `Renderer.Directory()`/`Renderer.File(name)` extract the output.
- `BuildDocs(ctx, source, docsDir?) *dagger.Directory` — thin wrapper around `Render(source.Directory(docsDir)).Directory()`. `docsDir` defaults to `"docs"`. `source` is `+defaultPath="."`, which only auto-resolves to the caller's working directory when this module is the one loaded directly (no `-m`, `dagger.json` in cwd); when loaded via `-m` (local path or remote ref), the default-path context resolves relative to the *loaded* module instead, so callers must pass `--source .` explicitly (verified empirically — omitting it fails with "stat docs: no such file or directory" against the quarto module's own tree). This is the function meant to be called directly by consumers via `dagger call -m github.com/daniel-naegele/daggerverse/quarto build-docs --source .`, without adding this module as a dependency — for consumers that need custom composition beyond "render one directory" (e.g. merging in separately-generated content), install it as a real dependency instead (`dagger module client add github.com/daniel-naegele/daggerverse/quarto --sdk=go`) and call `Render`/`BuildDocs` directly from Go.

`quarto/tests/` is a separate Dagger module (depends on `quarto` via a local path, `"source": ".."`) that exercises the module against the `testdata/` fixture — run `dagger check --module quarto-tests` after changing `quarto/main.go` (after `dagger generate -y` to regenerate bindings).

## sops module architecture

Module type: `Sops`. Pinned defaults in `New()`: `SopsVersion` `3.13.3`, `AgeVersion` `1.3.2`, `SshToAgeVersion` `1.3.0` (no `v` prefix); override via `WithSopsVersion` / `WithAgeVersion` / `WithSshToAgeVersion`.

- `container.go` — `Container(platform?)`: `alpine:3.24.2` + static `sops`, `age`, `age-keygen`, `ssh-to-age` from GitHub releases, verified via `dag.HTTP(..., Checksum)`. sops/ssh-to-age checksums come from the release checksum files; age publishes none, so digests come from `knownAgeDigests` (pinned) or the GitHub API asset `digest` field. Entrypoint `sops`, workdir `/work`. Only `linux/amd64` and `linux/arm64`.
- `sops.go` — `Encrypt` (public material only: `ageRecipients`, `sshPublicKeys` via ssh-to-age, `config` `.sops.yaml`), `Decrypt` (`ageKey`, `sshKey`, `keyservice` service bound as `sops-keyservice`, `keyserviceAddress`), `Keyservice` (`sops keyservice --network tcp`). Keys are mounted secrets; the `sops-with-keys` wrapper builds `SOPS_AGE_KEY` at runtime.
- `keys.go` — `AgeKeygen` (`+cache="never"`, returns `AgeKeypair{PublicKey, PrivateKey}`), `SshToAge(publicKey)`.
- `version.go` — `LatestSopsVersion` / `LatestAgeVersion` / `LatestSshToAgeVersion(token?)`, used by `.github/workflows/sops-version-check.yml` (bump PRs on `sops-version/<tool>-<ver>`).
- `publish.go` — `Publish(registry, username, password, platforms?)` pushes `<registry>/sops:<SopsVersion>` and `:latest`; run by `.github/workflows/sops-publish.yml` on pushes to `main` touching `sops/**`.

`sops/tests/` is a separate Dagger module (pattern of `quarto/tests`); run `dagger check --module sops-tests` after changes.

## codemagic-cli-tools module architecture

Module type: `CodemagicCliTools`. Pinned defaults as struct fields set in `New()`: `CodemagicVersion` (PyPI), `BundletoolVersion` (google/bundletool releases), `PythonVersion` (`python:<v>-slim-trixie`), `JavaVersion` (Temurin JDK major, jlink'ed). Override via `WithCodemagicVersion` / `WithBundletoolVersion` / `WithPythonVersion` / `WithJavaVersion`.

- `container.go` — `Container(platform?)`: python slim + git + jlink'ed JRE (`/opt/java`, includes keytool + jarsigner via `jdk.jartool`) + pinned bundletool (`/opt/bundletool/bundletool.jar`, `bundletool` wrapper; codemagic's vendored jar is replaced by a symlink) + `pip install codemagic-cli-tools`; runs as `codemagic` (uid 1000) in `/workspace`. bundletool's `aapt2` is x86_64-only → APK generation works on amd64 only.
- `android_app_bundle.go` — `AndroidAppBundle(bundle)` object: `Dump`, `Validate`, `IsSigned`, `Sign`, `BuildApks`, `BuildUniversalApk`; plus `BundletoolInfo`. Shared `withSigning`/`withOptionalSigning` helpers.
- `android_keystore.go` — `CreateKeystore(...) *File`; `AndroidKeystore(keystore, password, alias)` object: `Verify`, `Certificate`.
- `google_play.go` — `GooglePlay(credentials)` object: `LatestBuildNumber`, `PublishBundle`, `UploadBundle`, `UploadToInternalAppSharing`, `PromoteRelease`, `GetTrack`, `ListTracks`.
- `git_changelog.go` — `GitChangelog(source, ...)`; `universal_apk.go` — deprecated `UniversalApk`.
- `main.go` — struct, `New()`, `With*`, `Exec(args, source?, secretEnv?, secrets?)` escape hatch, `wrapExecError`/`output` helpers (surface codemagic's stderr in errors).
- `version.go` — `LatestCodemagicVersion` (PyPI JSON) / `LatestBundletoolVersion` (GitHub releases/latest redirect), used by `.github/workflows/codemagic-cli-tools-version-check.yml` (opens bump PRs, skips if branch exists).
- `publish.go` — `Publish(ctx, registry, username, password, platforms?)` pushes `<registry>/codemagic-cli-tools:<CodemagicVersion>`; run by `.github/workflows/codemagic-cli-tools-publish.yml` on pushes to main touching `codemagic-cli-tools/**`.

Secrets are never plain args: they are set with `WithSecretVariable` and passed to codemagic as `@env:<NAME>` references. `codemagic-cli-tools/tests/` is a separate module (depends on `..`, like `quarto/tests`): run `dagger check --module codemagic-cli-tools-tests` (and `dagger call -m codemagic-cli-tools/tests arm-64` for the arm64 image) after changes.

## Generated files — do not edit

`dagger.gen.go` and `internal/` in each module are auto-generated and committed (1.0 modules do not run codegen at load time). After changing the public API (struct fields, method signatures) or dependencies, run `dagger generate -y` from the repo root and commit the result; CI's staleness check fails otherwise. Generated Go names use acronym casing (`CodemagicCLITools`, `SHA`), and objects are re-loaded by ID with `dagger.Ref[*dagger.T](dag, id)`.
