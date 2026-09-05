package factui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/saltyorg/sb-go/facts"
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Faint(true)
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
)

// display makes control bytes visible instead of allowing fact data to issue
// terminal commands. Ordinary values, including credentials, stay plaintext.
func display(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *Model) treeView(width, height int) string {
	rows := m.rows()
	if len(rows) == 0 {
		return "No matching facts"
	}
	m.selected()
	start := max(0, m.cursor-height+1)
	var lines []string
	for i := start; i < min(len(rows), start+height); i++ {
		n := rows[i]
		label := n.role
		prefix := "▸ "
		if m.expanded[n] || m.filter != "" {
			prefix = "▾ "
		}
		if n.instance != "" {
			label = n.instance
			prefix = "  " + prefix
		}
		if n.key != "" {
			label = n.key
			prefix = "    · "
		}
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		status := ""
		if m.marked(n) {
			status = " [delete]"
		} else if _, ok := m.edits[n]; ok {
			original, exists := m.baseline(n)
			if !exists || original != m.edits[n] {
				status = " *"
			}
		}
		line := ansi.Truncate(marker+prefix+display(label)+status, max(1, width), "…")
		if m.marked(n) {
			line = dimStyle.Render(line)
		} else if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) inspector() string {
	n := m.selected()
	if n.role == "" {
		return "Select a role, instance, or key."
	}
	status := "unchanged"
	if m.blocked(n) {
		status = "deleted with parent (mutations disabled)"
	} else if m.deleted[n] {
		status = "staged deletion (d to undo)"
	} else if value, ok := m.edits[n]; ok {
		original, exists := m.baseline(n)
		if !exists {
			status = "new fact"
		} else if original != value {
			status = "edited"
		}
	}
	text := fmt.Sprintf("Role:     %s\nInstance: %s\nKey:      %s\nStatus:   %s", display(n.role), display(n.instance), display(n.key), status)
	if n.key != "" {
		return text + "\n\nValue\n" + display(m.factValue(n))
	}
	if n.instance != "" {
		return text + "\n\nSelect a key to inspect its value.\nAdd a fact with a."
	}
	for _, r := range m.catalog.Roles {
		if r.Name == n.role {
			text += fmt.Sprintf("\n\n%d existing instances", len(r.Instances))
			break
		}
	}
	return text + "\nExpand to browse instances."
}

func (m *Model) reviewText() string {
	changes := m.changes()
	if len(changes) == 0 {
		return "No effective changes."
	}
	var b strings.Builder
	for _, c := range changes {
		path := display(c.Role)
		if c.Instance != "" {
			path += " / " + display(c.Instance)
		}
		if c.Key != "" {
			path += " / " + display(c.Key)
		}
		switch c.Kind {
		case facts.DeleteRole:
			fmt.Fprintf(&b, "Delete role %s\n", path)
			for _, r := range m.catalog.Roles {
				if r.Name != c.Role {
					continue
				}
				instances := slices.Clone(r.Instances)
				slices.SortFunc(instances, func(a, b facts.Instance) int { return strings.Compare(a.Name, b.Name) })
				for _, i := range instances {
					fmt.Fprintf(&b, "  Delete instance %s\n", display(i.Name))
				}
			}
		case facts.DeleteInstance:
			fmt.Fprintf(&b, "Delete instance %s\n", path)
		case facts.DeleteFact:
			fmt.Fprintf(&b, "Delete fact %s\n", path)
		case facts.SetFact:
			old, exists := m.baseline(node{role: c.Role, instance: c.Instance, key: c.Key})
			action := "Add"
			if exists {
				action = "Edit"
			}
			fmt.Fprintf(&b, "%s %s\n", action, path)
			if exists {
				fmt.Fprintf(&b, "  Before: %s\n", display(old))
			}
			fmt.Fprintf(&b, "  After:  %s\n", display(c.Value))
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func driftText(drift *facts.Drift) string {
	var b strings.Builder
	b.WriteString("Disk changed since the catalog was loaded.\nReload keeps the lock and cancels this attempted change.\nRetry the change manually after reloading.\n")
	for _, side := range []struct {
		label string
		role  facts.Role
	}{{"Before", drift.Before}, {"On disk", drift.After}} {
		fmt.Fprintf(&b, "\n%s: %s\n", side.label, display(side.role.Name))
		if len(side.role.Instances) == 0 {
			b.WriteString("  (no instances; role may have been removed)\n")
		}
		for _, i := range side.role.Instances {
			fmt.Fprintf(&b, "  %s\n", display(i.Name))
			for _, f := range i.Facts {
				fmt.Fprintf(&b, "    %s = %s\n", display(f.Key), display(f.Value))
			}
		}
	}
	return b.String()
}

func window(content string, width, height, offset int) string {
	lines := strings.Split(ansi.Hardwrap(content, max(1, width), false), "\n")
	offset = max(0, min(offset, max(0, len(lines)-height)))
	return strings.Join(lines[offset:min(len(lines), offset+max(1, height))], "\n")
}

func (m *Model) View() tea.View {
	var names []string
	for name := range m.locks {
		names = append(names, display(name))
	}
	slices.Sort(names)
	locks := fmt.Sprintf("Locks: %d", len(names))
	if len(names) > 0 {
		locks += " (" + strings.Join(names, ", ") + ")"
	}
	header := titleStyle.Render("Saltbox facts") + fmt.Sprintf("  •  %d pending", len(m.changes()))
	footer := "↑/↓ navigate  ←/→ expand  / search  s review  q quit"
	content := ""
	bodyHeight := max(1, m.height-6)
	switch m.mode {
	case reviewing, applying:
		header = titleStyle.Render("Review Changes")
		if m.mode == applying {
			header += " — applying…"
		}
		content = window(m.outcome+m.reviewText(), m.width, bodyHeight, m.scroll)
		options := []string{"Apply", "Discard", "Return"}
		if m.exitAfter {
			options = []string{"Apply-and-exit", "Discard-and-exit", "Return"}
		}
		for i := range options {
			options[i] = "[ " + options[i] + " ]"
			if i == m.reviewCursor {
				options[i] = selectedStyle.Render(options[i])
			}
		}
		footer = strings.Join(options, "  ") + "\na Apply  d Discard  r Return  ←/→ choose  ↑/↓ scroll"
	case driftReview, reloading:
		header = titleStyle.Render("Role changed on disk")
		if m.drift != nil {
			content = window(driftText(m.drift), m.width, bodyHeight, m.scroll)
		}
		footer = "r Reload and retry manually  Esc Cancel  ↑/↓ scroll"
		if m.mode == reloading {
			footer = "Updating role lock…"
		}
	case waiting:
		content = fmt.Sprintf("Waiting for %s.ini.lock…\nLock acquisition times out after 30 seconds.\n\nEsc cancels the attempted change.", display(m.target.role))
		footer = "Esc Cancel"
	case adding, editing:
		header = titleStyle.Render("Edit value")
		content = fmt.Sprintf("Role:     %s\nInstance: %s\n", display(m.target.role), display(m.target.instance))
		if m.mode == adding {
			header = titleStyle.Render("Add fact")
			content += "\n" + m.key.View()
		} else {
			content += "Key:      " + display(m.target.key)
		}
		content += "\n\nValue:\n" + m.value.View()
		footer = "Enter Stage  Alt+Enter Newline  Tab Change field  Esc Cancel"
		if m.escapedValue {
			footer = "Escaped value: \\t tab, \\n newline, \\\\ backslash\nEnter Stage  Esc Cancel"
		}
	default:
		if m.width >= 80 {
			treeWidth := max(24, m.width/3)
			tree := lipgloss.NewStyle().Width(treeWidth).Render(m.treeView(treeWidth, bodyHeight))
			content = lipgloss.JoinHorizontal(lipgloss.Top, tree, " │ ", window(m.inspector(), m.width-treeWidth-3, bodyHeight, m.scroll))
		} else {
			treeHeight := max(1, bodyHeight/2)
			content = m.treeView(m.width, treeHeight) + "\n" + window(m.inspector(), m.width, max(1, bodyHeight-treeHeight-1), m.scroll)
		}
		n := m.selected()
		if !m.blocked(n) && n.role != "" {
			footer += "\n"
			if n.instance != "" && !m.marked(n) {
				footer += "a add fact  "
			}
			if n.key != "" && !m.marked(n) {
				footer += "e edit value  "
			}
			footer += "d toggle deletion  x/c expand/collapse all  [/ ] inspect"
		}
		if m.mode == searching {
			header = m.search.View()
			footer = "Type to filter  Enter Browse matches  Esc Clear"
		} else if m.filter != "" {
			header += "  / " + display(m.filter)
		}
	}
	message := m.notice
	if m.err != nil {
		message = "Error: " + display(m.err.Error())
	}
	// Keep status and controls anchored to the terminal bottom as the tree grows.
	content = lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight).Render(content)
	text := header + "\n" + content + "\n" + ansi.Truncate(locks, m.width, "…") + "\n" + ansi.Truncate(message, m.width, "…") + "\n" + footer
	v := tea.NewView(window(text, m.width, m.height, 0))
	v.AltScreen = true
	return v
}
