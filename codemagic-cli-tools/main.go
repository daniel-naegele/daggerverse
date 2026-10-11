// Codemagic CLI tools (https://github.com/codemagic-ci-cd/cli-tools) in a minimal container.
//
// Builds a small image with codemagic-cli-tools, a jlink'ed Java runtime (java,
// keytool, jarsigner), a pinned bundletool and git, and exposes typed wrappers
// for the Android-related tools: google-play, android-app-bundle,
// android-keystore, universal-apk and git-changelog. Secrets (keystore
// passwords, Google Play service account credentials) are always passed as
// Dagger secrets and handed to the CLI via environment variables
// ("@env:<NAME>" argument references), never as plain command-line arguments.
//
// Versions are pinned to sensible defaults and can be overridden with the
// With*Version chaining functions.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"dagger/codemagic-cli-tools/internal/dagger"
)

const (
	// workspaceDir is the working directory (owned by the non-root user) in which inputs are placed.
	workspaceDir = "/workspace"
	// secretsDir holds mounted keystores and other sensitive files.
	secretsDir = "/run/codemagic"
	// javaHome is where the jlink'ed Java runtime is installed.
	javaHome = "/opt/java"
	// bundletoolJar is the path of the pinned bundletool jar.
	bundletoolJar = "/opt/bundletool/bundletool.jar"
	// user is the non-root user the tools run as.
	user = "codemagic"
	// uid is the numeric id of user.
	uid = "1000"
)

type CodemagicCliTools struct {
	// codemagic-cli-tools version from PyPI (e.g. "0.70.0").
	CodemagicVersion string
	// bundletool version from google/bundletool GitHub releases (e.g. "1.18.3").
	BundletoolVersion string
	// Python version used for the python:<version>-slim-trixie base image (e.g. "3.13").
	PythonVersion string
	// Eclipse Temurin JDK major version used to jlink the Java runtime (e.g. "21").
	JavaVersion string
}

func New() *CodemagicCliTools {
	return &CodemagicCliTools{
		CodemagicVersion:  "0.70.0",
		BundletoolVersion: "1.18.3",
		PythonVersion:     "3.13",
		JavaVersion:       "21",
	}
}

// WithCodemagicVersion returns this module configured to use the given codemagic-cli-tools version.
func (m *CodemagicCliTools) WithCodemagicVersion(version string) *CodemagicCliTools {
	m.CodemagicVersion = version
	return m
}

// WithBundletoolVersion returns this module configured to use the given bundletool version.
func (m *CodemagicCliTools) WithBundletoolVersion(version string) *CodemagicCliTools {
	m.BundletoolVersion = version
	return m
}

// WithPythonVersion returns this module configured to use the given Python base image version.
func (m *CodemagicCliTools) WithPythonVersion(version string) *CodemagicCliTools {
	m.PythonVersion = version
	return m
}

// WithJavaVersion returns this module configured to use the given Java (Temurin JDK) major version.
func (m *CodemagicCliTools) WithJavaVersion(version string) *CodemagicCliTools {
	m.JavaVersion = version
	return m
}

// Exec runs an arbitrary command (e.g. ["google-play", "tracks", "list", "--package-name", "com.example"])
// in the codemagic container and returns the resulting container. This is the escape hatch for tools and
// options that have no typed wrapper.
//
// The optional source directory is copied to /workspace (the working directory). Pass secrets via
// secretEnv names + secrets (same length, paired by index) and reference them as "@env:<NAME>" in args.
func (m *CodemagicCliTools) Exec(
	// Command and arguments to run.
	args []string,
	// Directory copied to /workspace before running the command.
	// +optional
	source *dagger.Directory,
	// Names of environment variables to set from secrets (paired with secrets by index).
	// +optional
	secretEnv []string,
	// Secret values for secretEnv.
	// +optional
	secrets []*dagger.Secret,
) (*dagger.Container, error) {
	if len(secretEnv) != len(secrets) {
		return nil, fmt.Errorf("secretEnv and secrets must have the same length (got %d and %d)", len(secretEnv), len(secrets))
	}
	ctr := m.base()
	if source != nil {
		ctr = ctr.WithDirectory(workspaceDir, source, dagger.ContainerWithDirectoryOpts{Owner: user})
	}
	for i, name := range secretEnv {
		ctr = ctr.WithSecretVariable(name, secrets[i])
	}
	return ctr.WithExec(args), nil
}

// base returns the codemagic container for the engine's native platform.
func (m *CodemagicCliTools) base() *dagger.Container {
	return m.Container("")
}

// parsePlatforms splits a comma-separated platform string into a slice.
func parsePlatforms(s string) []dagger.Platform {
	parts := strings.Split(s, ",")
	out := make([]dagger.Platform, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, dagger.Platform(p))
		}
	}
	return out
}

// wrapExecError turns a failed exec into an error that carries the command's stderr, so callers of this
// module (which only see the error message) get the actual codemagic error instead of just an exit code.
// Commands never contain secret values (secrets are passed as "@env:" references).
func wrapExecError(err error) error {
	var e *dagger.ExecError
	if errors.As(err, &e) {
		msg := strings.TrimSpace(e.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(e.Stdout)
		}
		return fmt.Errorf("%s failed with exit code %d:\n%s", strings.Join(e.Cmd, " "), e.ExitCode, msg)
	}
	return err
}

// output returns the trimmed stdout of running args in ctr, with exec errors wrapped.
func output(ctx context.Context, ctr *dagger.Container, args []string) (string, error) {
	out, err := ctr.WithExec(args).Stdout(ctx)
	if err != nil {
		return "", wrapExecError(err)
	}
	return strings.TrimSpace(out), nil
}
