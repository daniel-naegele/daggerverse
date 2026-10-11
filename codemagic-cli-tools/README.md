# codemagic-cli-tools

Dagger module wrapping [Codemagic CLI tools](https://github.com/codemagic-ci-cd/cli-tools)
(`codemagic-cli-tools` on PyPI) in a small, non-root container, with typed functions for the
Android tooling: `google-play`, `android-app-bundle`, `android-keystore`, `universal-apk` and
`git-changelog`.

## Image

| Component | Default | Override |
|-----------|---------|----------|
| codemagic-cli-tools | pinned in `main.go` (`CodemagicVersion`) | `with-codemagic-version` |
| bundletool | pinned in `main.go` (`BundletoolVersion`) | `with-bundletool-version` |
| Python base | `python:3.13-slim-trixie` | `with-python-version` |
| Java runtime | Temurin 21, `jlink`ed (java, keytool, jarsigner) | `with-java-version` |

- bundletool lives at `/opt/bundletool/bundletool.jar`, with a `bundletool` wrapper on `PATH`. The jar
  vendored by codemagic is replaced by a symlink to it, and `ANDROID_APP_BUNDLE_BUNDLETOOL` points at it,
  so every codemagic code path uses the pinned version.
- `git` is installed for `git-changelog` (`safe.directory=*` is set system-wide, so mounted repositories
  owned by another user work).
- Runs as user `codemagic` (uid 1000) in `/workspace`.
- Unused Google API discovery documents are removed (only `androidpublisher` and
  `firebaseappdistribution` are kept).
- Size (linux/amd64): ~150 MB compressed, ~190 MB on disk.
- `linux/amd64` and `linux/arm64`. bundletool's bundled `aapt2` is x86_64-only, so APK generation from
  bundles (`build-apks`, `build-universal-apk`, `universal-apk`) works on `linux/amd64` only; everything
  else works on both.

A scheduled workflow (`codemagic-cli-tools-version-check.yml`) opens a PR whenever a new
codemagic-cli-tools (PyPI) or bundletool (GitHub) release appears; merging it publishes a new image
(`codemagic-cli-tools-publish.yml`).

### Prebuilt image

Published as `ghcr.io/daniel-naegele/codemagic-cli-tools:<codemagic-version>`
(multi-arch `linux/amd64` + `linux/arm64`).

```sh
# Show available tools
docker run --rm ghcr.io/daniel-naegele/codemagic-cli-tools:0.70.0

# Validate and inspect a bundle
docker run --rm -v "$PWD:/workspace" ghcr.io/daniel-naegele/codemagic-cli-tools:0.70.0 \
  android-app-bundle validate --bundle app-release.aab

# Upload to Google Play (credentials read from the environment)
docker run --rm -v "$PWD:/workspace" \
  -e GOOGLE_PLAY_SERVICE_ACCOUNT_CREDENTIALS="$(cat service-account.json)" \
  ghcr.io/daniel-naegele/codemagic-cli-tools:0.70.0 \
  google-play bundles publish --bundle app-release.aab --track internal

# Changelog since the last tag
docker run --rm -v "$PWD:/workspace" ghcr.io/daniel-naegele/codemagic-cli-tools:0.70.0 \
  git-changelog generate --previous-commit "$(git rev-list -n1 "$(git describe --tags --abbrev=0)")"
```

The container runs as uid 1000; if your files are owned by another user and a command needs to write
(e.g. `sign`, `build-universal-apk`), add `--user "$(id -u):$(id -g)"`.

## Dagger usage

```sh
M=github.com/daniel-naegele/daggerverse/codemagic-cli-tools
```

### android-app-bundle

```sh
dagger call -m $M android-app-bundle --bundle app.aab dump --xpath /manifest/@android:versionCode
dagger call -m $M android-app-bundle --bundle app.aab validate
dagger call -m $M android-app-bundle --bundle app.aab is-signed
dagger call -m $M android-app-bundle --bundle app.aab \
  sign --keystore upload.jks --keystore-password env:KS_PASS --key-alias upload [--key-password env:KEY_PASS] \
  export --path app-signed.aab
dagger call -m $M android-app-bundle --bundle app.aab \
  build-universal-apk [--keystore upload.jks --keystore-password env:KS_PASS --key-alias upload] \
  export --path app-universal.apk
dagger call -m $M android-app-bundle --bundle app.aab build-apks [--universal] export --path app.apks
dagger call -m $M bundletool-info
```

### android-keystore

```sh
dagger call -m $M create-keystore --keystore-password env:KS_PASS --key-alias upload \
  --common-name "Example Corp" --country DE export --path upload.jks
dagger call -m $M android-keystore --keystore upload.jks --keystore-password env:KS_PASS --key-alias upload verify
dagger call -m $M android-keystore --keystore upload.jks --keystore-password env:KS_PASS --key-alias upload certificate
```

### google-play

`--credentials` is the service account JSON key as a Dagger secret (`env:`, `file:`, `op://`, ...).

```sh
dagger call -m $M google-play --credentials file:./service-account.json \
  latest-build-number --package-name com.example.app [--tracks internal,production]
dagger call -m $M google-play --credentials env:GOOGLE_PLAY_CREDENTIALS \
  publish-bundle --bundle app.aab --track internal \
  [--release-notes '[{"language":"en-US","text":"Bug fixes"}]'] [--rollout-fraction 0.1 | --draft] \
  [--in-app-update-priority 3] [--release-name 1.2.3] [--changes-not-sent-for-review]
dagger call -m $M google-play --credentials env:GOOGLE_PLAY_CREDENTIALS upload-bundle --bundle app.aab
dagger call -m $M google-play --credentials env:GOOGLE_PLAY_CREDENTIALS upload-to-internal-app-sharing --bundle app.aab
dagger call -m $M google-play --credentials env:GOOGLE_PLAY_CREDENTIALS \
  promote-release --package-name com.example.app --source-track internal --target-track production \
  [--release-status inProgress --user-fraction 0.2] [--version-code-filter 42]
dagger call -m $M google-play --credentials env:GOOGLE_PLAY_CREDENTIALS get-track --package-name com.example.app --track production
dagger call -m $M google-play --credentials env:GOOGLE_PLAY_CREDENTIALS list-tracks --package-name com.example.app
```

### git-changelog

```sh
dagger call -m $M git-changelog --source . [--previous-commit <sha>] [--skip-pattern <regex>] [--commit-limit 50]
```

### universal-apk (deprecated upstream)

```sh
dagger call -m $M universal-apk --bundle app.aab export --path app-universal.apk
```

Prefer `android-app-bundle build-universal-apk`.

### Anything else: `exec` / `container`

```sh
# Run any codemagic command; secrets are exposed as env vars and referenced with "@env:<NAME>"
dagger call -m $M exec --source . \
  --args google-play,deobfuscation-files,upload,--package-name,com.example.app,--deobfuscation-file,mapping.txt,--version-code,42 \
  --secret-env GOOGLE_PLAY_SERVICE_ACCOUNT_CREDENTIALS --secrets env:GOOGLE_PLAY_CREDENTIALS \
  stdout

dagger call -m $M container terminal
```

### Versions and publishing

```sh
dagger call -m $M with-codemagic-version --version 0.69.0 with-bundletool-version --version 1.18.2 container
dagger call -m $M latest-codemagic-version
dagger call -m $M latest-bundletool-version

# Publish <registry>/codemagic-cli-tools:<codemagic-version> (linux/amd64 + linux/arm64)
dagger call -m $M publish --registry ghcr.io/your-org --username $GITHUB_ACTOR --password env:GITHUB_TOKEN \
  [--platforms linux/amd64]
```

### Secrets

Keystore passwords and Google Play credentials are only accepted as Dagger secrets. They are set as
environment variables in the container and passed to codemagic as `@env:<NAME>` references, so they never
appear in command lines, logs or cache keys. Keystores are mounted (not copied) into the container.

## Tests

```sh
cd tests
dagger call all              # keystore, android-app-bundle, universal-apk, git-changelog, google-play wiring, exec, version overrides
dagger call latest-versions  # Latest*Version return semver (network-dependent, not in all)
dagger call arm-64           # linux/arm64 image builds and runs (needs emulation on amd64 hosts)
```

The App Bundle fixture is `install-time-permanent-modules.aab` from bundletool's own test resources
(Apache-2.0), downloaded at a pinned commit and verified by checksum.
