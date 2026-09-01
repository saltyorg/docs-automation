package github

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/saltyorg/docs-automation/health"
)

const (
	issueStateMaxEncodedSize = 256 << 10
	issueStateMaxDecodedSize = 1 << 20
)

const issueStateMarkerPrefix = "docs-automation-state-v"

var issueStateMarkerPattern = regexp.MustCompile(`(?m)^<!-- docs-automation-state-v([0-9]+):([^\r\n]*) -->$`)

func encodeIssueState(state health.State) (string, error) {
	if state.Version != health.StateVersion {
		return "", fmt.Errorf("encoding issue state: unsupported version %d", state.Version)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encoding issue state JSON: %w", err)
	}
	if len(data) > issueStateMaxDecodedSize {
		return "", fmt.Errorf("encoding issue state: JSON exceeds %d bytes", issueStateMaxDecodedSize)
	}

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		return "", fmt.Errorf("compressing issue state: %w", err)
	}
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("closing issue state compressor: %w", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(compressed.Bytes())
	if len(payload) > issueStateMaxEncodedSize {
		return "", fmt.Errorf("encoding issue state: payload exceeds %d bytes", issueStateMaxEncodedSize)
	}
	return "<!-- docs-automation-state-v1:" + payload + " -->", nil
}

func decodeIssueState(body string) (health.State, bool, error) {
	prefixCount := strings.Count(body, issueStateMarkerPrefix)
	if prefixCount == 0 {
		return health.State{}, false, nil
	}
	matches := issueStateMarkerPattern.FindAllStringSubmatch(body, prefixCount+1)
	if len(matches) != prefixCount {
		return health.State{}, true, fmt.Errorf("decoding issue state: malformed state marker")
	}
	if len(matches) != 1 {
		return health.State{}, true, fmt.Errorf("decoding issue state: multiple state markers")
	}
	if matches[0][1] != "1" {
		return health.State{}, true, fmt.Errorf("decoding issue state: unsupported marker version")
	}

	payload := matches[0][2]
	if len(payload) > issueStateMaxEncodedSize {
		return health.State{}, true, fmt.Errorf("decoding issue state: payload exceeds %d bytes", issueStateMaxEncodedSize)
	}
	compressed, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return health.State{}, true, fmt.Errorf("decoding issue state payload: invalid base64url")
	}

	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return health.State{}, true, fmt.Errorf("decoding issue state payload: invalid gzip data")
	}
	data, readErr := io.ReadAll(io.LimitReader(zr, issueStateMaxDecodedSize+1))
	closeErr := zr.Close()
	if readErr != nil {
		return health.State{}, true, fmt.Errorf("decoding issue state payload: invalid gzip stream")
	}
	if closeErr != nil {
		return health.State{}, true, fmt.Errorf("decoding issue state payload: closing gzip stream")
	}
	if len(data) > issueStateMaxDecodedSize {
		return health.State{}, true, fmt.Errorf("decoding issue state: JSON exceeds %d bytes", issueStateMaxDecodedSize)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var state health.State
	if err := decoder.Decode(&state); err != nil {
		return health.State{}, true, fmt.Errorf("decoding issue state JSON: invalid document")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return health.State{}, true, fmt.Errorf("decoding issue state JSON: trailing data")
	}
	if state.Version != health.StateVersion {
		return health.State{}, true, fmt.Errorf("decoding issue state: unsupported state version")
	}
	return state, true, nil
}
