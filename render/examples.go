package render

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/saltyorg/docs-automation/document"
	"go.yaml.in/yaml/v3"
)

// renderExampleOverride returns a complete fenced assignment, or an empty
// string if neither this scope nor its canonical variable has an example.
// Markdown nesting belongs to the template; YAML indentation belongs here.
func renderExampleOverride(cfg *document.SaltboxAutomationConfig, canonicalName, displayName string) (string, error) {
	entry, ok := cfg.GetExampleOverride(displayName)
	if !ok && displayName != canonicalName {
		entry, ok = cfg.GetExampleOverride(canonicalName)
	}
	if !ok {
		return "", nil
	}
	key := *entry.Content[0]
	key.Value = displayName
	entry.Content[0] = &key
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&entry); err != nil {
		return "", fmt.Errorf("encoding example for %s: %w", displayName, err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("finishing example for %s: %w", displayName, err)
	}
	// Remove only the encoder's last line ending, retaining block-scalar
	// trailing blank lines. Fence length must exceed any run in the YAML.
	payload := strings.TrimSuffix(buf.String(), "\n")
	longest, run := 0, 0
	for _, ch := range payload {
		if ch == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + "yaml\n" + payload + "\n" + fence, nil
}
