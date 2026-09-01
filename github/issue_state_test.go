package github

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/health"
)

func TestIssueStateRoundTrip(t *testing.T) {
	want := health.State{
		Version: health.StateVersion,
		Results: []health.StateResult{
			{
				Kind:       health.MissingDocumentation,
				Enabled:    true,
				Exemptions: 2,
				Findings: []health.StateFinding{{
					ID:    "fixed-id",
					Kind:  health.MissingDocumentation,
					Label: "radarr",
				}},
			},
			{Kind: health.EditorialAttention, Enabled: false, Findings: []health.StateFinding{}},
		},
	}

	marker, err := encodeIssueState(want)
	if err != nil {
		t.Fatalf("encodeIssueState() error = %v", err)
	}
	if strings.Contains(marker, "=") {
		t.Fatalf("encodeIssueState() used padded base64url: %q", marker)
	}
	got, found, err := decodeIssueState("human content\n\n" + marker + "\n")
	if err != nil {
		t.Fatalf("decodeIssueState() error = %v", err)
	}
	if !found {
		t.Fatal("decodeIssueState() found = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded state = %#v, want %#v", got, want)
	}
}

func TestIssueStateDecodeNoMarker(t *testing.T) {
	got, found, err := decodeIssueState("legacy issue body")
	if err != nil {
		t.Fatalf("decodeIssueState() error = %v", err)
	}
	if found {
		t.Fatal("decodeIssueState() found = true, want false")
	}
	if !reflect.DeepEqual(got, health.State{}) {
		t.Fatalf("decodeIssueState() state = %#v, want zero state", got)
	}
}

func TestIssueStateDecodeRecognizesEveryVersionedMarker(t *testing.T) {
	valid, err := encodeIssueState(health.State{Version: health.StateVersion, Results: []health.StateResult{}})
	if err != nil {
		t.Fatalf("encodeIssueState() error = %v", err)
	}

	tests := []struct {
		name        string
		body        string
		wantErrText string
	}{
		{
			name:        "unsupported envelope version before payload decode",
			body:        "<!-- docs-automation-state-v2:DO-NOT-ECHO invalid payload -->",
			wantErrText: "unsupported marker version",
		},
		{
			name:        "mixed valid and unsupported markers",
			body:        valid + "\n<!-- docs-automation-state-v2:DO-NOT-ECHO -->",
			wantErrText: "multiple state markers",
		},
		{
			name:        "two valid markers",
			body:        valid + "\n" + valid,
			wantErrText: "multiple state markers",
		},
		{
			name:        "whitespace-corrupted v1 payload",
			body:        "<!-- docs-automation-state-v1:DO-NOT-ECHO invalid -->",
			wantErrText: "invalid base64url",
		},
		{
			name:        "malformed versioned marker",
			body:        "<!-- docs-automation-state-v1 DO-NOT-ECHO -->",
			wantErrText: "malformed state marker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found, err := decodeIssueState(tt.body)
			if err == nil {
				t.Fatal("decodeIssueState() error = nil, want marker rejection")
			}
			if !found {
				t.Fatal("decodeIssueState() found = false for a versioned marker prefix")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("decodeIssueState() error = %q, want text %q", err, tt.wantErrText)
			}
			if strings.Contains(err.Error(), "DO-NOT-ECHO") || strings.Contains(err.Error(), tt.body) {
				t.Fatalf("decodeIssueState() error echoed marker body or payload: %v", err)
			}
		})
	}
}

func TestIssueStateDecodeRejectsMalformedPayloadsWithoutEchoingBody(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "base64", payload: "%%%not-base64%%%"},
		{name: "gzip", payload: base64.RawURLEncoding.EncodeToString([]byte("not gzip"))},
		{name: "json", payload: compressedIssueStatePayload(t, []byte("not json"))},
		{name: "unsupported version", payload: compressedIssueStatePayload(t, []byte(`{"version":2,"results":[]}`))},
		{name: "trailing JSON", payload: compressedIssueStatePayload(t, []byte(`{"version":1,"results":[]} {}`))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const secret = "DO-NOT-ECHO-THIS-BODY"
			body := secret + "\n<!-- docs-automation-state-v1:" + tt.payload + " -->"
			_, found, err := decodeIssueState(body)
			if err == nil {
				t.Fatal("decodeIssueState() error = nil, want rejection")
			}
			if !found {
				t.Fatal("decodeIssueState() found = false for a present marker")
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), body) {
				t.Fatalf("decodeIssueState() error echoed issue body: %v", err)
			}
		})
	}
}

func TestIssueStateDecodeRejectsEncodedPayloadAboveLimit(t *testing.T) {
	payload := strings.Repeat("A", issueStateMaxEncodedSize+1)
	body := "<!-- docs-automation-state-v1:" + payload + " -->"

	_, found, err := decodeIssueState(body)
	if err == nil {
		t.Fatal("decodeIssueState() error = nil, want encoded-size rejection")
	}
	if !found {
		t.Fatal("decodeIssueState() found = false for an oversized marker")
	}
}

func TestIssueStateDecodeRejectsDecompressedPayloadAboveLimit(t *testing.T) {
	jsonPrefix := `{"version":1,"results":[],"padding":"`
	jsonSuffix := `"}`
	oversized := jsonPrefix + strings.Repeat("x", issueStateMaxDecodedSize) + jsonSuffix
	payload := compressedIssueStatePayload(t, []byte(oversized))

	_, found, err := decodeIssueState("<!-- docs-automation-state-v1:" + payload + " -->")
	if err == nil {
		t.Fatal("decodeIssueState() error = nil, want decompressed-size rejection")
	}
	if !found {
		t.Fatal("decodeIssueState() found = false for an oversized decompressed payload")
	}
}

func TestIssueStateEncodeRejectsOversizedState(t *testing.T) {
	state := health.State{
		Version: health.StateVersion,
		Results: []health.StateResult{{
			Kind:    health.EditorialAttention,
			Enabled: true,
			Findings: []health.StateFinding{{
				ID:    strings.Repeat("x", issueStateMaxDecodedSize),
				Kind:  health.EditorialAttention,
				Label: "large",
			}},
		}},
	}

	if _, err := encodeIssueState(state); err == nil {
		t.Fatal("encodeIssueState() error = nil, want decompressed-size rejection")
	}
}

func compressedIssueStatePayload(t *testing.T, data []byte) string {
	t.Helper()
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("compressing test payload: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing test gzip writer: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(compressed.Bytes())
}
