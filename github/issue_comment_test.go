package github

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/saltyorg/docs-automation/health"
)

var testIssueCommentStateHash = regexp.MustCompile(`docs-automation-state-sha256:[0-9a-f]{64}`)

func TestIssueRendererUpdateCommentGolden(t *testing.T) {
	oldState := health.State{
		Version: health.StateVersion,
		Results: []health.StateResult{
			{
				Kind:    health.MissingDocumentation,
				Enabled: true,
				Findings: []health.StateFinding{
					{ID: "radarr-id", Kind: health.MissingDocumentation, Label: "radarr"},
					{ID: "sonarr-id", Kind: health.MissingDocumentation, Label: "sonarr"},
				},
			},
			{
				Kind:    health.EditorialAttention,
				Enabled: true,
				Findings: []health.StateFinding{
					{ID: "draft-id", Kind: health.EditorialAttention, Label: "draft page"},
				},
			},
		},
	}
	newState := health.State{
		Version: health.StateVersion,
		Results: []health.StateResult{
			{
				Kind:    health.MissingDocumentation,
				Enabled: true,
				Findings: []health.StateFinding{
					{ID: "lidarr-id", Kind: health.MissingDocumentation, Label: "lidarr"},
					{ID: "radarr-id", Kind: health.MissingDocumentation, Label: "radarr"},
				},
			},
			{
				Kind:    health.EditorialAttention,
				Enabled: true,
				Findings: []health.StateFinding{
					{ID: "draft-id", Kind: health.EditorialAttention, Label: "draft page"},
				},
			},
		},
	}
	run := health.RunInfo{
		CheckedAt:   time.Date(2026, 9, 1, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60)),
		WorkflowURL: "https://github.com/saltyorg/docs/actions/runs/123456",
	}

	comment := NewIssueRenderer("saltyorg/docs").UpdateComment(oldState, newState, run)
	wantMarker := "<!-- docs-automation-state-sha256:" + testIssueStateHash(t, newState) + " -->"
	if !strings.Contains(comment, wantMarker) {
		t.Fatalf("UpdateComment() has no canonical state marker %q:\n%s", wantMarker, comment)
	}
	normalized := testIssueCommentStateHash.ReplaceAllString(comment, "docs-automation-state-sha256:HASH")
	want, err := os.ReadFile("testdata/issue_comment.golden.md")
	if err != nil {
		t.Fatalf("reading golden file: %v", err)
	}
	if normalized != string(want) {
		t.Fatalf("UpdateComment() differs from golden output\n--- got ---\n%s\n--- want ---\n%s", normalized, want)
	}
	if strings.Contains(comment, "Editorial Attention") {
		t.Fatalf("UpdateComment() included an unchanged result row:\n%s", comment)
	}
}

func TestIssueRendererUpdateCommentCapsAddedAndResolvedFindings(t *testing.T) {
	oldFindings := make([]health.StateFinding, 30)
	newFindings := make([]health.StateFinding, 30)
	for i := range 30 {
		oldFindings[i] = health.StateFinding{
			ID:    "old-" + twoDigits(i),
			Kind:  health.OrphanedDocumentation,
			Label: "resolved-" + twoDigits(i),
		}
		newFindings[i] = health.StateFinding{
			ID:    "new-" + twoDigits(i),
			Kind:  health.MissingDocumentation,
			Label: "added-" + twoDigits(i),
		}
	}
	oldState := health.State{Version: health.StateVersion, Results: []health.StateResult{{
		Kind: health.OrphanedDocumentation, Enabled: true, Findings: oldFindings,
	}}}
	newState := health.State{Version: health.StateVersion, Results: []health.StateResult{{
		Kind: health.MissingDocumentation, Enabled: true, Findings: newFindings,
	}}}

	comment := NewIssueRenderer("saltyorg/docs").UpdateComment(oldState, newState, health.RunInfo{})
	for _, want := range []string{
		"- Missing Documentation: added-24",
		"5 additional added findings omitted.",
		"- Orphaned Documentation: resolved-24",
		"5 additional resolved findings omitted.",
	} {
		if !strings.Contains(comment, want) {
			t.Errorf("UpdateComment() missing %q:\n%s", want, comment)
		}
	}
	for _, notWant := range []string{"added-25", "resolved-25"} {
		if strings.Contains(comment, notWant) {
			t.Errorf("UpdateComment() rendered finding beyond cap %q:\n%s", notWant, comment)
		}
	}
}

func TestIssueRendererUpdateCommentEscapesLiteralMarkdownCharacters(t *testing.T) {
	oldState := health.State{Version: health.StateVersion, Results: []health.StateResult{{
		Kind: health.MissingDocumentation, Enabled: true,
	}}}
	newState := health.State{Version: health.StateVersion, Results: []health.StateResult{{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.StateFinding{{
			ID: "literal-markdown", Kind: health.MissingDocumentation, Label: "role*under_score~old",
		}},
	}}}

	comment := NewIssueRenderer("saltyorg/docs").UpdateComment(oldState, newState, health.RunInfo{})
	if !strings.Contains(comment, `role\*under\_score\~old`) {
		t.Fatalf("UpdateComment() did not render the label literally:\n%s", comment)
	}
}

func testIssueStateHash(t *testing.T, state health.State) string {
	t.Helper()
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshalling test state: %v", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
