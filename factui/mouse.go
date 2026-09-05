package factui

import tea "charm.land/bubbletea/v2"

type mouseActionKind uint8

const (
	mouseSelect mouseActionKind = iota + 1
	mouseToggle
	mouseOpenContext
	mouseMoveTree
	mouseScrollContent
	mouseReviewChoice
	mouseRunContextChoice
	mouseDismissContext
	mouseMutation
)

type mouseAction struct {
	kind     mouseActionKind
	node     node
	delta    int
	choice   int
	x, y     int
	mutation string
}

type mouseActionMsg struct{ action mouseAction }

type contextMenuState struct {
	node   node
	x, y   int
	cursor int
}

type contextItem struct {
	label     string
	action    mouseAction
	dangerous bool
}

func (m *Model) contextItems(n node) []contextItem {
	if n.role == "" || m.blocked(n) {
		return nil
	}
	var items []contextItem
	if n.key == "" {
		label := "Expand"
		if m.expanded[n] {
			label = "Collapse"
		}
		items = append(items, contextItem{label: label, action: mouseAction{kind: mouseToggle, node: n}})
	}
	if m.deleted[n] {
		return append(items, contextItem{
			label:  "Undo staged deletion",
			action: mouseAction{kind: mouseMutation, node: n, mutation: "d"},
		})
	}
	switch {
	case n.key != "":
		items = append(items,
			contextItem{label: "Edit value", action: mouseAction{kind: mouseMutation, node: n, mutation: "e"}},
			contextItem{label: "Add sibling fact", action: mouseAction{kind: mouseMutation, node: n, mutation: "a"}},
			contextItem{label: "Stage fact deletion", action: mouseAction{kind: mouseMutation, node: n, mutation: "d"}, dangerous: true},
		)
	case n.instance != "":
		items = append(items,
			contextItem{label: "Add fact", action: mouseAction{kind: mouseMutation, node: n, mutation: "a"}},
			contextItem{label: "Stage instance deletion", action: mouseAction{kind: mouseMutation, node: n, mutation: "d"}, dangerous: true},
		)
	default:
		items = append(items, contextItem{label: "Stage role deletion", action: mouseAction{kind: mouseMutation, node: n, mutation: "d"}, dangerous: true})
	}
	return items
}

func (m *Model) handleMouseAction(action mouseAction) tea.Cmd {
	switch action.kind {
	case mouseSelect:
		m.contextMenu = nil
		m.selectTarget(action.node)
	case mouseToggle:
		m.contextMenu = nil
		m.selectTarget(action.node)
		if m.mode == browsing && action.node.key == "" {
			m.expanded[action.node] = !m.expanded[action.node]
		}
	case mouseOpenContext:
		m.selectTarget(action.node)
		if m.blocked(action.node) {
			m.contextMenu = nil
			m.notice = "This item is deleted with its parent; undo the parent deletion first."
			return nil
		}
		if m.mode == searching {
			m.mode = browsing
			m.search.Blur()
		}
		if len(m.contextItems(action.node)) == 0 {
			m.contextMenu = nil
			return nil
		}
		m.err = nil
		m.notice = ""
		m.contextMenu = &contextMenuState{node: action.node, x: action.x, y: action.y}
	case mouseMoveTree:
		m.contextMenu = nil
		m.cursor = min(max(0, len(m.rows())-1), max(0, m.cursor+action.delta))
	case mouseScrollContent:
		m.contextMenu = nil
		m.scroll = max(0, m.scroll+action.delta)
	case mouseReviewChoice:
		if m.mode != reviewing || action.choice < 0 || action.choice > 2 {
			return nil
		}
		m.reviewCursor = action.choice
		return m.reviewKey([]string{"a", "d", "r"}[action.choice])
	case mouseRunContextChoice:
		return m.runContextItem(action.choice)
	case mouseDismissContext:
		m.contextMenu = nil
	case mouseMutation:
		m.contextMenu = nil
		m.selectTarget(action.node)
		return m.requestMutation(action.mutation, action.node)
	}
	return nil
}

func (m *Model) contextMenuKey(key string) tea.Cmd {
	if m.contextMenu == nil {
		return nil
	}
	items := m.contextItems(m.contextMenu.node)
	if len(items) == 0 {
		m.contextMenu = nil
		return nil
	}
	switch key {
	case "down", "j", "tab":
		m.contextMenu.cursor = (m.contextMenu.cursor + 1) % len(items)
	case "up", "k", "shift+tab":
		m.contextMenu.cursor = (m.contextMenu.cursor + len(items) - 1) % len(items)
	case "enter":
		return m.runContextItem(m.contextMenu.cursor)
	case "esc", "q":
		m.contextMenu = nil
	}
	return nil
}

func (m *Model) runContextItem(index int) tea.Cmd {
	if m.contextMenu == nil {
		return nil
	}
	items := m.contextItems(m.contextMenu.node)
	if index < 0 || index >= len(items) {
		return nil
	}
	action := items[index].action
	m.contextMenu = nil
	return m.handleMouseAction(action)
}
