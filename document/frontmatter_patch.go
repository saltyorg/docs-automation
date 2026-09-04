package document

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FrontmatterChanges describes fill-only frontmatter edits.
type FrontmatterChanges struct {
	Icon  *string
	Links []AppLinkChange
}

// AppLinkChange describes fill-only edits for one app link by index.
type AppLinkChange struct {
	Index   int
	Name    *string
	URL     *string
	Purpose *AppLinkPurpose
}

type frontmatterEdit struct {
	start       int
	end         int
	replacement []byte
}

type frontmatterSource struct {
	content    []byte
	span       frontmatterSpan
	raw        []byte
	lineStarts []int
}

// PatchFrontmatter fills blank requested fields without reserializing the YAML.
func PatchFrontmatter(content []byte, changes FrontmatterChanges) ([]byte, bool, error) {
	if !changes.requested() {
		return content, false, nil
	}

	span, found, err := locateFrontmatter(content)
	if err != nil {
		return content, false, err
	}
	if !found {
		return content, false, fmt.Errorf("frontmatter is required for mutation")
	}

	source := newFrontmatterSource(content, span)
	root, err := parseFrontmatterNode(source.raw)
	if err != nil {
		return content, false, err
	}
	if err := requireBlockCollection(root, yaml.MappingNode, "frontmatter"); err != nil {
		return content, false, err
	}

	edits := make([]frontmatterEdit, 0, 1+len(changes.Links)*3)
	if changes.Icon != nil {
		iconEdits, err := source.planRootScalarFill(root, "icon", *changes.Icon)
		if err != nil {
			return content, false, err
		}
		edits = append(edits, iconEdits...)
	}

	linkChanges := make([]AppLinkChange, 0, len(changes.Links))
	seenIndexes := make(map[int]struct{}, len(changes.Links))
	for _, change := range changes.Links {
		if !change.requested() {
			continue
		}
		if change.Index < 0 {
			return content, false, fmt.Errorf("app link index %d is invalid", change.Index)
		}
		if _, exists := seenIndexes[change.Index]; exists {
			return content, false, fmt.Errorf("app link index %d has duplicate changes", change.Index)
		}
		seenIndexes[change.Index] = struct{}{}
		linkChanges = append(linkChanges, change)
	}
	sort.Slice(linkChanges, func(i, j int) bool { return linkChanges[i].Index < linkChanges[j].Index })

	if len(linkChanges) > 0 {
		linkEdits, err := source.planLinkFills(root, linkChanges)
		if err != nil {
			return content, false, err
		}
		edits = append(edits, linkEdits...)
	}

	if len(edits) == 0 {
		return content, false, nil
	}
	result, err := applyFrontmatterEdits(content, edits)
	if err != nil {
		return content, false, err
	}
	if _, _, err := ParseFrontmatter(string(result)); err != nil {
		return content, false, fmt.Errorf("reparsing patched frontmatter: %w", err)
	}
	return result, true, nil
}

func (c FrontmatterChanges) requested() bool {
	if c.Icon != nil {
		return true
	}
	for _, link := range c.Links {
		if link.requested() {
			return true
		}
	}
	return false
}

func (c AppLinkChange) requested() bool {
	return c.Name != nil || c.URL != nil || c.Purpose != nil
}

func newFrontmatterSource(content []byte, span frontmatterSpan) frontmatterSource {
	raw := content[span.yamlStart:span.yamlEnd]
	lineStarts := []int{0}
	for i, b := range raw {
		if b == '\n' && i+1 < len(raw) {
			lineStarts = append(lineStarts, i+1)
		}
	}
	return frontmatterSource{content: content, span: span, raw: raw, lineStarts: lineStarts}
}

func parseFrontmatterNode(raw []byte) (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parsing frontmatter YAML for mutation: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("frontmatter must contain one YAML document")
	}
	return document.Content[0], nil
}

func requireBlockCollection(node *yaml.Node, kind yaml.Kind, label string) error {
	if node == nil || node.Kind == yaml.AliasNode {
		return fmt.Errorf("%s aliases are not supported", label)
	}
	if node.Kind != kind {
		return fmt.Errorf("%s must be a %s", label, yamlKindName(kind))
	}
	if node.Style&yaml.FlowStyle != 0 {
		return fmt.Errorf("%s must use block YAML style", label)
	}
	return nil
}

