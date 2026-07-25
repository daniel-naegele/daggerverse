package main

import (
	"context"

	"dagger/quarto/tests/internal/dagger"

	"github.com/sourcegraph/conc/pool"
)

type Tests struct{}

// All executes all tests.
func (m *Tests) All(ctx context.Context) error {
	p := pool.New().WithErrors().WithContext(ctx)

	p.Go(m.Render)
	p.Go(m.BuildDocs)

	return p.Wait()
}

func (m *Tests) Render(ctx context.Context) error {
	dir := dag.CurrentModule().Source().Directory("./testdata")

	_, err := dag.Quarto().Render(dir).Directory().Sync(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (m *Tests) BuildDocs(ctx context.Context) error {
	dir := dag.CurrentModule().Source()

	_, err := dag.Quarto().
		BuildDocs(dagger.QuartoBuildDocsOpts{Source: dir, DocsDir: "testdata"}).
		Sync(ctx)
	if err != nil {
		return err
	}

	return nil
}
