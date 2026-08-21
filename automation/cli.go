package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/docs-automation/clihelp"
	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
)

// UpdateCLIHelp updates the CLI help documentation.
// Returns true if content was actually changed, false if unchanged.
func (r *Runner) UpdateCLIHelp(ctx context.Context, cfg *config.Config, binaryPath string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Determine binary path
	if binaryPath == "" {
		binaryPath = cfg.CLIHelp.BinaryPath
	}

	if binaryPath == "" {
		return false, fmt.Errorf("no binary path configured (set cli_help.binary_path in config or use --binary flag)")
	}

	// Get template path
	templatePath := cfg.CLIHelpTemplatePath()

	// Create generator with template
	generator := clihelp.NewHelpGenerator(binaryPath, templatePath)
	if !generator.BinaryExists() {
		return false, fmt.Errorf("binary not found at %s", binaryPath)
	}

	// Load template
	if err := generator.LoadTemplate(); err != nil {
		return false, fmt.Errorf("loading template: %w", err)
	}

	r.verbosef("Using binary: %s\n", binaryPath)

	// Generate help output
	helpContent, err := generator.Generate(ctx)
	if err != nil {
		return false, fmt.Errorf("generating help: %w", err)
	}

	// Determine docs file path
	docsFile := cfg.CLIHelp.DocsFile
	if docsFile == "" {
		return false, fmt.Errorf("no docs file configured (set cli_help.docs_file in config)")
	}

	docsPath := filepath.Join(cfg.Repositories.Docs, docsFile)

	// Check if docs file exists
	if _, err := os.Stat(docsPath); os.IsNotExist(err) {
		return false, fmt.Errorf("docs file not found at %s", docsPath)
	}

	// Create docs manager
	manager := document.NewManager(document.MarkerConfig{
		Variables: cfg.Markers.Variables,
		CLI:       cfg.Markers.CLI,
		Overview:  cfg.Markers.Overview,
	})

	// Load document
	doc, err := manager.LoadDocument(docsPath)
	if err != nil {
		return false, fmt.Errorf("loading document: %w", err)
	}

	// Store original content to detect actual changes
	originalContent := doc.Content

	// Check if document has CLI section
	if !manager.HasCLISection(doc) {
		return false, fmt.Errorf("document does not have CLI section markers (<!-- BEGIN %s --> / <!-- END %s -->)",
			cfg.Markers.CLI, cfg.Markers.CLI)
	}

	// Update CLI section
	if err := manager.UpdateCLISection(doc, helpContent); err != nil {
		return false, fmt.Errorf("updating CLI section: %w", err)
	}

	// Check if content actually changed
	if doc.Content == originalContent {
		r.verbosef("CLI help unchanged in %s\n", docsPath)
		return false, nil
	}

	// Save document
	if err := manager.SaveDocument(doc); err != nil {
		return false, fmt.Errorf("saving document: %w", err)
	}

	r.printf("Updated CLI help in %s\n", docsPath)
	return true, nil
}
