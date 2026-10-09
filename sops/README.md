# sops

Minimal [sops](https://github.com/getsops/sops) + [age](https://github.com/FiloSottile/age) toolbox for Dagger pipelines, also published as a standalone multi-arch image.

The image is `alpine:3.24.2` plus four static binaries in `/usr/local/bin`, downloaded from their GitHub releases and checksum-verified:

| Tool | Default version | Override | Checksum source |
|------|-----------------|----------|-----------------|
| `sops` | 3.13.3 | `with-sops-version` | `sops-v<ver>.checksums.txt` |
| `age`, `age-keygen` | 1.3.2 | `with-age-version` | pinned digest, else GitHub release asset digest |
| `ssh-to-age` | 1.3.0 | `with-ssh-to-age-version` | `sha256sums.txt` |

Entrypoint: `sops`. Working directory: `/work`. Platforms: `linux/amd64`, `linux/arm64`. Size: about 28 MB compressed (`sops` itself is ~52 MB uncompressed).

## Dagger usage

```bash
M=github.com/daniel-naegele/daggerverse/sops

# Generate an age keypair
dagger call -m $M age-keygen public-key

# Encrypt: only public material is needed (age recipients, SSH public keys or a .sops.yaml)
dagger call -m $M encrypt --file secrets.yaml --age-recipients age1... export --path secrets.enc.yaml
dagger call -m $M encrypt --file secrets.yaml --ssh-public-keys "$(cat ~/.ssh/id_ed25519.pub)" export --path secrets.enc.yaml
dagger call -m $M encrypt --file secrets.yaml --config .sops.yaml --file-path secrets/prod.yaml export --path secrets.enc.yaml

# Decrypt with an age identity or an SSH ed25519 private key (converted with ssh-to-age)
dagger call -m $M decrypt --file secrets.enc.yaml --age-key env:SOPS_AGE_KEY export --path secrets.yaml
dagger call -m $M decrypt --file secrets.enc.yaml --ssh-key file:$HOME/.ssh/id_ed25519 export --path secrets.yaml

# Extract one value
dagger call -m $M decrypt --file secrets.enc.json --age-key env:SOPS_AGE_KEY --extract '["database"]["password"]' contents
```

SSH keys are handled with `ssh-to-age`: `--ssh-public-keys` is converted to an `age1...` recipient, `--ssh-key` to the matching age identity. A file encrypted this way decrypts with either the SSH key or the converted age identity. It does **not** use sops' native `ssh-ed25519` age recipients.

### Functions

| Function | Description |
|----------|-------------|
| `container(platform?)` | The toolbox image. |
| `encrypt(file, ageRecipients?, sshPublicKeys?, config?, filePath?, encryptedRegex?, unencryptedRegex?, encryptedSuffix?, unencryptedSuffix?, inputType?, outputType?)` | Encrypts. Needs no private key. `filePath` is the path used to match `creation_rules` `path_regex` (defaults to the file name). |
| `decrypt(file, ageKey?, sshKey?, keyservice?, keyservicePort=5000, keyserviceAddress?, inputType?, outputType?, extract?)` | Decrypts. Returns a file with the same name. |
| `keyservice(ageKey?, sshKey?, port=5000)` | A `sops keyservice` service that holds the keys. |
| `age-keygen` | A fresh `AgeKeypair` (`publicKey`, `privateKey` secret). Never cached. |
| `ssh-to-age(publicKey)` | Converts an SSH ed25519 public key to an age recipient. |
| `latest-sops-version`, `latest-age-version`, `latest-ssh-to-age-version` (`token?`) | Latest GitHub release versions. Used by the version-check workflow. |
| `publish(registry, username, password, platforms="linux/amd64,linux/arm64")` | Pushes the multi-arch image. |

### Keyservice: decrypt without the key

`keyservice` runs `sops keyservice --network tcp --address 0.0.0.0:<port>` with the key mounted as a secret. A container bound to it can decrypt with `--keyservice tcp://<alias>:<port>`, and never sees the key.

From Go, after `dagger install github.com/daniel-naegele/daggerverse/sops`:

```go
svc := dag.Sops().Keyservice(dagger.SopsKeyserviceOpts{AgeKey: ageKey})

// Built-in: Decrypt binds the service as "sops-keyservice" for you.
plain := dag.Sops().Decrypt(encrypted, dagger.SopsDecryptOpts{Keyservice: svc})

// Your own container: bind the service and point sops at it.
out, err := dag.Sops().Container().
	WithServiceBinding("keys", svc).
	WithMountedFile("/work/secrets.yaml", encrypted).
	WithExec([]string{"sops", "decrypt", "--enable-local-keyservice=false",
		"--keyservice", "tcp://keys:5000", "secrets.yaml"}).
	Stdout(ctx)
```

`AgeKeygen` generates a new key on every call. In Go, pin the returned object before you read both fields, so that both come from the same key:

```go
id, _ := dag.Sops().AgeKeygen().ID(ctx)
kp := dag.LoadSopsAgeKeypairFromID(dagger.SopsAgeKeypairID(id))
pub, _ := kp.PublicKey(ctx)
priv := kp.PrivateKey()
```

## Published image

`.github/workflows/sops-publish.yml` publishes on every push to `main` that touches `sops/`:

- `<registry>/sops:<sopsVersion>` (e.g. `ghcr.io/daniel-naegele/sops:3.13.3`)
- `<registry>/sops:latest`

`.github/workflows/sops-version-check.yml` checks hourly for new sops, age and ssh-to-age releases. It opens a bump PR on branch `sops-version/<tool>-<version>`, and skips if that branch already exists.

### Plain `docker run` usage

```bash
IMG=ghcr.io/daniel-naegele/sops:latest

# Encrypt (entrypoint is sops, workdir is /work)
docker run --rm -v "$PWD:/work" $IMG encrypt --age age1... secrets.yaml > secrets.enc.yaml

# Decrypt with an age key
docker run --rm -v "$PWD:/work" -e SOPS_AGE_KEY="$(cat key.txt)" $IMG decrypt secrets.enc.yaml

# Decrypt with an SSH ed25519 key (via ssh-to-age)
docker run --rm -v "$PWD:/work" -v "$HOME/.ssh/id_ed25519:/key:ro" --entrypoint sh $IMG \
  -c 'SOPS_AGE_KEY="$(ssh-to-age -private-key -i /key)" sops decrypt secrets.enc.yaml'

# Other tools
docker run --rm --entrypoint age-keygen $IMG
docker run --rm -i --entrypoint ssh-to-age $IMG < ~/.ssh/id_ed25519.pub

# Run a keyservice, then decrypt elsewhere without the key
docker run -d --name sops-keys -p 5000:5000 -e SOPS_AGE_KEY="$(cat key.txt)" $IMG \
  keyservice --network tcp --address 0.0.0.0:5000
sops decrypt --keyservice tcp://localhost:5000 secrets.enc.yaml
```

## Tests

`tests/` is a separate Dagger module that depends on this one. It covers: amd64 and arm64 images, age/SSH/keyservice round trips for YAML, JSON and dotenv, `.sops.yaml` encryption, `--extract`, and failure with a wrong key. `VersionOverride` builds sops 3.10.2, age 1.3.1 and ssh-to-age 1.2.0 and checks each version. `LatestVersions` is not part of `all`, because it calls the GitHub API without a token, which allows 60 requests per hour.

```bash
cd sops/tests && dagger call all
```
