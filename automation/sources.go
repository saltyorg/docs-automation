package automation

import (
	"fmt"
	"path/filepath"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/parser"
	"github.com/saltyorg/docs-automation/render"
)

func loadSourceCatalog(cfg *config.Config) (render.SourceCatalog, error) {
	roleVarLookups, err := parser.ScanInventoryForRoleVarLookups(
		cfg.InventoryPath(),
		cfg.GlobalOverrides.IgnoreSuffixes,
	)
	if err != nil {
		return render.SourceCatalog{}, fmt.Errorf("scanning inventory: %w", err)
	}

	resourcesPath := filepath.Join(cfg.Repositories.Saltbox, "resources")
	dockerVarSuffixes, err := parser.NewDockerVarScanner(resourcesPath).FindDockerVarLookups()
	if err != nil {
		return render.SourceCatalog{}, fmt.Errorf("scanning docker variables: %w", err)
	}

	return render.SourceCatalog{
		RoleVarLookups:    roleVarLookups,
		DockerVarSuffixes: dockerVarSuffixes,
	}, nil
}
