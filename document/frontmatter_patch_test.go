package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestPatchFrontmatterSurgicallyInsertsIconAndPurpose(t *testing.T) {
	input := []byte("---\n# preserve this comment\ntitle: 'Sonarr' # and this one\nunknown: \"value\"\nsaltbox_automation:\n  app_links:\n    - name: Manual\n      url: null # optional by purpose\n      type: home\n---\n# Sonarr\n")
	icon := "material/docker"
	purpose := AppLinkPurposeManual
	want := []byte("---\nicon: material/docker\n# preserve this comment\ntitle: 'Sonarr' # and this one\nunknown: \"value\"\nsaltbox_automation:\n  app_links:\n    - name: Manual\n      url: null # optional by purpose\n      type: home\n      purpose: manual\n---\n# Sonarr\n")

	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{
		Icon:  &icon,
		Links: []AppLinkChange{{Index: 0, Purpose: &purpose}},
	})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if !changed {
		t.Fatal("PatchFrontmatter() changed = false, want true")
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("patched frontmatter:\n%s\nwant:\n%s", got, want)
	}
}

func TestPatchFrontmatterPreservesCRLFAndScalarStyle(t *testing.T) {
	input := []byte("---\r\nicon: null # icon note\r\nsaltbox_automation:\r\n  app_links:\r\n    - name: '   ' # name note\r\n      url: \"\" # url note\r\n      type: tags\r\n      purpose: release\r\n---\r\n# Releases\r\n")
	icon := "material/docker"
	name := "Image tags"
	url := "https://example.invalid/tags"
	want := []byte("---\r\nicon: material/docker # icon note\r\nsaltbox_automation:\r\n  app_links:\r\n    - name: 'Image tags' # name note\r\n      url: \"https://example.invalid/tags\" # url note\r\n      type: tags\r\n      purpose: release\r\n---\r\n# Releases\r\n")
	changes := FrontmatterChanges{
		Icon:  &icon,
		Links: []AppLinkChange{{Index: 0, Name: &name, URL: &url}},
	}

	got, changed, err := PatchFrontmatter(input, changes)
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if !changed || !bytes.Equal(got, want) {
		t.Fatalf("PatchFrontmatter() = changed %t, content:\n%s\nwant:\n%s", changed, got, want)
	}

	again, changed, err := PatchFrontmatter(got, changes)
	if err != nil {
		t.Fatalf("second PatchFrontmatter() error = %v", err)
	}
	if changed || !bytes.Equal(again, want) {
		t.Fatalf("second PatchFrontmatter() = changed %t, content:\n%s\nwant unchanged", changed, again)
	}
}

func TestPatchFrontmatterFillsImplicitNullBeforeInlineComment(t *testing.T) {
	input := []byte("---\nsaltbox_automation:\n  app_links:\n    - name: Releases\n      url:   # preserve this note\n      purpose: release\n---\n")
	url := "https://example.invalid/tags"
	want := []byte("---\nsaltbox_automation:\n  app_links:\n    - name: Releases\n      url: https://example.invalid/tags # preserve this note\n      purpose: release\n---\n")

	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{Links: []AppLinkChange{{Index: 0, URL: &url}}})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if !changed || !bytes.Equal(got, want) {
		t.Fatalf("PatchFrontmatter() = changed %t, content:\n%s\nwant:\n%s", changed, got, want)
	}
}

func TestPatchFrontmatterFillsScalarOnFollowingLine(t *testing.T) {
	input := []byte("---\nsaltbox_automation:\n  app_links:\n    - name: Releases\n      url:\n        null # preserve this note\n      purpose: release\n---\n")
	url := "https://example.invalid/tags"
	want := []byte("---\nsaltbox_automation:\n  app_links:\n    - name: Releases\n      url:\n        https://example.invalid/tags # preserve this note\n      purpose: release\n---\n")

	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{Links: []AppLinkChange{{Index: 0, URL: &url}}})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if !changed || !bytes.Equal(got, want) {
		t.Fatalf("PatchFrontmatter() = changed %t, content:\n%s\nwant:\n%s", changed, got, want)
	}
}

func TestPatchFrontmatterBlankDesiredMatchingBlankTargetIsUnchanged(t *testing.T) {
	input := []byte("---\nicon: \"\"\n---\n")
	icon := ""

	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{Icon: &icon})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if changed || !bytes.Equal(got, input) {
		t.Fatalf("PatchFrontmatter() = changed %t, content %q, want unchanged", changed, got)
	}
}

