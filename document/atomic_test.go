package document

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWriteFileAtomicCreatesCompleteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.md")

	if err := WriteFileAtomic(path, []byte("complete\n"), 0o640, false); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	assertFile(t, path, "complete\n", 0o640)
	assertDirectoryEntries(t, dir, []string{"app.md"})
}

func TestWriteFileAtomicRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteFileAtomic(path, []byte("new"), 0o644, false)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("error = %v, want fs.ErrExist", err)
	}
	assertFile(t, path, "old", 0o644)
	assertDirectoryEntries(t, dir, []string{"app.md"})
}

func TestWriteFileAtomicReplacesCompleteRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, []byte("new"), 0o644, true); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	assertFile(t, path, "new", 0o600)
	assertDirectoryEntries(t, dir, []string{"app.md"})
}

func TestWriteFileAtomicRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "app.md")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := WriteFileAtomic(link, []byte("new"), 0o644, true)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("WriteFileAtomic() error = %v, want non-regular destination error", err)
	}
	assertFile(t, target, "old", 0o644)
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link mode = %v, want symlink", info.Mode())
	}
	assertDirectoryEntries(t, dir, []string{"app.md", "target.md"})
}

func assertFile(t *testing.T, path, wantContent string, wantMode fs.FileMode) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != wantContent {
		t.Fatalf("%s content = %q, want %q", path, content, wantContent)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("%s mode = %v, want %v", path, info.Mode().Perm(), wantMode)
	}
}

func assertDirectoryEntries(t *testing.T, dir string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("directory entries = %v, want %v", got, want)
	}
}
