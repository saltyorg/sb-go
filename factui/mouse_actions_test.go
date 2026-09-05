package factui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/saltyorg/sb-go/facts"
)

func TestContextItemsMatchNodeAndDeletionState(t *testing.T) {
	m, _ := fixture(t)
	role := node{role: "plex"}
	instance := node{role: "plex", instance: "main"}
	fact := node{role: "plex", instance: "main", key: "token"}

	for _, test := range []struct {
		name string
		node node
		want []string
	}{
		{name: "role", node: role, want: []string{"Collapse", "Stage role deletion"}},
		{name: "instance", node: instance, want: []string{"Expand", "Add fact", "Stage instance deletion"}},
		{name: "fact", node: fact, want: []string{"Edit value", "Add sibling fact", "Stage fact deletion"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := contextLabels(m.contextItems(test.node)); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("context items = %q, want %q", got, test.want)
			}
		})
	}

	m.deleted[fact] = true
	if got := contextLabels(m.contextItems(fact)); !reflect.DeepEqual(got, []string{"Undo staged deletion"}) {
		t.Fatalf("deleted fact context items = %q", got)
	}
	delete(m.deleted, fact)
	m.deleted[instance] = true
	if got := contextLabels(m.contextItems(fact)); len(got) != 0 {
		t.Fatalf("child deleted with parent has actions: %q", got)
	}
}

func TestContextMenuActionsUseExistingMutationFlow(t *testing.T) {
	m, _ := fixture(t)
	fact := node{role: "plex", instance: "main", key: "token"}
	m.mode = searching
	m.filter = "token"
	m.search.SetValue("token")
	_ = m.search.Focus()

	m.handleMouseAction(mouseAction{kind: mouseOpenContext, node: fact, x: 78, y: 20})
	if m.contextMenu == nil || m.contextMenu.node != fact || m.contextMenu.x != 78 || m.contextMenu.y != 20 {
		t.Fatalf("context menu state = %+v", m.contextMenu)
	}
	if m.mode != browsing || m.filter != "token" || m.selected() != fact {
		t.Fatalf("right-click search result did not preserve filter and select row: mode=%v filter=%q selected=%+v", m.mode, m.filter, m.selected())
	}

	cmd := m.contextMenuKey("enter")
	if cmd == nil || m.contextMenu != nil || m.mode != waiting {
		t.Fatalf("Edit value did not enter existing lock flow: mode=%v menu=%+v", m.mode, m.contextMenu)
	}
	complete(m, cmd)
	if m.mode != editing || m.target != fact {
		t.Fatalf("Edit value did not open existing editor: mode=%v target=%+v", m.mode, m.target)
	}
}

func TestContextMenuKeyboardAndResizeDismissal(t *testing.T) {
	m, _ := fixture(t)
	instance := node{role: "plex", instance: "main"}
	m.handleMouseAction(mouseAction{kind: mouseOpenContext, node: instance, x: 10, y: 10})

	m.contextMenuKey("down")
	if m.contextMenu == nil || m.contextMenu.cursor != 1 {
		t.Fatalf("down did not move menu cursor: %+v", m.contextMenu)
	}
	m.contextMenuKey("up")
	if m.contextMenu.cursor != 0 {
		t.Fatalf("up did not wrap menu cursor: %+v", m.contextMenu)
	}
	m.contextMenuKey("q")
	if m.contextMenu != nil {
		t.Fatal("q did not close context menu")
	}

	m.handleMouseAction(mouseAction{kind: mouseOpenContext, node: instance})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.contextMenu != nil {
		t.Fatal("resize retained stale context menu geometry")
	}
}

func TestContextMenuDoesNotBlockGlobalInterruptReview(t *testing.T) {
	m, _ := fixture(t)
	fact := node{role: "plex", instance: "main", key: "token"}
	editValue(t, m, fact.role, fact.instance, fact.key, "changed")
	m.handleMouseAction(mouseAction{kind: mouseOpenContext, node: fact})

	press(m, "ctrl+c")
	if m.mode != reviewing || m.contextMenu != nil || !m.exitAfter {
		t.Fatalf("Ctrl+C did not replace context menu with exit review: mode=%v menu=%+v exit=%v", m.mode, m.contextMenu, m.exitAfter)
	}
}

func TestContextMenuContentUsesApprovedVisualLanguage(t *testing.T) {
	m, _ := fixture(t)
	fact := node{role: "plex", instance: "main", key: "token"}
	m.handleMouseAction(mouseAction{kind: mouseOpenContext, node: fact})

	menu := m.contextMenuContent(78)
	plain := ansi.Strip(menu)
	for _, want := range []string{"ACTIONS", "Edit value", "Add sibling fact", "Stage fact deletion", "↑/↓ choose  Enter run  Esc close", "╭", "╰"} {
		if !strings.Contains(plain, want) {
			t.Errorf("context menu missing %q", want)
		}
	}
	if !strings.Contains(menu, "48;2;125;211;252") {
		t.Error("selected context action does not use the approved cyan fill")
	}
	if width := lipgloss.Width(menu); width > 78 {
		t.Fatalf("context menu width = %d, want at most 78", width)
	}
}

func TestMouseActionsMoveTreeScrollContentAndChooseReview(t *testing.T) {
	m, _ := fixture(t)
	m.handleMouseAction(mouseAction{kind: mouseMoveTree, delta: 1})
	if m.cursor != 1 {
		t.Fatalf("tree cursor = %d, want 1", m.cursor)
	}
	m.handleMouseAction(mouseAction{kind: mouseScrollContent, delta: 1})
	if m.scroll != 1 {
		t.Fatalf("content scroll = %d, want 1", m.scroll)
	}

	editValue(t, m, "plex", "main", "token", "changed")
	press(m, "s")
	m.handleMouseAction(mouseAction{kind: mouseReviewChoice, choice: 2})
	if m.mode != browsing || len(m.changes()) != 1 {
		t.Fatal("mouse Return did not preserve changes and return to browsing")
	}

	press(m, "s")
	m.handleMouseAction(mouseAction{kind: mouseReviewChoice, choice: 1})
	if m.mode != browsing || len(m.changes()) != 0 {
		t.Fatal("mouse Discard did not use existing review behavior")
	}
}

func TestMouseApplyUsesExistingSessionApply(t *testing.T) {
	m, session := fixture(t)
	editValue(t, m, "plex", "main", "token", "changed")
	var applied []facts.Change
	session.apply = func(_ context.Context, changes []facts.Change) (facts.ApplyResult, error) {
		applied = append([]facts.Change(nil), changes...)
		return facts.ApplyResult{Applied: []string{"plex"}}, nil
	}
	press(m, "s")
	cmd := m.handleMouseAction(mouseAction{kind: mouseReviewChoice, choice: 0})
	if cmd == nil || m.mode != applying {
		t.Fatalf("mouse Apply did not enter applying mode: %v", m.mode)
	}
	complete(m, cmd)
	if len(applied) != 1 || applied[0].Value != "changed" || len(m.changes()) != 0 {
		t.Fatalf("mouse Apply changes = %+v, pending = %+v", applied, m.changes())
	}
}

func contextLabels(items []contextItem) []string {
	labels := make([]string, len(items))
	for index, item := range items {
		labels[index] = item.label
	}
	return labels
}