func yamlKindName(kind yaml.Kind) string {
	switch kind {
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	default:
		return "supported collection"
	}
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node, bool, error) {
	var keyNode, valueNode *yaml.Node
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		candidate := mapping.Content[i]
		if candidate.Tag == "!!merge" || candidate.Value == "<<" {
			return nil, nil, false, fmt.Errorf("merge keys are not supported in target mapping")
		}
		if candidate.Kind == yaml.ScalarNode && candidate.Value == key {
			if keyNode != nil {
				return nil, nil, false, fmt.Errorf("target key %q is duplicated", key)
			}
			keyNode = candidate
			valueNode = mapping.Content[i+1]
		}
	}
	return keyNode, valueNode, keyNode != nil, nil
}

func (s frontmatterSource) planRootScalarFill(root *yaml.Node, key, value string) ([]frontmatterEdit, error) {
	keyNode, valueNode, found, err := mappingValue(root, key)
	if err != nil {
		return nil, err
	}
	if found {
		edit, needed, err := s.planExistingScalarFill(keyNode, valueNode, value, key)
		if err != nil || !needed {
			return nil, err
		}
		return []frontmatterEdit{edit}, nil
	}

	encoded, err := encodeInsertedScalar(value)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", key, err)
	}
	replacement := []byte(key + ": " + encoded)
	replacement = append(replacement, s.span.lineBreak...)
	return []frontmatterEdit{{
		start:       s.span.yamlStart,
		end:         s.span.yamlStart,
		replacement: replacement,
	}}, nil
}

func (s frontmatterSource) planLinkFills(root *yaml.Node, changes []AppLinkChange) ([]frontmatterEdit, error) {
	_, automation, found, err := mappingValue(root, "saltbox_automation")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("saltbox_automation is required for app link mutation")
	}
	if err := requireBlockCollection(automation, yaml.MappingNode, "saltbox_automation"); err != nil {
		return nil, err
	}

	_, links, found, err := mappingValue(automation, "app_links")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("app_links is required for app link mutation")
	}
	if err := requireBlockCollection(links, yaml.SequenceNode, "app_links"); err != nil {
		return nil, err
	}

	edits := make([]frontmatterEdit, 0, len(changes)*3)
	for _, change := range changes {
		if change.Index >= len(links.Content) {
			return nil, fmt.Errorf("app link index %d is out of range", change.Index)
		}
		link := links.Content[change.Index]
		if err := requireBlockCollection(link, yaml.MappingNode, fmt.Sprintf("app_links[%d]", change.Index)); err != nil {
			return nil, err
		}
		linkEdits, err := s.planOneLinkFill(link, change)
		if err != nil {
			return nil, fmt.Errorf("app_links[%d]: %w", change.Index, err)
		}
		edits = append(edits, linkEdits...)
	}
	return edits, nil
}

type linkField struct {
	name    string
	desired *string
	key     *yaml.Node
	value   *yaml.Node
}

func (s frontmatterSource) planOneLinkFill(link *yaml.Node, change AppLinkChange) ([]frontmatterEdit, error) {
	purpose := (*string)(nil)
	if change.Purpose != nil {
		value := string(*change.Purpose)
		purpose = &value
	}
	fields := []linkField{
		{name: "name", desired: change.Name},
		{name: "url", desired: change.URL},
		{name: "type"},
		{name: "purpose", desired: purpose},
	}

	for i := range fields {
		keyNode, valueNode, _, err := mappingValue(link, fields[i].name)
		if err != nil {
			return nil, err
		}
		fields[i].key = keyNode
		fields[i].value = valueNode
	}

	edits := make([]frontmatterEdit, 0, 3)
	missing := make(map[int][]linkField)
	for i, field := range fields {
		if field.desired == nil {
			continue
		}
		if field.key != nil {
			edit, needed, err := s.planExistingScalarFill(field.key, field.value, *field.desired, field.name)
			if err != nil {
				return nil, err
			}
			if needed {
				edits = append(edits, edit)
			}
			continue
		}

		anchor := len(fields)
		for j := i + 1; j < len(fields); j++ {
			if fields[j].key != nil {
				anchor = j
				break
			}
		}
		missing[anchor] = append(missing[anchor], field)
	}

	anchors := make([]int, 0, len(missing))
	for anchor := range missing {
		anchors = append(anchors, anchor)
	}
	sort.Ints(anchors)
	for _, anchor := range anchors {
		var edit frontmatterEdit
		var err error
		if anchor < len(fields) {
			edit, err = s.insertBeforeMappingKey(link, fields[anchor].key, missing[anchor])
		} else {
			edit, err = s.appendMappingFields(link, missing[anchor])
		}
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit)
	}
	return edits, nil
}

