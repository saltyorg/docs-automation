package cmd

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/buildinfo"
	"github.com/saltyorg/docs-automation/config"
	"github.com/spf13/pflag"
)

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestNewRootCmdBuildsFreshCommandTrees(t *testing.T) {
	first := NewRootCmd()
	second := NewRootCmd()
	if first == second {
		t.Fatal("NewRootCmd returned the same root command instance")
	}

	firstVersion, _, err := first.Find([]string{"version"})
	if err != nil {
		t.Fatalf("finding first version command: %v", err)
	}
	secondVersion, _, err := second.Find([]string{"version"})
	if err != nil {
		t.Fatalf("finding second version command: %v", err)
	}
	if firstVersion == secondVersion {
		t.Fatal("NewRootCmd reused a child command instance")
	}

	first.SetArgs([]string{"--verbose", "version"})
	first.SetOut(new(bytes.Buffer))
	first.SetErr(new(bytes.Buffer))
	if err := first.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("executing first command: %v", err)
	}
	if got := second.PersistentFlags().Lookup("verbose").Value.String(); got != "false" {
		t.Fatalf("fresh verbose flag = %q, want false", got)
	}
}

func TestRootCommandMetadata(t *testing.T) {
	root := NewRootCmd()
	if !root.SilenceUsage || !root.SilenceErrors {
		t.Fatal("root command must silence Cobra usage and error printing")
	}
	if root.Version != buildinfo.VersionString() {
		t.Fatalf("root version = %q, want %q", root.Version, buildinfo.VersionString())
	}

	paths := [][]string{
		{"cli"},
		{"generate"},
		{"index"},
		{"scaffold"},
		{"update"},
		{"validate", "config"},
		{"validate", "frontmatter"},
		{"version"},
	}
	for _, path := range paths {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("finding %s: %v", strings.Join(path, " "), err)
		}
		if cmd.RunE == nil {
			t.Errorf("%s does not use RunE", strings.Join(path, " "))
		}
		if cmd.Run != nil {
			t.Errorf("%s unexpectedly defines Run", strings.Join(path, " "))
		}
	}

	flags := map[string][]string{
		"cli":      {"binary"},
		"generate": {"cli"},
		"scaffold": {"force", "output", "template"},
		"update":   {"check", "issue-label", "manage-issue", "no-cli"},
	}
	for commandName, wantFlags := range flags {
		cmd, _, err := root.Find([]string{commandName})
		if err != nil {
			t.Fatalf("finding %s: %v", commandName, err)
		}
		var gotFlags []string
		cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
			gotFlags = append(gotFlags, flag.Name)
		})
		slices.Sort(gotFlags)
		if !slices.Equal(gotFlags, wantFlags) {
			t.Errorf("%s flags = %v, want %v", commandName, gotFlags, wantFlags)
		}
	}

	configFlag := root.PersistentFlags().Lookup("config")
	if configFlag == nil || configFlag.DefValue != "config.yml" {
		t.Fatalf("config flag = %#v, want default config.yml", configFlag)
	}
	verboseFlag := root.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil || verboseFlag.Shorthand != "v" || verboseFlag.DefValue != "false" {
		t.Fatalf("verbose flag = %#v, want -v and default false", verboseFlag)
	}
}

func TestUpdateCheckFlagDescribesConfiguredDocumentationHealth(t *testing.T) {
	root := NewRootCmd()
	update, _, err := root.Find([]string{"update"})
	if err != nil {
		t.Fatalf("finding update command: %v", err)
	}
	check := update.Flags().Lookup("check")
	if check == nil {
		t.Fatal("update --check flag is missing")
	}
	const want = "run configured documentation-health checks after updating"
	if check.Usage != want {
		t.Fatalf("update --check help = %q, want %q", check.Usage, want)
	}
}

func TestVersionOutputUsesCommandWriter(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := NewRootCmd()
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatalf("ExecuteContext() error = %v", err)
			}
			if got, want := output.String(), buildinfo.VersionString()+"\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestConfigFlagAndLoaderAreInjected(t *testing.T) {
	var loadedPath string
	deps := dependencies{
		loadConfig: func(path string) (*config.Config, error) {
			loadedPath = path
			return &config.Config{}, nil
		},
	}
	root := newRootCmd(deps)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--config", "custom.yml", "validate", "config"})

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if loadedPath != "custom.yml" {
		t.Fatalf("loaded path = %q, want custom.yml", loadedPath)
	}
	if output.String() != "✅ Config is valid\n" {
		t.Fatalf("output = %q, want validation confirmation", output.String())
	}
}

func TestCommandsRejectUnexpectedArguments(t *testing.T) {
	for _, args := range [][]string{{"version", "extra"}, {"cli", "extra"}, {"index", "extra"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := NewRootCmd()
			root.SetOut(new(bytes.Buffer))
			root.SetErr(new(bytes.Buffer))
			root.SetArgs(args)
			if err := root.ExecuteContext(t.Context()); err == nil {
				t.Fatal("ExecuteContext() error = nil, want argument validation error")
			}
		})
	}
}

func TestIndexCommandReturnsOutputError(t *testing.T) {
	root := NewRootCmd()
	root.SetOut(closedWriter{})
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"index"})

	if err := root.ExecuteContext(t.Context()); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("ExecuteContext() error = %v, want io.ErrClosedPipe", err)
	}
}
