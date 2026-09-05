package factui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/saltyorg/sb-go/facts"
)

var (
	ink     = lipgloss.Color("#E6EDF3")
	muted   = lipgloss.Color("#7D8590")
	panel   = lipgloss.Color("#283442")
	accent  = lipgloss.Color("#7DD3FC")
	accent2 = lipgloss.Color("#A78BFA")
	warning = lipgloss.Color("#F8C76A")
	danger  = lipgloss.Color("#FF7B72")
	dark    = lipgloss.Color("#081018")

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(ink)
	mutedStyle    = lipgloss.NewStyle().Foreground(muted)
	accentStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	secondary     = lipgloss.NewStyle().Foreground(accent2)
	dirtyStyle    = lipgloss.NewStyle().Bold(true).Foreground(warning)
	dangerStyle   = lipgloss.NewStyle().Bold(true).Foreground(danger)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(dark).Background(accent)
	border        = lipgloss.RoundedBorder()
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
		return mutedStyle.Render("No facts match this search.")
	}
	m.selected()
	reserved := 0
	var prefix string
	if m.mode == searching || m.filter != "" {
		search := m.search.View()
		if m.mode != searching {
			search = mutedStyle.Render("Search  " + display(m.filter))
		}
		prefix = search + "\n" + mutedStyle.Render(fmt.Sprintf("%d visible nodes", len(rows))) + "\n\n"
		reserved = 3
	}
	start, end := visibleWindow(m.cursor, len(rows), height-reserved)
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		n := rows[index]
		line := ansi.Truncate(m.treeEntryLabel(n), max(1, width), "…")
		if index == m.cursor {
			line = selectedStyle.Width(max(1, width)).Render(line)
		} else if m.marked(n) {
			line = mutedStyle.Faint(true).Render(line)
		}
		lines = append(lines, line)
	}
	return prefix + strings.Join(lines, "\n")
}

func (m *Model) treeEntryLabel(n node) string {
	if n.instance == "" {
		arrow := "▸"
		if m.expanded[n] || m.filter != "" {
			arrow = "▾"
		}
		return fmt.Sprintf("%s %s  %s%s", arrow, display(n.role), mutedStyle.Render(fmt.Sprintf("(%d instances)", m.instanceCount(n.role))), m.deleteLabel(n))
	}
	if n.key == "" {
		arrow := "▸"
		if m.expanded[n] || m.filter != "" {
			arrow = "▾"
		}
		return fmt.Sprintf("  %s %s  %s%s", arrow, display(n.instance), mutedStyle.Render(fmt.Sprintf("(%d facts)", m.factCount(n))), m.deleteLabel(n))
	}
	marker := "•"
	status := ""
	if m.marked(n) {
		marker = "●"
		status = "  " + dangerStyle.Render("delete")
	} else if value, ok := m.edits[n]; ok {
		original, exists := m.baseline(n)
		if !exists || original != value {
			marker = "●"
		}
	}
	return fmt.Sprintf("      %s %s%s", marker, display(n.key), status)
}

func (m *Model) deleteLabel(n node) string {
	if !m.marked(n) {
		return ""
	}
	return "  " + dangerStyle.Render("delete")
}

func (m *Model) inspector() string {
	return m.inspectorView(70, 24, false)
}

func (m *Model) inspectorView(width, height int, compact bool) string {
	n := m.selected()
	if n.role == "" {
		return mutedStyle.Render("Nothing selected")
	}
	if compact {
		details := m.compactInspector(n, width)
		separator := mutedStyle.Render(strings.Repeat("─", max(8, width)))
		remaining := max(1, height-lipgloss.Height(details)-1)
		return details + "\n" + separator + "\n" + m.changeSummary(remaining, width)
	}

	details := m.entryDetails(n, width)
	remaining := max(1, height-lipgloss.Height(details)-3)
	separator := mutedStyle.Render(strings.Repeat("─", max(8, width)))
	return details + "\n\n" + separator + "\n" + m.changeSummary(remaining, width)
}

