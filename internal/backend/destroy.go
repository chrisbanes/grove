package backend

import (
	"errors"
	"os"

	"github.com/chrisbanes/grove/internal/config"
	"github.com/chrisbanes/grove/internal/image"
	"github.com/chrisbanes/grove/internal/workspace"
)

func destroyWorkspace(goldenRoot string, cfg *config.Config, id string) error {
	if _, err := image.LoadWorkspaceMeta(goldenRoot, id); err == nil {
		return image.DestroyWorkspace(goldenRoot, id, nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return workspace.Destroy(cfg, id)
}
