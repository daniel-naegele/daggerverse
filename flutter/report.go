package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/flutter/internal/dagger"
)

// runReporting runs script (with args as positional parameters) and tolerates a
// non-zero exit code so that the reports written to reportsDir can still be
// returned. Unless ignoreFailures is set, a non-zero exit code is turned into an
// error carrying the tail of the command output.
func runReporting(
	ctx context.Context,
	ctr *dagger.Container,
	what string,
	script string,
	args []string,
	ignoreFailures bool,
	opts ...dagger.ContainerWithExecOpts,
) (*dagger.Directory, error) {
	var o dagger.ContainerWithExecOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	o.Expect = dagger.ReturnTypeAny

	ctr, err := ctr.
		WithExec([]string{"mkdir", "-p", reportsDir}).
		WithExec(append([]string{"sh", "-c", script, "sh"}, args...), o).
		Sync(ctx)
	if err != nil {
		return nil, err
	}
	code, err := ctr.ExitCode(ctx)
	if err != nil {
		return nil, err
	}
	if code != 0 && !ignoreFailures {
		stdout, _ := ctr.Stdout(ctx)
		stderr, _ := ctr.Stderr(ctx)
		return nil, fmt.Errorf("%s failed with exit code %d\n--- stdout (tail) ---\n%s\n--- stderr (tail) ---\n%s",
			what, code, tail(stdout, 80), tail(stderr, 80))
	}
	return ctr.Directory(reportsDir), nil
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// runChecked runs args and returns its stdout. On a non-zero exit code the
// error contains the command output, so callers (and other modules) can see
// which files or issues caused the failure.
func runChecked(ctx context.Context, ctr *dagger.Container, what string, args []string) (string, error) {
	ctr, err := ctr.WithExec(args, dagger.ContainerWithExecOpts{Expect: dagger.ReturnTypeAny}).Sync(ctx)
	if err != nil {
		return "", err
	}
	code, err := ctr.ExitCode(ctx)
	if err != nil {
		return "", err
	}
	stdout, err := ctr.Stdout(ctx)
	if err != nil {
		return "", err
	}
	if code != 0 {
		stderr, _ := ctr.Stderr(ctx)
		return stdout, fmt.Errorf("%s failed with exit code %d\n%s\n%s", what, code, tail(stdout, 200), tail(stderr, 50))
	}
	return stdout, nil
}