func (m *Model) compactInspector(n node, width int) string {
	status := m.nodeStatus(n)
	lines := []string{metadataField("Role", n.role), metadataField("Status", status)}
	if n.instance != "" {
		lines = []string{metadataField("Role", n.role), metadataField("Instance", n.instance), metadataField("Status", status)}
	}
	if n.key != "" {
		lines = []string{
			metadataField("Role", n.role),
			metadataField("Instance", n.instance),
			metadataField("Key", n.key),
			metadataField("Status", status),
			metadataField("Value", ansi.Truncate(display(m.factValue(n)), max(1, width-11), "…")),
		}
	} else if n.instance != "" {
		lines = append(lines, metadataField("Facts", fmt.Sprintf("%d", m.factCount(n))))
	} else {
		lines = append(lines, metadataField("Instances", fmt.Sprintf("%d", m.instanceCount(n.role))))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) entryDetails(n node, width int) string {
	if n.instance == "" {
		lines := []string{
			metadataField("Role", n.role),
			metadataField("Status", m.nodeStatus(n)),
			metadataField("Instances", fmt.Sprintf("%d", m.instanceCount(n.role))),
			"",
		}
		for _, r := range m.catalog.Roles {
			if r.Name != n.role {
				continue
			}
			instances := slices.Clone(r.Instances)
			slices.SortFunc(instances, func(a, b facts.Instance) int { return strings.Compare(a.Name, b.Name) })
			for _, instance := range instances {
				lines = append(lines, fmt.Sprintf("  %-28s %d facts", display(instance.Name), len(instance.Facts)))
			}
			break
		}
		if len(lines) == 4 {
			lines = append(lines, mutedStyle.Render("  No instances found."))
		}
		return strings.Join(lines, "\n")
	}
	if n.key == "" {
		lines := []string{
			metadataField("Role", n.role),
			metadataField("Instance", n.instance),
			metadataField("Status", m.nodeStatus(n)),
			metadataField("Facts", fmt.Sprintf("%d", m.factCount(n))),
			"",
		}
		for _, fact := range m.instanceFacts(n) {
			factNode := node{role: n.role, instance: n.instance, key: fact.key}
			marker := " "
			if m.marked(factNode) || fact.staged {
				marker = "●"
			}
			valueWidth := max(8, width-34)
			lines = append(lines, fmt.Sprintf("%s %-26s %s", marker, display(fact.key), ansi.Truncate(display(fact.value), valueWidth, "…")))
		}
		if len(lines) == 5 {
			lines = append(lines, mutedStyle.Render("No facts found."))
		}
		return strings.Join(lines, "\n")
	}

	valueWidth := max(1, width)
	value := lipgloss.NewStyle().Width(valueWidth).Foreground(ink).Render(display(m.factValue(n)))
	return metadataField("Role", n.role) + "\n" +
		metadataField("Instance", n.instance) + "\n" +
		metadataField("Key", n.key) + "\n" +
		metadataField("Status", m.nodeStatus(n)) + "\n\n" +
		mutedStyle.Render("PLAINTEXT VALUE") + "\n" + value + "\n\n" + secondary.Render("Enter or e to edit")
}

type inspectorFact struct {
	key, value string
	staged     bool
}

func (m *Model) instanceFacts(n node) []inspectorFact {
	values := make(map[string]inspectorFact)
	for _, r := range m.catalog.Roles {
		if r.Name != n.role {
			continue
		}
		for _, instance := range r.Instances {
			if instance.Name != n.instance {
				continue
			}
			for _, fact := range instance.Facts {
				values[fact.Key] = inspectorFact{key: fact.Key, value: fact.Value}
			}
		}
	}
	for edited, value := range m.edits {
		if edited.role == n.role && edited.instance == n.instance {
			fact := values[edited.key]
			fact.key, fact.value, fact.staged = edited.key, value, true
			values[edited.key] = fact
		}
	}
	facts := make([]inspectorFact, 0, len(values))
	for _, fact := range values {
		facts = append(facts, fact)
	}
	slices.SortFunc(facts, func(a, b inspectorFact) int { return strings.Compare(a.key, b.key) })
	return facts
}

func (m *Model) nodeStatus(n node) string {
	if m.blocked(n) {
		return "deleted with parent"
	}
	if m.deleted[n] {
		return "staged deletion"
	}
	if value, ok := m.edits[n]; ok {
		original, exists := m.baseline(n)
		if !exists {
			return "new fact"
		}
		if original != value {
			return "staged edit"
		}
	}
	return "unchanged"
}

func metadataField(label, value string) string {
	return mutedStyle.Width(11).Render(label+":") + titleStyle.Render(display(value))
}

func (m *Model) changeSummary(capacity, width int) string {
	changes := m.changes()
	if len(changes) == 0 {
		return mutedStyle.Render("PENDING CHANGES") + "\n" + mutedStyle.Render("No staged changes")
	}
	lines := []string{dirtyStyle.Render("PENDING CHANGES  ·  PRESS S TO REVIEW")}
	start := max(0, len(changes)-max(1, capacity-1))
	for _, change := range changes[start:] {
		lines = append(lines, dirtyStyle.Render("●")+" "+ansi.Truncate(changeLabel(change), max(1, width-2), "…"))
	}
	if start > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("…and %d earlier", start)))
	}
	return strings.Join(lines, "\n")
}

