package backend

import (
	"github.com/chrisbanes/grove/internal/clone"
	"github.com/chrisbanes/grove/internal/config"
	"github.com/chrisbanes/grove/internal/workspace"
)

type cpBackend struct{}

func (cpBackend) Name() string {
	return "cp"
}

func (cpBackend) CreateWorkspace(goldenRoot string, cfg *config.Config, opts CreateOptions) (*workspace.Info, error) {
	cloner, err := clone.NewCloner(goldenRoot)
	if err != nil {
		return nil, err
	}

	return workspace.Create(goldenRoot, cfg, cloner, workspace.CreateOpts{
		Branch:       opts.Branch,
		BranchForID:  opts.BranchForID,
		GoldenCommit: opts.GoldenCommit,
		OnClone:      opts.OnClone,
	})
}

func (cpBackend) DestroyWorkspace(goldenRoot string, cfg *config.Config, id string) error {
	return destroyWorkspace(goldenRoot, cfg, id)
}

func (cpBackend) RefreshBase(_ string, _ string) error {
	return nil
}