func TestPatchFrontmatterFillsMultilineQuotedWhitespaceScalars(t *testing.T) {
	icon := "material/docker"
	url := "https://example.invalid/tags"
	tests := []struct {
		name    string
		input   string
		want    string
		changes FrontmatterChanges
	}{
		{
			name:  "LF double quoted URL",
			input: "---\nsaltbox_automation:\n  app_links:\n    - name: Releases\n      url: \"\n        \" # URL note\n      type: tags\n      purpose: release\n---\n# Body\n",
			want:  "---\nsaltbox_automation:\n  app_links:\n    - name: Releases\n      url: \"https://example.invalid/tags\" # URL note\n      type: tags\n      purpose: release\n---\n# Body\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{
				Index: 0,
				URL:   &url,
			}}},
		},
		{
			name:  "CRLF single quoted URL",
			input: "---\r\nsaltbox_automation:\r\n  app_links:\r\n    - name: Releases\r\n      url: '\r\n        ' # URL note\r\n      type: tags\r\n      purpose: release\r\n---\r\n# Body\r\n",
			want:  "---\r\nsaltbox_automation:\r\n  app_links:\r\n    - name: Releases\r\n      url: 'https://example.invalid/tags' # URL note\r\n      type: tags\r\n      purpose: release\r\n---\r\n# Body\r\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{
				Index: 0,
				URL:   &url,
			}}},
		},
		{
			name:    "LF single quoted icon",
			input:   "---\nicon: '\n  ' # icon note\ntitle: 'Preserve me'\n---\n# Body\n",
			want:    "---\nicon: 'material/docker' # icon note\ntitle: 'Preserve me'\n---\n# Body\n",
			changes: FrontmatterChanges{Icon: &icon},
		},
		{
			name:    "CRLF double quoted icon",
			input:   "---\r\nicon: \"\r\n  \" # icon note\r\ntitle: \"Preserve me\"\r\n---\r\n# Body\r\n",
			want:    "---\r\nicon: \"material/docker\" # icon note\r\ntitle: \"Preserve me\"\r\n---\r\n# Body\r\n",
			changes: FrontmatterChanges{Icon: &icon},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := PatchFrontmatter([]byte(tt.input), tt.changes)
			if err != nil {
				t.Fatalf("PatchFrontmatter() error = %v", err)
			}
			if !changed || string(got) != tt.want {
				t.Fatalf("PatchFrontmatter() = changed %t, content %q, want %q", changed, got, tt.want)
			}

			again, changed, err := PatchFrontmatter(got, tt.changes)
			if err != nil {
				t.Fatalf("second PatchFrontmatter() error = %v", err)
			}
			if changed || string(again) != tt.want {
				t.Fatalf("second PatchFrontmatter() = changed %t, content %q, want unchanged", changed, again)
			}
		})
	}
}

func TestPatchFrontmatterPreservesNonblankMultilineQuotedAuthorScalars(t *testing.T) {
	icon := "material/docker"
	url := "https://generated.invalid/tags"
	tests := []struct {
		input   string
		changes FrontmatterChanges
	}{
		{
			input:   "---\nicon: \"author/\n  icon\" # author note\ntitle: App\n---\n",
			changes: FrontmatterChanges{Icon: &icon},
		},
		{
			input:   "---\r\nicon: 'author/\r\n  icon' # author note\r\ntitle: App\r\n---\r\n",
			changes: FrontmatterChanges{Icon: &icon},
		},
		{
			input:   "---\nsaltbox_automation:\n  app_links:\n    - name: Releases\n      url: \"https://author.invalid/\n        tags\" # author note\n      purpose: release\n---\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{Index: 0, URL: &url}}},
		},
		{
			input:   "---\r\nsaltbox_automation:\r\n  app_links:\r\n    - name: Releases\r\n      url: 'https://author.invalid/\r\n        tags' # author note\r\n      purpose: release\r\n---\r\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{Index: 0, URL: &url}}},
		},
	}

	for _, tt := range tests {
		got, changed, err := PatchFrontmatter([]byte(tt.input), tt.changes)
		if err != nil {
			t.Fatalf("PatchFrontmatter() error = %v", err)
		}
		if changed || string(got) != tt.input {
			t.Fatalf("PatchFrontmatter() = changed %t, content %q, want author input unchanged", changed, got)
		}
	}
}

func TestPatchFrontmatterPreservesQuotedDoubleAngleKey(t *testing.T) {
	input := []byte("---\n\"<<\": preserve this ordinary key\nicon: null\n---\n# Body\n")
	icon := "material/docker"
	want := []byte("---\n\"<<\": preserve this ordinary key\nicon: material/docker\n---\n# Body\n")

	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{Icon: &icon})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if !changed || !bytes.Equal(got, want) {
		t.Fatalf("PatchFrontmatter() = changed %t, content:\n%s\nwant:\n%s", changed, got, want)
	}
}

