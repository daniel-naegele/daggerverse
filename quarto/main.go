// Open-source scientific and technical publishing system built on Pandoc.
package main

import (
	"context"
	"dagger/quarto/internal/dagger"
	"fmt"

	"golang.org/x/exp/slices"
)

// defaultImageRepository is used when no image is specified.
const defaultImageRepository = "ghcr.io/quarto-dev/quarto"

type Quarto struct {
	// +private
	Ctr *dagger.Container
}

func New(
	// Version (image tag) to use from the official image repository as a base container.
	// +optional
	version string,

	// Custom image reference in "repository:tag" format to use as a base container.
	// +optional
	image string,

	// Custom container to use as a base container.
	// +optional
	container *dagger.Container,
) *Quarto {
	var ctr *dagger.Container

	if version != "" {
		ctr = dag.Container().From(fmt.Sprintf("%s:%s", defaultImageRepository, version))
	} else if image != "" {
		ctr = dag.Container().From(image)
	} else if container != nil {
		ctr = container
	} else {
		ctr = dag.Container().From(defaultImageRepository)
	}

	ctr = ctr.
		WithExec([]string{"sh", "-c",
			"apt update && " +
			"apt install -y xz-utils && " +
			"rm -rf /var/lib/apt/lists/*"}).
		WithExec([]string{"quarto", "install", "tinytex"})

	return &Quarto{ctr}
}

func (m *Quarto) Container() *dagger.Container {
	return m.Ctr
}

// Render files or projects to various document types.
func (m *Quarto) Render(
	ctx context.Context,

	// Quarto source directory.
	source *dagger.Directory,

	// Input to render within the project.
	// +optional
	input string,

	// Override site-url for website or book output.
	// +optional
	siteUrl string,
) *Renderer {
	args := []string{
		"quarto", "render",
	}

	if siteUrl != "" {
		args = append(args, "--site-url", siteUrl)
	}

	if input != "" {
		args = append(args, input)
	}

	return &Renderer{
		Ctr:  m.Ctr.WithWorkdir("/work/source").WithDirectory("/work/source", source),
		Args: args,
	}
}

// BuildDocs renders the docs subdirectory of source (default "docs") and
// returns the rendered output directory — the common case of Render, for
// consumers with no custom composition needs (e.g. calling this module
// directly via `dagger call -m github.com/daniel-naegele/daggerverse/quarto
// build-docs`, with no local Dagger module of their own).
func (m *Quarto) BuildDocs(
	ctx context.Context,

	// Project source directory.
	// +defaultPath="."
	source *dagger.Directory,

	// Subdirectory within source containing the Quarto project.
	// +optional
	// +default="docs"
	docsDir string,
) *dagger.Directory {
	return m.Render(ctx, source.Directory(docsDir), "", "").Directory()
}

type Renderer struct {
	// +private
	Ctr *dagger.Container

	// +private
	Args []string
}

func (m *Renderer) run(args ...string) *dagger.Container {
	args = append(slices.Clone(m.Args), args...)

	return m.Ctr.WithExec(args)
}

// Get the output directory after rendering.
func (m *Renderer) Directory() *dagger.Directory {
	return m.run("--output-dir", "../output").Directory("/work/output")
}

// Get the output file after rendering.
func (m *Renderer) File(name string) *dagger.Directory {
	return m.run("--output", "/work/source/_site/"+name).Directory("/work/source/_site/" + name)
}