func changeLabel(change facts.Change) string {
	path := display(change.Role)
	if change.Instance != "" {
		path += " / " + display(change.Instance)
	}
	if change.Key != "" {
		path += " / " + display(change.Key)
	}
	switch change.Kind {
	case facts.DeleteRole:
		return "Delete role " + path
	case facts.DeleteInstance:
		return "Delete instance " + path
	case facts.DeleteFact:
		return "Delete fact " + path
	case facts.SetFact:
		return "Set " + path
	default:
		return path
	}
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
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(ansi.Hardwrap(content, width, false), "\n")
	offset = max(0, min(offset, max(0, len(lines)-height)))
	return strings.Join(lines[offset:min(len(lines), offset+height)], "\n")
}

func (m *Model) View() tea.View {
	width, height := max(1, m.width), max(1, m.height)
	body := m.mainView(width, height)
	switch m.mode {
	case adding, editing:
		body = m.formModal(width, height)
	case waiting:
		body = m.messageModal(width, height, "WAITING FOR ROLE LOCK", "", fmt.Sprintf("Waiting for %s.ini.lock…\nLock acquisition times out after 30 seconds.", display(m.target.role)), "Esc cancels the attempted change", accent)
	case driftReview:
		content := ""
		if m.drift != nil {
			content = driftText(m.drift)
		}
		body = m.messageModal(width, height, "ROLE CHANGED ON DISK", m.locksText(), content, "r reload and retry manually   Esc cancel   ↑/↓ scroll", warning)
	case reviewing, applying:
		body = m.reviewModal(width, height)
	case reloading:
		body = m.messageModal(width, height, "UPDATING ROLE LOCK", "", "Refreshing the protected role state…", "Please wait", accent)
	}
	view := tea.NewView(window(body, width, height, 0))
	view.AltScreen = true
	return view
}

func (m *Model) mainView(width, height int) string {
	header := m.header(width)
	footer := m.footer(width)
	bodyHeight := max(1, height-lipgloss.Height(header)-lipgloss.Height(footer))

	var body string
	if width >= 96 {
		treeWidth := min(56, max(40, width/2-5))
		inspectorWidth := width - treeWidth
		tree := pane("FACT TREE", m.treeView(max(1, treeWidth-4), max(1, bodyHeight-4)), treeWidth, bodyHeight, accent, 0)
		inspector := pane("INSPECTOR", m.inspectorView(max(1, inspectorWidth-4), max(1, bodyHeight-4), false), inspectorWidth, bodyHeight, accent2, m.scroll)
		body = lipgloss.JoinHorizontal(lipgloss.Top, tree, inspector)
	} else {
		treeHeight := max(8, (bodyHeight+1)/2)
		treeHeight = min(treeHeight, bodyHeight)
		inspectorHeight := max(0, bodyHeight-treeHeight)
		tree := pane("FACT TREE", m.treeView(max(1, width-4), max(1, treeHeight-4)), width, treeHeight, accent, 0)
		if inspectorHeight >= 4 {
			inspector := pane("INSPECTOR", m.inspectorView(max(1, width-4), max(1, inspectorHeight-4), true), width, inspectorHeight, accent2, m.scroll)
			body = lipgloss.JoinVertical(lipgloss.Left, tree, inspector)
		} else {
			body = tree
		}
	}
	return header + "\n" + body + "\n" + footer
}

