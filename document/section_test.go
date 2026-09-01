package document

import (
	"slices"
	"strings"
	"testing"
)

func TestFindManagedSection(t *testing.T) {
	content := "before\n<!-- BEGIN TEST -->\nold\n<!-- END TEST -->\nafter\n"
	section := FindManagedSection(content, "TEST")
	if section == nil {
		t.Fatal("FindManagedSection() = nil")
	}
	if section.Name != "TEST" {
		t.Fatalf("Name = %q, want TEST", section.Name)
	}
	if section.Content != "\nold\n" {
		t.Fatalf("Content = %q, want newline-wrapped old", section.Content)
	}
	if section.StartLine != 2 || section.EndLine != 4 {
		t.Fatalf("lines = %d-%d, want 2-4", section.StartLine, section.EndLine)
	}
	if got := content[section.StartIndex:section.EndIndex]; got != "<!-- BEGIN TEST -->\nold\n<!-- END TEST -->" {
		t.Fatalf("indexed section = %q", got)
	}
}

func TestUpdateManagedSection(t *testing.T) {
	content := "before\n<!-- BEGIN TEST -->\nold\n<!-- END TEST -->\nafter\n"
	got, err := UpdateManagedSection(content, "TEST", "new")
	if err != nil {
		t.Fatalf("UpdateManagedSection() error = %v", err)
	}
	want := "before\n<!-- BEGIN TEST -->\nnew\n<!-- END TEST -->\nafter\n"
	if got != want {
		t.Fatalf("updated content = %q, want %q", got, want)
	}
}

func TestUpdateManagedSectionRejectsMissingMarkers(t *testing.T) {
	_, err := UpdateManagedSection("# Sonarr\n", "TEST", "new")
	if err == nil || !strings.Contains(err.Error(), `managed section "TEST" not found`) {
		t.Fatalf("error = %v, want missing managed section", err)
	}
}

func TestCreateManagedSection(t *testing.T) {
	got := CreateManagedSection("TEST", "content")
	want := "<!-- BEGIN TEST -->\ncontent\n<!-- END TEST -->"
	if got != want {
		t.Fatalf("CreateManagedSection() = %q, want %q", got, want)
	}
}

func TestValidateManagedSections(t *testing.T) {
	content := strings.Join([]string{
		"<!-- BEGIN COMPLETE -->",
		"<!-- END COMPLETE -->",
		"<!-- BEGIN MISSING_END -->",
		"<!-- END MISSING_BEGIN -->",
	}, "\n")

	got := ValidateManagedSections(content)
	slices.Sort(got)
	want := []string{
		`missing BEGIN marker for "MISSING_BEGIN"`,
		`missing END marker for "MISSING_END"`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ValidateManagedSections() = %v, want %v", got, want)
	}
}
