package automation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestRunnerIndexUsesConfiguredOutput(t *testing.T) {
	var output bytes.Buffer
	runner := NewRunner(&output, new(bytes.Buffer), false)
	if err := runner.Index(); err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	want := "Index generation is not yet implemented.\n\n" +
		"This command will eventually:\n" +
		"  1. Scan all app documentation files\n" +
		"  2. Read categories from saltbox_automation.project_description.categories\n" +
		"  3. Generate categorized index.md files\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestRunnerIndexReturnsOutputError(t *testing.T) {
	runner := NewRunner(closedWriter{}, new(bytes.Buffer), false)
	if err := runner.Index(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Index() error = %v, want io.ErrClosedPipe", err)
	}
}

func TestRunnerHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "cli", run: func() error { _, err := runner.UpdateCLIHelp(ctx, nil, ""); return err }},
		{name: "generate", run: func() error { return runner.Generate(ctx, nil, "", GenerateOptions{}) }},
		{name: "scaffold", run: func() error { return runner.Scaffold(ctx, nil, "role", ScaffoldOptions{}) }},
		{name: "update", run: func() error { return runner.Update(ctx, nil, "", UpdateOptions{}) }},
		{name: "validate", run: func() error { return runner.ValidateFrontmatter(ctx, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestRunnerVerboseOutputUsesErrorStream(t *testing.T) {
	var output bytes.Buffer
	var errOutput bytes.Buffer
	runner := NewRunner(&output, &errOutput, true)
	runner.verbosef("verbose %d", 1)

	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", output.String())
	}
	if errOutput.String() != "verbose 1" {
		t.Fatalf("stderr = %q, want verbose output", errOutput.String())
	}
}
