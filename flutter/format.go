package main

import (
	"context"

	"dagger/flutter/internal/dagger"
)

// Format checks that all Dart files are formatted
// (dart format --set-exit-if-changed). Fails and lists the offending files
// when formatting would change anything.
func (m *Flutter) Format(
	ctx context.Context,
	// Flutter project directory.
	// +ignore=["build", ".dart_tool", "android/.gradle", "ios/Pods"]
	project *dagger.Directory,
	// Files or directories to check (defaults to the whole project).
	// +optional
	targets []string,
	// Pre-built Flutter image to use instead of building one.
	// +optional
	flutterImage *dagger.Container,
) (string, error) {
	if len(targets) == 0 {
		targets = []string{"."}
	}
	args := append([]string{"dart", "format", "--output=none", "--set-exit-if-changed"}, targets...)
	return runChecked(ctx, m.flutterCtr(project, flutterImage), "dart format check", args)
}
