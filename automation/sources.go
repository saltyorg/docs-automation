package automation

import (
	"fmt"
	"path/filepath"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/parser"
	"github.com/saltyorg/docs-automation/render"
)

func loadSourceCatalog(cfg *config.Config) (render.SourceCatalog, error) {
	roleVarLookups, err := parser.ScanRoleVarLookups(
		[]string{
			cfg.InventoryPath(),
			filepath.Join(cfg.Repositories.Saltbox, "resources", "tasks", "directories", "create_directories.yml"),
		},
		cfg.GlobalOverrides.IgnoreSuffixes,
	)
	if err != nil {
		return render.SourceCatalog{}, fmt.Errorf("scanning inventory and directory role variables: %w", err)
	}

	resourcesPath := filepath.Join(cfg.Repositories.Saltbox, "resources")
	dockerVarSuffixes, err := parser.NewDockerVarScanner(resourcesPath).FindDockerVarLookups()
	if err != nil {
		return render.SourceCatalog{}, fmt.Errorf("scanning docker variables: %w", err)
	}
	saltboxManagedDirectories, err := parser.ScanManagedDirectoryRoles(cfg.SaltboxRolesPath())
	if err != nil {
		return render.SourceCatalog{}, fmt.Errorf("scanning saltbox managed-directory roles: %w", err)
	}
	sandboxManagedDirectories, err := parser.ScanManagedDirectoryRoles(cfg.SandboxRolesPath())
	if err != nil {
		return render.SourceCatalog{}, fmt.Errorf("scanning sandbox managed-directory roles: %w", err)
	}

	return render.SourceCatalog{
		RoleVarLookups:    roleVarLookups,
		DockerVarSuffixes: dockerVarSuffixes,
		ManagedDirectoryRoles: map[string]map[string]struct{}{
			"saltbox": saltboxManagedDirectories,
			"sandbox": sandboxManagedDirectories,
		},
	}, nil
}