func (s frontmatterSource) planExistingScalarFill(keyNode, valueNode *yaml.Node, desired, label string) (frontmatterEdit, bool, error) {
	if valueNode == nil || valueNode.Kind == yaml.AliasNode {
		return frontmatterEdit{}, false, fmt.Errorf("%s aliases are not supported", label)
	}
	if valueNode.Kind != yaml.ScalarNode {
		return frontmatterEdit{}, false, fmt.Errorf("%s target must be a scalar", label)
	}
	if !blankScalar(valueNode) {
		return frontmatterEdit{}, false, nil
	}
	if valueNode.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return frontmatterEdit{}, false, fmt.Errorf("%s block scalar targets are not supported", label)
	}
	start, end, implicit, commentFollows, err := s.scalarTokenRange(keyNode, valueNode)
	if err != nil {
		return frontmatterEdit{}, false, fmt.Errorf("locating %s scalar: %w", label, err)
	}
	encoded, err := encodeScalarWithStyle(desired, valueNode.Style)
	if err != nil {
		return frontmatterEdit{}, false, fmt.Errorf("encoding %s scalar: %w", label, err)
	}
	if implicit {
		encoded = " " + encoded
		if commentFollows {
			encoded += " "
		}
	}
	if bytes.Equal(s.content[start:end], []byte(encoded)) {
		return frontmatterEdit{}, false, nil
	}
	return frontmatterEdit{start: start, end: end, replacement: []byte(encoded)}, true, nil
}

func blankScalar(node *yaml.Node) bool {
	return node.Tag == "!!null" || strings.TrimSpace(node.Value) == ""
}

func encodeScalarWithStyle(value string, style yaml.Style) (string, error) {
	switch {
	case style&yaml.SingleQuotedStyle != 0:
		return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
	case style&yaml.DoubleQuotedStyle != 0:
		return strconv.Quote(value), nil
	default:
		return encodeInsertedScalar(value)
	}
}

func encodeInsertedScalar(value string) (string, error) {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return "", err
	}
	encoded = bytes.TrimSuffix(encoded, []byte("\n"))
	if bytes.ContainsAny(encoded, "\r\n") {
		return "", fmt.Errorf("multiline scalar values are not supported")
	}
	return string(encoded), nil
}

func (s frontmatterSource) scalarTokenRange(keyNode, valueNode *yaml.Node) (start, end int, implicit, commentFollows bool, err error) {
	if valueNode.Line > keyNode.Line {
		lineStart, lineEnd, lineErr := s.lineRange(valueNode.Line)
		if lineErr != nil {
			return 0, 0, false, false, lineErr
		}
		tokenStart := lineStart + valueNode.Column - 1
		if tokenStart < lineStart || tokenStart >= lineEnd {
			return 0, 0, false, false, fmt.Errorf("invalid value column %d", valueNode.Column)
		}
		line := s.content[lineStart:lineEnd]
		comment := yamlCommentStart(line, valueNode.Column-1)
		tokenEnd := lineEnd
		if comment >= 0 {
			tokenEnd = lineStart + comment
			commentFollows = true
		}
		for tokenEnd > tokenStart && (s.content[tokenEnd-1] == ' ' || s.content[tokenEnd-1] == '\t') {
			tokenEnd--
		}
		return tokenStart, tokenEnd, false, commentFollows, nil
	}

	lineStart, lineEnd, err := s.lineRange(keyNode.Line)
	if err != nil {
		return 0, 0, false, false, err
	}
	line := s.content[lineStart:lineEnd]
	keyOffset := keyNode.Column - 1
	if keyOffset < 0 || keyOffset >= len(line) {
		return 0, 0, false, false, fmt.Errorf("invalid key column %d", keyNode.Column)
	}
	colon := bytes.IndexByte(line[keyOffset:], ':')
	if colon == -1 {
		return 0, 0, false, false, fmt.Errorf("key has no value delimiter")
	}
	colon += keyOffset
	areaStart := colon + 1
	comment := yamlCommentStart(line, areaStart)
	areaEnd := len(line)
	if comment >= 0 {
		areaEnd = comment
		commentFollows = true
	}
	tokenStart := areaStart
	for tokenStart < areaEnd && (line[tokenStart] == ' ' || line[tokenStart] == '\t') {
		tokenStart++
	}
	tokenEnd := areaEnd
	for tokenEnd > tokenStart && (line[tokenEnd-1] == ' ' || line[tokenEnd-1] == '\t') {
		tokenEnd--
	}
	if tokenStart == tokenEnd {
		return lineStart + areaStart, lineStart + areaEnd, true, commentFollows, nil
	}
	return lineStart + tokenStart, lineStart + tokenEnd, false, commentFollows, nil
}