func (m *Model) header(width int) string {
	roles, instances, factCount := m.counts()
	left := titleStyle.Render("◆ Saltbox Facts") + "  " + mutedStyle.Render("TREE EDITOR")
	right := mutedStyle.Render(fmt.Sprintf("%d roles  ·  %d instances  ·  %d facts", roles, instances, factCount))
	innerWidth := max(1, width-2)
	if ansi.StringWidth(left)+ansi.StringWidth(right)+1 > innerWidth {
		left = titleStyle.Render("◆ Saltbox Facts")
	}
	space := strings.Repeat(" ", max(1, innerWidth-ansi.StringWidth(left)-ansi.StringWidth(right)))
	line := ansi.Truncate(left+space+right, innerWidth, "")
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Render(line)
}

func (m *Model) footer(width int) string {
	n := m.selected()
	context := "a add"
	if n.role != "" {
		switch {
		case n.key != "":
			context = "Enter/e edit   a add sibling fact   d toggle deletion"
		case n.instance != "":
			context = "a add fact   d toggle deletion"
		default:
			context = "d toggle deletion"
		}
	}
	if m.mode == searching {
		context = "Type to search   ↑/↓ result   Enter browse   Esc clear"
	} else if m.filter != "" {
		context += "   Esc clear filter"
	}
	state := mutedStyle.Render("clean")
	if pending := len(m.changes()); pending > 0 {
		state = dirtyStyle.Render(fmt.Sprintf("● %d staged", pending))
	}
	message := m.notice
	if m.err != nil {
		message = dangerStyle.Render("Error: " + display(m.err.Error()))
	}
	status := state + "  " + mutedStyle.Render(m.locksText())
	if message != "" {
		status += "  " + message
	}
	innerWidth := max(1, width-2)
	globalControls := "/ search   x expand all   c collapse all   [/] inspect   s review changes   q quit"
	if width < 96 {
		globalControls = "/ search   x expand   c collapse   [/] inspect   s review changes   q quit"
	}
	lines := []string{
		accentStyle.Render("↑/↓ navigate   → expand   ← collapse/parent   " + context),
		status,
		mutedStyle.Render(globalControls),
	}
	for index := range lines {
		lines[index] = ansi.Truncate(lines[index], innerWidth, "…")
	}
	return lipgloss.NewStyle().Width(width).Padding(0, 1).BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(panel).Render(strings.Join(lines, "\n"))
}

func (m *Model) formModal(width, height int) string {
	title := "EDIT FACT VALUE"
	content := metadataField("Role", m.target.role) + "\n" + metadataField("Instance", m.target.instance) + "\n" + metadataField("Key", m.target.key)
	if m.mode == adding {
		title = "ADD FACT"
		content = metadataField("Role", m.target.role) + "\n" + metadataField("Instance", m.target.instance) + "\n\n" + m.key.View()
	}
	content += "\n\n" + mutedStyle.Render("PLAINTEXT VALUE") + "\n" + m.value.View()
	footer := "Enter stage   Alt+Enter newline   Tab change field   Esc cancel"
	if m.escapedValue {
		footer = "Escaped value: \\t tab, \\n newline, \\\\ backslash\nEnter stage   Esc cancel"
	}
	if m.err != nil {
		content += "\n\n" + dangerStyle.Render("Error: "+display(m.err.Error()))
	}
	return m.messageModal(width, height, title, "", content, footer, accent)
}

