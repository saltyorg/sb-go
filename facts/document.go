package facts

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// document keeps source lines so editing one fact cannot reformat or reinterpret
// unrelated values. Values follow Saltbox's ConfigParser conventions: only '='
// separates keys, quotes and inline comment symbols are literal, and indented
// continuation lines form multiline values. Semicolon comments from the legacy
// Go writer remain accepted and preserved alongside Python's hash comments.
type document struct {
	lines    []string
	sections []*section
	newline  string
}
type section struct {
	name       string
	start, end int
	keys       []*entry
}
type entry struct {
	name, value        string
	start, end, indent int
}
type lineEdit struct {
	start, end int
	text       string
}

func parseDocument(data []byte) (*document, error) {
	if !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
		return nil, fmt.Errorf("facts must be UTF-8 text without NUL")
	}
	doc := &document{lines: strings.SplitAfter(string(data), "\n"), newline: "\n"}
	if strings.Contains(string(data), "\r\n") {
		doc.newline = "\r\n"
	}
	current := &section{name: "DEFAULT", start: 0}
	doc.sections = append(doc.sections, current)
	var previous *entry
	seenSections := map[string]bool{}
	for i, line := range doc.lines {
		raw := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			previous = nil
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
		if previous != nil && indent > previous.indent {
			previous.value += "\n" + trimmed
			previous.end = i + 1
			continue
		}
		previous = nil
		if strings.HasPrefix(trimmed, "[") {
			end := strings.LastIndex(trimmed, "]")
			if end < 2 {
				return nil, fmt.Errorf("malformed section at line %d", i+1)
			}
			name := trimmed[1:end]
			if tail := strings.TrimSpace(trimmed[end+1:]); tail != "" && !strings.HasPrefix(tail, "#") && !strings.HasPrefix(tail, ";") {
				return nil, fmt.Errorf("unexpected section suffix at line %d", i+1)
			}
			if strings.IndexFunc(name, unicode.IsControl) >= 0 || strings.ContainsAny(name, "[]") {
				return nil, fmt.Errorf("invalid section at line %d", i+1)
			}
			if seenSections[name] {
				return nil, fmt.Errorf("duplicate instance %q", name)
			}
			seenSections[name] = true
			current.end = i
			if name == "DEFAULT" && len(doc.sections) == 1 && len(current.keys) == 0 {
				current.start = i
			} else {
				current = &section{name: name, start: i}
				doc.sections = append(doc.sections, current)
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.IndexFunc(key, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("invalid fact at line %d", i+1)
		}
		if current.findKey(key) != nil {
			return nil, fmt.Errorf("duplicate key %q in %s", key, current.name)
		}
		previous = &entry{name: key, value: strings.TrimSpace(value), start: i, end: i + 1, indent: indent}
		current.keys = append(current.keys, previous)
	}
	current.end = len(doc.lines)
	return doc, nil
}

func (d *document) findSection(name string) *section {
	for _, section := range d.sections {
		if section.name == name {
			return section
		}
	}
	return nil
}
func (s *section) findKey(name string) *entry {
	for _, key := range s.keys {
		if key.name == name {
			return key
		}
	}
	return nil
}

func (d *document) catalog(name string) Role {
	role := Role{Name: name}
	for _, section := range d.sections {
		if section.name == "DEFAULT" {
			continue
		}
		instance := Instance{Name: section.name}
		for _, key := range section.keys {
			instance.Facts = append(instance.Facts, Fact{Key: key.name, Value: key.value})
		}
		slices.SortFunc(instance.Facts, func(a, b Fact) int { return strings.Compare(a.Key, b.Key) })
		role.Instances = append(role.Instances, instance)
	}
	slices.SortFunc(role.Instances, func(a, b Instance) int { return strings.Compare(a.Name, b.Name) })
	return role
}

func (d *document) apply(changes []Change) ([]byte, error) {
	var edits []lineEdit
	for _, change := range changes {
		section := d.findSection(change.Instance)
		if change.Kind == DeleteInstance {
			edits = append(edits, lineEdit{start: section.start, end: section.end})
			continue
		}
		key := section.findKey(change.Key)
		var comments strings.Builder
		if key != nil {
			for _, line := range d.lines[key.start+1 : key.end] {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
					comments.WriteString(line)
				}
			}
		}
		if change.Kind == DeleteFact {
			edits = append(edits, lineEdit{start: key.start, end: key.end, text: comments.String()})
			continue
		}
		if key != nil && key.value == change.Value {
			continue
		}
		valueLines := strings.Split(change.Value, "\n")
		for _, line := range valueLines {
			if line != strings.TrimSpace(line) || line == "" && len(valueLines) > 1 {
				return nil, fmt.Errorf("value whitespace cannot be represented without loss")
			}
		}
		prefix := change.Key + " = "
		start, end := section.end, section.end
		if key != nil {
			start, end = key.start, key.end
			before, _, _ := strings.Cut(d.lines[start], "=")
			rest := strings.TrimSuffix(strings.TrimSuffix(strings.SplitN(d.lines[start], "=", 2)[1], "\n"), "\r")
			prefix = before + "=" + rest[:len(rest)-len(strings.TrimLeftFunc(rest, unicode.IsSpace))]
		}
		text := prefix + strings.Join(valueLines, d.newline+"\t") + d.newline
		if key == nil && start > 0 && !strings.HasSuffix(d.lines[start-1], "\n") {
			text = d.newline + text
		}
		if key != nil && end == len(d.lines) && !strings.HasSuffix(d.lines[end-1], "\n") {
			text = strings.TrimSuffix(text, d.newline)
		}
		edits = append(edits, lineEdit{start: start, end: end, text: text + comments.String()})
	}
	slices.SortStableFunc(edits, func(a, b lineEdit) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		// An insertion at the next section's boundary precedes its deletion.
		return a.end - b.end
	})
	var result strings.Builder
	cursor := 0
	for _, edit := range edits {
		for _, line := range d.lines[cursor:edit.start] {
			result.WriteString(line)
		}
		result.WriteString(edit.text)
		cursor = edit.end
	}
	for _, line := range d.lines[cursor:] {
		result.WriteString(line)
	}
	return []byte(result.String()), nil
}
