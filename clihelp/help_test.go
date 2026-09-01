package clihelp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHelpGeneratorRendersBinaryHelp(t *testing.T) {
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "sb")
	templatePath := filepath.Join(dir, "cli.tmpl")
	writeExecutable(t, binaryPath, "#!/bin/sh\nprintf 'usage: sb test\\n'\n")
	if err := os.WriteFile(templatePath, []byte("Help:\n{{ .HelpText }}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	generator := NewHelpGenerator(binaryPath, templatePath)
	if !generator.BinaryExists() {
		t.Fatal("BinaryExists() = false, want true")
	}
	if err := generator.LoadTemplate(); err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	got, err := generator.Generate(t.Context())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if want := "Help:\nusage: sb test\n"; got != want {
		t.Fatalf("Generate() = %q, want %q", got, want)
	}
}

func TestHelpGeneratorHonorsCanceledContext(t *testing.T) {
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "sb")
	templatePath := filepath.Join(dir, "cli.tmpl")
	writeExecutable(t, binaryPath, "#!/bin/sh\nprintf 'unreachable\\n'\n")
	if err := os.WriteFile(templatePath, []byte("{{ .HelpText }}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	generator := NewHelpGenerator(binaryPath, templatePath)
	if err := generator.LoadTemplate(); err != nil {
		t.Fatalf("LoadTemplate() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := generator.Generate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate() error = %v, want context.Canceled", err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