func TestPatchFrontmatterInsertsLinkFieldsInCanonicalOrder(t *testing.T) {
	input := []byte("---\nsaltbox_automation:\n  app_links:\n    - custom: keep\n      type: home\n---\n")
	name := "Manual"
	url := "https://example.invalid/docs"
	purpose := AppLinkPurposeManual
	want := []byte("---\nsaltbox_automation:\n  app_links:\n    - custom: keep\n      name: Manual\n      url: https://example.invalid/docs\n      type: home\n      purpose: manual\n---\n")

	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{Links: []AppLinkChange{{
		Index:   0,
		Name:    &name,
		URL:     &url,
		Purpose: &purpose,
	}}})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if !changed || !bytes.Equal(got, want) {
		t.Fatalf("PatchFrontmatter() = changed %t, content:\n%s\nwant:\n%s", changed, got, want)
	}
}

func TestPatchFrontmatterPreservesNonEmptyAuthorValues(t *testing.T) {
	input := []byte("---\nicon: author/icon\nsaltbox_automation:\n  app_links:\n    - name: Author name\n      url: https://author.invalid\n      type: author-type\n      purpose: other\n---\n")
	icon := "material/docker"
	name := "Generated name"
	url := "https://generated.invalid"
	purpose := AppLinkPurposeRelease

	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{
		Icon:  &icon,
		Links: []AppLinkChange{{Index: 0, Name: &name, URL: &url, Purpose: &purpose}},
	})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if changed || !bytes.Equal(got, input) {
		t.Fatalf("PatchFrontmatter() = changed %t, content:\n%s\nwant author input unchanged", changed, got)
	}
}

func TestPatchFrontmatterNoChangesSkipsMalformedInput(t *testing.T) {
	input := []byte("---\nsaltbox_automation: [\n")
	got, changed, err := PatchFrontmatter(input, FrontmatterChanges{})
	if err != nil {
		t.Fatalf("PatchFrontmatter() error = %v", err)
	}
	if changed || !bytes.Equal(got, input) {
		t.Fatalf("PatchFrontmatter() = changed %t, content %q, want unchanged", changed, got)
	}
}

func TestPatchFrontmatterRejectsUnsafeTargetsAtomically(t *testing.T) {
	icon := "material/docker"
	url := "https://example.invalid/tags"
	purpose := AppLinkPurposeRelease
	tests := []struct {
		name    string
		input   string
		changes FrontmatterChanges
	}{
		{
			name:    "malformed delimiters",
			input:   "---\nicon: null\n",
			changes: FrontmatterChanges{Icon: &icon},
		},
		{
			name:    "missing frontmatter",
			input:   "# App\n",
			changes: FrontmatterChanges{Icon: &icon},
		},
		{
			name:    "duplicate target key",
			input:   "---\nicon: null\nicon: null\n---\n",
			changes: FrontmatterChanges{Icon: &icon},
		},
		{
			name:  "flow target collection",
			input: "---\nsaltbox_automation:\n  app_links: [{name: Release}]\n---\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{
				Index: 0, Purpose: &purpose,
			}}},
		},
		{
			name:  "alias target",
			input: "---\nlink: &release\n  name: Release\nsaltbox_automation:\n  app_links:\n    - *release\n---\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{
				Index: 0, Purpose: &purpose,
			}}},
		},
		{
			name:  "merge key",
			input: "---\nlink: &release\n  name: Release\nsaltbox_automation:\n  app_links:\n    - <<: *release\n---\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{
				Index: 0, Purpose: &purpose,
			}}},
		},
		{
			name:  "block scalar target",
			input: "---\nsaltbox_automation:\n  app_links:\n    - url: |\n        \n---\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{
				Index: 0, URL: &url,
			}}},
		},
		{
			name:  "non scalar target",
			input: "---\nsaltbox_automation:\n  app_links:\n    - url: []\n---\n",
			changes: FrontmatterChanges{Links: []AppLinkChange{{
				Index: 0, URL: &url,
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(tt.input)
			got, changed, err := PatchFrontmatter(input, tt.changes)
			if err == nil {
				t.Fatal("PatchFrontmatter() error = nil, want rejection")
			}
			if changed {
				t.Fatal("PatchFrontmatter() changed = true on failure")
			}
			if !bytes.Equal(got, input) {
				t.Fatalf("PatchFrontmatter() content changed on failure:\n%s\nwant:\n%s", got, input)
			}
			if strings.TrimSpace(err.Error()) == "" {
				t.Fatal("PatchFrontmatter() returned an empty error")
			}
		})
	}
}