func yamlCommentStart(line []byte, start int) int {
	var quote byte
	escaped := false
	for i := start; i < len(line); i++ {
		b := line[i]
		if quote == '"' {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				continue
			}
			if b == quote {
				quote = 0
			}
			continue
		}
		if quote == '\'' {
			if b == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		if b == '\'' || b == '"' {
			quote = b
			continue
		}
		if b == '#' && (i == start || line[i-1] == ' ' || line[i-1] == '\t') {
			return i
		}
	}
	return -1
}

func (s frontmatterSource) insertBeforeMappingKey(mapping, key *yaml.Node, fields []linkField) (frontmatterEdit, error) {
	lineStart, _, err := s.lineRange(key.Line)
	if err != nil {
		return frontmatterEdit{}, err
	}
	keyStart := lineStart + key.Column - 1
	indent := strings.Repeat(" ", key.Column-1)
	start := lineStart
	firstPrefix := indent
	trailingPrefix := ""
	if len(mapping.Content) > 0 && mapping.Content[0] == key {
		prefix := s.content[lineStart:keyStart]
		if bytes.Contains(prefix, []byte("- ")) {
			start = keyStart
			firstPrefix = ""
			trailingPrefix = indent
		}
	}

	replacement, err := s.renderInsertedFields(fields, firstPrefix, indent)
	if err != nil {
		return frontmatterEdit{}, err
	}
	replacement = append(replacement, trailingPrefix...)
	return frontmatterEdit{start: start, end: start, replacement: replacement}, nil
}

func (s frontmatterSource) appendMappingFields(mapping *yaml.Node, fields []linkField) (frontmatterEdit, error) {
	if len(mapping.Content) == 0 {
		return frontmatterEdit{}, fmt.Errorf("empty link mappings are not supported")
	}
	lastLine := maxNodeLine(mapping)
	start, err := s.lineFollowing(lastLine)
	if err != nil {
		return frontmatterEdit{}, err
	}
	if start > s.span.yamlEnd {
		return frontmatterEdit{}, fmt.Errorf("link mapping ends outside frontmatter")
	}
	indent := strings.Repeat(" ", mapping.Content[0].Column-1)
	replacement, err := s.renderInsertedFields(fields, indent, indent)
	if err != nil {
		return frontmatterEdit{}, err
	}
	return frontmatterEdit{start: start, end: start, replacement: replacement}, nil
}

func (s frontmatterSource) renderInsertedFields(fields []linkField, firstPrefix, otherPrefix string) ([]byte, error) {
	var replacement []byte
	for i, field := range fields {
		encoded, err := encodeInsertedScalar(*field.desired)
		if err != nil {
			return nil, fmt.Errorf("encoding %s: %w", field.name, err)
		}
		prefix := otherPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		replacement = append(replacement, prefix...)
		replacement = append(replacement, field.name...)
		replacement = append(replacement, ':', ' ')
		replacement = append(replacement, encoded...)
		replacement = append(replacement, s.span.lineBreak...)
	}
	return replacement, nil
}

func maxNodeLine(node *yaml.Node) int {
	line := node.Line
	for _, child := range node.Content {
		line = max(line, maxNodeLine(child))
	}
	return line
}

func (s frontmatterSource) lineRange(line int) (start, end int, err error) {
	if line <= 0 || line > len(s.lineStarts) {
		return 0, 0, fmt.Errorf("invalid YAML line %d", line)
	}
	start = s.span.yamlStart + s.lineStarts[line-1]
	end, _, _ = frontmatterLine(s.content, start)
	return start, end, nil
}

func (s frontmatterSource) lineFollowing(line int) (int, error) {
	start, _, err := s.lineRange(line)
	if err != nil {
		return 0, err
	}
	_, next, lineBreak := frontmatterLine(s.content, start)
	if len(lineBreak) == 0 {
		return 0, fmt.Errorf("YAML line %d has no line ending", line)
	}
	return next, nil
}

func applyFrontmatterEdits(content []byte, edits []frontmatterEdit) ([]byte, error) {
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].start == edits[j].start {
			return edits[i].end > edits[j].end
		}
		return edits[i].start > edits[j].start
	})
	previousStart := len(content)
	for _, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(content) {
			return nil, fmt.Errorf("frontmatter edit range %d:%d is invalid", edit.start, edit.end)
		}
		if edit.end > previousStart {
			return nil, fmt.Errorf("frontmatter edits overlap")
		}
		previousStart = edit.start
	}

	result := bytes.Clone(content)
	for _, edit := range edits {
		updated := make([]byte, 0, len(result)-(edit.end-edit.start)+len(edit.replacement))
		updated = append(updated, result[:edit.start]...)
		updated = append(updated, edit.replacement...)
		updated = append(updated, result[edit.end:]...)
		result = updated
	}
	return result, nil
}
