package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/docs-automation/internal/cli"
	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/saltyorg/docs-automation/internal/docs"
	"github.com/spf13/cobra"
)

var (
	cliBinaryPath string
)

var cliCmd = &cobra.Command{
	Use:   "cli",
	Short: "Update CLI help documentation",
	Long: `Update CLI help documentation from sb-go binary output.

Executes the sb binary with -h flag and updates the managed
CLI section in the documentation file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.Load(GetConfigPath())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		_, err = updateCLIHelp(cfg)
		return err
	},
}

func init() {
	cliCmd.Flags().StringVar(&cliBinaryPath, "binary", "", "path to sb binary (default: from config)")
	rootCmd.AddCommand(cliCmd)
}

// updateCLIHelp updates the CLI help documentation.
// Returns true if content was actually changed, false if unchanged.
func updateCLIHelp(cfg *config.Config) (bool, error) {
	// Determine binary path
	binaryPath := cliBinaryPath
	if binaryPath == "" {
		binaryPath = cfg.CLIHelp.BinaryPath
	}

	if binaryPath == "" {
		return false, fmt.Errorf("no binary path configured (set cli_help.binary_path in config or use --binary flag)")
	}

	// Get template path
	templatePath := cfg.CLIHelpTemplatePath()

	// Create generator with template
	generator := cli.NewHelpGenerator(binaryPath, templatePath)
	if !generator.BinaryExists() {
		return false, fmt.Errorf("binary not found at %s", binaryPath)
	}

	// Load template
	if err := generator.LoadTemplate(); err != nil {
		return false, fmt.Errorf("loading template: %w", err)
	}

	if IsVerbose() {
		fmt.Fprintf(os.Stderr, "Using binary: %s\n", binaryPath)
	}

	// Generate help output
	helpContent, err := generator.Generate()
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
	manager := docs.NewManager(docs.MarkerConfig{
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
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "CLI help unchanged in %s\n", docsPath)
		}
		return false, nil
	}

	// Save document
	if err := manager.SaveDocument(doc); err != nil {
		return false, fmt.Errorf("saving document: %w", err)
	}

	fmt.Printf("Updated CLI help in %s\n", docsPath)
	return true, nil
}