func (m *Model) reviewModal(width, height int) string {
	title := fmt.Sprintf("REVIEW %d PENDING CHANGE(S)", len(m.changes()))
	if m.exitAfter {
		title += " BEFORE EXITING"
	}
	if m.mode == applying {
		title = "APPLYING CHANGES"
	}
	content := m.outcome + m.reviewText()
	options := []string{"Apply", "Discard", "Return"}
	if m.exitAfter {
		options = []string{"Apply-and-exit", "Discard-and-exit", "Return"}
	}
	for index := range options {
		options[index] = "[ " + options[index] + " ]"
		if index == m.reviewCursor {
			options[index] = selectedStyle.Render(options[index])
		}
	}
	footer := strings.Join(options, "  ") + "\n" + "a Apply   d Discard   r Return   ←/→ choose   ↑/↓ scroll"
	if m.mode == applying {
		footer = "Writing protected role files…"
	}
	if m.err != nil {
		content = dangerStyle.Render("Error: "+display(m.err.Error())) + "\n\n" + content
	}
	return m.messageModal(width, height, title, m.locksText(), content, footer, warning)
}

func (m *Model) messageModal(width, height int, title, detail, content, footer string, borderColor color.Color) string {
	if width < 12 || height < 8 {
		return window(title+"\n\n"+content+"\n\n"+footer, width, height, m.scroll)
	}
	modalWidth := min(82, width-4)
	innerWidth := max(1, modalWidth-6)
	maxInnerHeight := max(1, height-6)
	staticHeight := 1
	if detail != "" {
		staticHeight++
	}
	if footer != "" {
		staticHeight += lipgloss.Height(footer) + 2
	}
	contentHeight := max(1, maxInnerHeight-staticHeight-1)
	parts := []string{accentStyle.Render(title)}
	if detail != "" {
		parts = append(parts, mutedStyle.Render(detail))
	}
	parts = append(parts, "", window(content, innerWidth, contentHeight, m.scroll))
	if footer != "" {
		parts = append(parts, "", mutedStyle.Render(footer))
	}
	modal := lipgloss.NewStyle().Width(modalWidth).Padding(1, 2).Border(border).BorderForeground(borderColor).Render(strings.Join(parts, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func pane(title, content string, width, height int, borderColor color.Color, offset int) string {
	if width < 4 || height < 4 {
		return window(content, width, height, 0)
	}
	innerWidth := max(1, width-4)
	innerHeight := max(1, height-2)
	payloadHeight := max(0, innerHeight-2)
	body := accentStyle.Render(title)
	if payloadHeight > 0 {
		body += "\n\n" + window(content, innerWidth, payloadHeight, offset)
	}
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Border(border).BorderForeground(borderColor).Render(body)
}

func visibleWindow(cursor, length, capacity int) (int, int) {
	capacity = max(1, capacity)
	if length <= capacity {
		return 0, length
	}
	start := max(0, cursor-capacity/2)
	start = min(start, length-capacity)
	return start, start + capacity
}

func (m *Model) counts() (roles, instances, factCount int) {
	roles = len(m.catalog.Roles)
	for _, role := range m.catalog.Roles {
		instances += len(role.Instances)
		for _, instance := range role.Instances {
			factCount += len(instance.Facts)
		}
	}
	for n := range m.edits {
		if _, exists := m.baseline(n); !exists {
			factCount++
		}
	}
	return roles, instances, factCount
}

func (m *Model) instanceCount(role string) int {
	for _, item := range m.catalog.Roles {
		if item.Name == role {
			return len(item.Instances)
		}
	}
	return 0
}

func (m *Model) factCount(n node) int {
	return len(m.instanceFacts(n))
}

func (m *Model) locksText() string {
	names := make([]string, 0, len(m.locks))
	for name := range m.locks {
		names = append(names, display(name))
	}
	slices.Sort(names)
	text := fmt.Sprintf("Locks: %d", len(names))
	if len(names) > 0 {
		text += " (" + strings.Join(names, ", ") + ")"
	}
	return text
}
