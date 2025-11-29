package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// HelpGenerator generates CLI help documentation.
type HelpGenerator struct {
	binaryPath   string
	templatePath string
	tmpl         *template.Template
}

// HelpData holds data for the CLI help template.
type HelpData struct {
	HelpText string
}

// NewHelpGenerator creates a new CLI help generator.
func NewHelpGenerator(binaryPath, templatePath string) *HelpGenerator {
	return &HelpGenerator{
		binaryPath:   binaryPath,
		templatePath: templatePath,
	}
}

// LoadTemplate loads the template from the configured path.
func (g *HelpGenerator) LoadTemplate() error {
	if g.templatePath == "" {
		return fmt.Errorf("no template path configured")
	}

	content, err := os.ReadFile(g.templatePath)
	if err != nil {
		return fmt.Errorf("reading template: %w", err)
	}

	tmpl, err := template.New("cli_help").Parse(string(content))
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	g.tmpl = tmpl
	return nil
}

// Generate executes the binary with -h flag and formats the output using the template.
func (g *HelpGenerator) Generate() (string, error) {
	if g.tmpl == nil {
		return "", fmt.Errorf("template not loaded")
	}

	// Execute the binary with -h flag
	cmd := exec.Command(g.binaryPath, "-h")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// -h often returns exit code 0, but some binaries return non-zero
		// Check if we got output anyway
		if len(output) == 0 {
			return "", fmt.Errorf("executing %s -h: %w", g.binaryPath, err)
		}
	}

	helpText := strings.TrimSpace(string(output))

	data := HelpData{HelpText: helpText}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// BinaryExists checks if the configured binary exists and is executable.
func (g *HelpGenerator) BinaryExists() bool {
	_, err := exec.LookPath(g.binaryPath)
	return err == nil
}

// GetBinaryPath returns the configured binary path.
func (g *HelpGenerator) GetBinaryPath() string {
	return g.binaryPath
}
