package backend

import (
	"fmt"

	"github.com/chrisbanes/grove/internal/clone"
	"github.com/chrisbanes/grove/internal/config"
	"github.com/chrisbanes/grove/internal/workspace"
)

// CreateOptions controls workspace creation across backend implementations.
type CreateOptions struct {
	Branch       string
	BranchForID  string
	GoldenCommit string
	OnClone      clone.ProgressFunc
}

// Backend provides workspace lifecycle operations for a clone backend.
type Backend interface {
	Name() string
	CreateWorkspace(goldenRoot string, cfg *config.Config, opts CreateOptions) (*workspace.Info, error)
	DestroyWorkspace(goldenRoot string, cfg *config.Config, id string) error
	RefreshBase(goldenRoot, commit string, excludes []string, onProgress func(int, string)) error
}

func ForName(name string) (Backend, error) {
	switch name {
	case "cp":
		return cpBackend{}, nil
	case "image":
		return imageBackend{}, nil
	default:
		return nil, fmt.Errorf("invalid clone_backend %q: expected cp or image", name)
	}
}
