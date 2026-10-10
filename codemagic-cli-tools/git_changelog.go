package main

import (
	"context"
	"strconv"

	"dagger/codemagic-cli-tools/internal/dagger"
)

// GitChangelog generates a changelog from the git history of source (`git-changelog generate`).
//
// source must contain the .git directory (e.g. pass the repository root; in CI make sure the checkout is
// not too shallow for the requested range).
func (m *CodemagicCliTools) GitChangelog(
	ctx context.Context,
	// Git repository (including .git).
	source *dagger.Directory,
	// Stop at this commit hash (exclusive), e.g. the commit of the previous release.
	// +optional
	previousCommit string,
	// Regex for commit message lines to skip. Defaults to codemagic's merge-commit pattern.
	// +optional
	skipPattern string,
	// Maximum number of commits to read from git before filtering.
	// +optional
	// +default=50
	commitLimit int,
) (string, error) {
	args := []string{"git-changelog", "generate"}
	if previousCommit != "" {
		args = append(args, "--previous-commit", previousCommit)
	}
	if skipPattern != "" {
		args = append(args, "--skip-pattern", skipPattern)
	}
	if commitLimit > 0 {
		args = append(args, "--commit-limit", strconv.Itoa(commitLimit))
	}
	ctr := m.base().WithDirectory(workspaceDir, source, dagger.ContainerWithDirectoryOpts{Owner: user})
	return output(ctx, ctr, args)
}
