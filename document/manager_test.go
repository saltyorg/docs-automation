package document

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerSaveDocumentPreservesMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(DefaultMarkers())
	doc := &Document{Path: path, Content: "new"}

	if err := manager.SaveDocument(doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	assertFile(t, path, "new", 0o600)
}
