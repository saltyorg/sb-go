package factui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/saltyorg/sb-go/facts"
)

type lockedMsg struct {
	drift *facts.Drift
	err   error
}
type refreshedMsg struct {
	catalog  facts.Catalog
	err      error
	released bool
}
type appliedMsg struct {
	result  facts.ApplyResult
	catalog facts.Catalog
	err     error
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.ctx.Err() != nil {
		if m.cancelLock != nil {
			m.cancelLock()
		}
		return m, tea.Quit
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(1, msg.Width)
		m.height = max(1, msg.Height)
		m.search.SetWidth(max(1, m.width-6))
		m.key.SetWidth(max(1, m.width-12))
		m.value.SetWidth(max(1, m.width-12))
		m.value.SetHeight(max(1, m.height-15))
	case lockedMsg:
		if m.cancelLock != nil {
			m.cancelLock()
			m.cancelLock = nil
		}
		if m.lockCanceled {
			m.mode = browsing
			if msg.err == nil {
				m.mode = reloading
				return m, m.releaseCommand()
			}
			if m.reviewAfterCancel {
				m.reviewAfterCancel = false
				return m, m.openReview(true)
			}
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err
			m.mode = browsing
			return m, nil
		}
		m.locks[m.target.role] = true
		if msg.drift != nil {
			m.drift = msg.drift
			m.mode = driftReview
			m.scroll = 0
			return m, nil
		}
		return m, m.mutate()
	case refreshedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.mode = driftReview
			return m, nil
		}
		if msg.released {
			delete(m.locks, m.target.role)
		} else {
			m.catalog = msg.catalog
			m.notice = "Reloaded role; retry your change manually."
		}
		m.drift = nil
		m.mode = browsing
		m.selectTarget(m.target)
		if m.reviewAfterCancel {
			m.reviewAfterCancel = false
			return m, m.openReview(true)
		}
	case appliedMsg:
		m.outcome = ""
		if len(msg.result.Applied) > 0 {
			m.outcome += "Applied: " + display(strings.Join(msg.result.Applied, ", ")) + "\n"
		}
		if msg.result.Failed != nil {
			m.outcome += "Failed: " + display(msg.result.Failed.Role) + "\n"
		}
		if len(msg.result.Unattempted) > 0 {
			m.outcome += "Unattempted: " + display(strings.Join(msg.result.Unattempted, ", ")) + "\n"
		}
		for _, role := range msg.result.Applied {
			for n := range m.edits {
				if n.role == role {
					delete(m.edits, n)
				}
			}
			for n := range m.deleted {
				if n.role == role {
					delete(m.deleted, n)
				}
			}
		}
		m.catalog = msg.catalog
		m.err = msg.err
		if msg.err == nil && msg.result.Failed != nil {
			m.err = fmt.Errorf("apply %s: %w", msg.result.Failed.Role, msg.result.Failed.Err)
		}
		if m.err == nil && len(m.changes()) != 0 {
			m.err = errors.New("some roles were not applied; remaining changes are still pending")
		}
		if m.err != nil {
			m.mode = reviewing
			m.scroll = 0
			return m, nil
		}
		m.notice = "Changes applied."
		m.mode = browsing
		if m.exitAfter {
			return m, tea.Quit
		}
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	default:
		var cmd tea.Cmd
		switch m.mode {
		case searching:
			m.search, cmd = m.search.Update(msg)
			m.filter = m.search.Value()
			m.cursor = 0
		case adding:
			if !m.valueFocus {
				m.key, cmd = m.key.Update(msg)
			} else {
				m.value, cmd = m.value.Update(msg)
			}
		case editing:
			m.value, cmd = m.value.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if m.mode == waiting {
		if key == "esc" || key == "ctrl+c" || key == "q" {
			m.lockCanceled = true
			m.reviewAfterCancel = key != "esc"
			m.notice = "Cancelling lock wait…"
			m.cancelLock()
		}
		return nil
	}
	if m.mode == applying || m.mode == reloading {
		return nil
	}
	if key == "ctrl+c" {
		return m.openReview(true)
	}
	switch m.mode {
	case driftReview:
		switch key {
		case "r", "enter":
			m.mode = reloading
			role := m.target.role
			return func() tea.Msg {
				err := m.session.ReloadRole(role)
				return refreshedMsg{catalog: m.session.Catalog(), err: err}
			}
		case "q":
			return m.openReview(true)
		case "esc", "c":
			m.mode = reloading
			return m.releaseCommand()
		case "down":
			m.scroll++
		case "up":
			m.scroll = max(0, m.scroll-1)
		}
		return nil
	case reviewing:
		return m.reviewKey(key)
	case searching:
		switch key {
		case "enter":
			m.filter = m.search.Value()
			m.mode = browsing
			m.search.Blur()
			return nil
		case "esc":
			m.filter = ""
			m.search.SetValue("")
			m.mode = browsing
			return nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.filter = m.search.Value()
		m.cursor = 0
		return cmd
	case adding, editing:
		switch key {
		case "esc":
			m.mode = browsing
			return nil
		case "tab", "shift+tab":
			if m.mode == adding {
				m.valueFocus = !m.valueFocus
				if m.valueFocus {
					m.key.Blur()
					return m.value.Focus()
				}
				m.value.Blur()
				return m.key.Focus()
			}
			return nil
		case "enter":
			m.stageValue()
			return nil
		}
		var cmd tea.Cmd
		if m.mode == adding && !m.valueFocus {
			m.key, cmd = m.key.Update(msg)
		} else {
			m.value, cmd = m.value.Update(msg)
		}
		return cmd
	}
	m.err = nil
	m.notice = ""
	n := m.selected()
	switch key {
	case "q":
		return m.openReview(true)
	case "s", "ctrl+s":
		return m.openReview(false)
	case "/":
		m.mode = searching
		m.search.SetValue(m.filter)
		return m.search.Focus()
	case "esc":
		m.filter = ""
		m.search.SetValue("")
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(max(0, len(m.rows())-1), m.cursor+1)
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = max(0, len(m.rows())-1)
	case "pgdown":
		m.cursor = min(max(0, len(m.rows())-1), m.cursor+max(1, m.height-10))
	case "pgup":
		m.cursor = max(0, m.cursor-max(1, m.height-10))
	case "right":
		if n.key == "" {
			m.expanded[n] = true
		}
	case "left":
		if m.expanded[n] {
			delete(m.expanded, n)
		} else {
			m.selectTarget(n.parent())
		}
	case "x":
		for _, r := range m.catalog.Roles {
			m.expanded[node{role: r.Name}] = true
			for _, i := range r.Instances {
				m.expanded[node{role: r.Name, instance: i.Name}] = true
			}
		}
	case "c":
		clear(m.expanded)
		m.selectTarget(node{role: n.role})
	case "]":
		m.scroll++
	case "[":
		m.scroll = max(0, m.scroll-1)
	case "enter":
		if n.key == "" {
			m.expanded[n] = !m.expanded[n]
		} else {
			return m.requestMutation("e", n)
		}
	case "a", "e", "d":
		return m.requestMutation(key, n)
	}
	return nil
}

func (m *Model) requestMutation(action string, n node) tea.Cmd {
	if n.role == "" || m.blocked(n) {
		return nil
	}
	if action == "e" && (n.key == "" || m.deleted[n]) {
		return nil
	}
	if action == "a" && (n.instance == "" || m.marked(n)) {
		return nil
	}
	m.target = n
	m.action = action
	m.err = nil
	if m.locks[n.role] {
		return m.mutate()
	}
	m.mode = waiting
	m.lockCanceled = false
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelLock = cancel
	return func() tea.Msg {
		drift, err := m.session.LockRole(ctx, n.role)
		return lockedMsg{drift: drift, err: err}
	}
}

func (m *Model) mutate() tea.Cmd {
	m.mode = browsing
	switch m.action {
	case "d":
		if m.deleted[m.target] {
			delete(m.deleted, m.target)
		} else {
			m.deleted[m.target] = true
		}
	case "a":
		m.mode = adding
		m.escapedValue = false
		m.key.SetValue("")
		m.value.SetValue("")
		m.valueFocus = false
		m.value.Blur()
		return m.key.Focus()
	case "e":
		m.mode = editing
		m.key.SetValue(m.target.key)
		value := m.factValue(m.target)
		// Bubbles expands tabs and removes other control bytes. Use explicit
		// escapes only for those values so opening an editor preserves bytes.
		m.escapedValue = strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' })
		if m.escapedValue {
			quoted := strconv.Quote(value)
			value = quoted[1 : len(quoted)-1]
		}
		m.value.SetValue(value)
		m.valueFocus = true
		m.key.Blur()
		return m.value.Focus()
	}
	return nil
}

func (m *Model) stageValue() {
	n := m.target
	if m.mode == adding {
		n.key = m.key.Value()
		if n.key == "" || strings.TrimSpace(n.key) != n.key || strings.Contains(n.key, "=") || strings.HasPrefix(n.key, "[") || strings.HasPrefix(n.key, "#") || strings.HasPrefix(n.key, ";") || strings.ContainsFunc(n.key, unicode.IsControl) {
			m.err = errors.New("enter a non-empty INI key without delimiters or control characters")
			return
		}
		_, original := m.baseline(n)
		_, staged := m.edits[n]
		if original || staged {
			m.err = errors.New("that key already exists; edit its value instead")
			return
		}
	}
	value := m.value.Value()
	if m.escapedValue {
		decoded, err := strconv.Unquote("\"" + value + "\"")
		if err != nil {
			m.err = fmt.Errorf("invalid escaped value: %w", err)
			return
		}
		value = decoded
	}
	if strings.ContainsAny(value, "\r\x00") {
		m.err = errors.New("fact values must not contain carriage returns or NUL")
		return
	}
	m.edits[n] = value
	m.mode = browsing
	m.err = nil
	m.expanded[n.parent()] = true
	m.selectTarget(n)
}

func (m *Model) releaseCommand() tea.Cmd {
	role := m.target.role
	return func() tea.Msg { return refreshedMsg{err: m.session.ReleaseRole(role), released: true} }
}

func (m *Model) openReview(exit bool) tea.Cmd {
	// Drift owns a newly acquired lock until reload or cancellation. Resolve it
	// before changing screens so it cannot silently become a usable baseline.
	if m.mode == driftReview {
		m.reviewAfterCancel = exit
		m.mode = reloading
		return m.releaseCommand()
	}
	if len(m.changes()) == 0 && exit {
		return tea.Quit
	}
	m.mode = reviewing
	m.outcome = ""
	m.exitAfter = exit
	m.reviewCursor = 0
	m.scroll = 0
	return nil
}

func (m *Model) reviewKey(key string) tea.Cmd {
	switch key {
	case "left", "shift+tab":
		m.reviewCursor = (m.reviewCursor + 2) % 3
		return nil
	case "right", "tab":
		m.reviewCursor = (m.reviewCursor + 1) % 3
		return nil
	case "down":
		m.scroll++
		return nil
	case "up":
		m.scroll = max(0, m.scroll-1)
		return nil
	case "enter":
		key = []string{"a", "d", "r"}[m.reviewCursor]
	}
	switch key {
	case "esc", "r", "q":
		m.mode = browsing
		m.exitAfter = false
	case "d":
		clear(m.edits)
		clear(m.deleted)
		m.err = nil
		m.mode = browsing
		m.notice = "Pending changes discarded."
		if m.exitAfter {
			return tea.Quit
		}
	case "a":
		changes := m.changes()
		if len(changes) == 0 {
			m.mode = browsing
			if m.exitAfter {
				return tea.Quit
			}
			return nil
		}
		m.mode = applying
		m.err = nil
		return func() tea.Msg {
			result, err := m.session.Apply(m.ctx, changes)
			return appliedMsg{result: result, catalog: m.session.Catalog(), err: err}
		}
	}
	return nil
}
