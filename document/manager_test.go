package document

import (
	"bytes"
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

func TestManagerApplyFrontmatterChangesSynchronizesDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.md")
	input := []byte("---\r\nsaltbox_automation:\r\n  app_links:\r\n    - name: Manual\r\n      purpose: null\r\n---\r\n# App\r\n")
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(DefaultMarkers())
	doc, err := manager.LoadDocument(path)
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	icon := "material/docker"
	purpose := AppLinkPurposeManual
	changes := FrontmatterChanges{
		Icon:  &icon,
		Links: []AppLinkChange{{Index: 0, Purpose: &purpose}},
	}

	changed, err := manager.ApplyFrontmatterChanges(doc, changes)
	if err != nil {
		t.Fatalf("ApplyFrontmatterChanges() error = %v", err)
	}
	if !changed {
		t.Fatal("ApplyFrontmatterChanges() changed = false, want true")
	}
	if doc.Frontmatter == nil || doc.Frontmatter.Icon != icon || doc.Frontmatter.SaltboxAutomation.AppLinks[0].Purpose != purpose {
		t.Fatalf("document frontmatter not synchronized: %#v", doc.Frontmatter)
	}
	if doc.Body != "# App\r\n" || doc.Content != "---\r\nicon: material/docker\r\nsaltbox_automation:\r\n  app_links:\r\n    - name: Manual\r\n      purpose: manual\r\n---\r\n# App\r\n" {
		t.Fatalf("document content/body not synchronized: content %q, body %q", doc.Content, doc.Body)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, input) {
		t.Fatal("ApplyFrontmatterChanges() wrote before SaveDocument")
	}

	changed, err = manager.ApplyFrontmatterChanges(doc, changes)
	if err != nil || changed {
		t.Fatalf("second ApplyFrontmatterChanges() = changed %t, error %v, want no-op", changed, err)
	}
	if err := manager.SaveDocument(doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	assertFile(t, path, doc.Content, 0o600)
}

func TestManagerApplyFrontmatterChangesIsAtomicOnFailure(t *testing.T) {
	manager := NewManager(DefaultMarkers())
	originalFrontmatter := &Frontmatter{Icon: "sentinel"}
	doc := &Document{
		Content:     "---\nicon: [\n---\n# App\n",
		Frontmatter: originalFrontmatter,
		Body:        "sentinel body",
	}
	icon := "material/docker"

	changed, err := manager.ApplyFrontmatterChanges(doc, FrontmatterChanges{Icon: &icon})
	if err == nil {
		t.Fatal("ApplyFrontmatterChanges() error = nil, want parse failure")
	}
	if changed {
		t.Fatal("ApplyFrontmatterChanges() changed = true on failure")
	}
	if doc.Content != "---\nicon: [\n---\n# App\n" || doc.Frontmatter != originalFrontmatter || doc.Body != "sentinel body" {
		t.Fatalf("document changed on failure: %#v", doc)
	}
}
