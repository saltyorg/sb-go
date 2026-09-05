package factui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/saltyorg/sb-go/facts"
)

func TestMouseViewEnablesCellModeAndAdvertisesControls(t *testing.T) {
	m, _ := fixture(t)
	view := m.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v, want cell motion", view.MouseMode)
	}
	if view.OnMouse == nil {
		t.Fatal("view has no mouse handler")
	}
	for _, want := range []string{"click select", "wheel navigate", "right-click actions"} {
		if plain := ansi.Strip(view.Content); !strings.Contains(plain, want) {
			t.Errorf("mouse guidance missing %q", want)
		}
	}
}

func TestMouseSelectsTreeRowsAndOnlyDisclosureToggles(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	instance := node{role: "plex", instance: "main"}
	view := m.View()
	x, y := textPosition(t, view.Content, "main  (2 facts)")
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if m.selected() != instance || m.expanded[instance] {
		t.Fatalf("row click selected=%+v expanded=%v", m.selected(), m.expanded[instance])
	}

	view = m.View()
	x, y = textPosition(t, view.Content, "▸ main")
	sendMouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.selected() != instance || !m.expanded[instance] {
		t.Fatalf("disclosure click selected=%+v expanded=%v", m.selected(), m.expanded[instance])
	}

	before := m.selected()
	view = m.View()
	x, y = textPosition(t, view.Content, "▾ arr")
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft, Mod: tea.ModShift})
	if m.selected() != before {
		t.Fatalf("modified click changed selection to %+v", m.selected())
	}
}

func TestMouseSelectionIsAppliedBeforeOnMouseReturns(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	view := m.View()
	x, y := textPosition(t, view.Content, "main  (2 facts)")
	cmd := view.OnMouse(tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if cmd != nil || m.selected() != (node{role: "plex", instance: "main"}) {
		t.Fatalf("OnMouse returned cmd=%v before selecting %+v", cmd, m.selected())
	}
}

func TestMouseWheelRoutesToTreeAndInspector(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{name: "desktop", width: 120, height: 36},
		{name: "compact", width: 80, height: 24},
	} {
		t.Run(size.name, func(t *testing.T) {
			m, _ := fixture(t)
			m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			view := m.View()
			x, y := textPosition(t, view.Content, "FACT TREE")
			sendMouse(t, m, tea.MouseWheelMsg{X: x + 1, Y: y + 2, Button: tea.MouseWheelDown})
			if m.cursor != 1 {
				t.Fatalf("tree wheel cursor = %d, want 1", m.cursor)
			}

			view = m.View()
			x, y = textPosition(t, view.Content, "INSPECTOR")
			sendMouse(t, m, tea.MouseWheelMsg{X: x + 1, Y: y + 2, Button: tea.MouseWheelDown})
			if m.cursor != 1 || m.scroll != 1 {
				t.Fatalf("inspector wheel cursor=%d scroll=%d", m.cursor, m.scroll)
			}
		})
	}
}

func TestMouseContextMenuDismissesOutsideAndRunsDeletion(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	press(m, "x")
	view := m.View()
	x, y := textPosition(t, view.Content, "• token")
	fact := node{role: "plex", instance: "4k", key: "token"}
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseRight})
	if m.contextMenu == nil || m.selected() != fact {
		t.Fatalf("right click menu=%+v selected=%+v", m.contextMenu, m.selected())
	}
	if plain := ansi.Strip(m.View().Content); !strings.Contains(plain, "ACTIONS") || !strings.Contains(plain, "Stage fact deletion") {
		t.Fatalf("context menu not rendered: %s", plain)
	}

	view = m.View()
	x, y = textPosition(t, view.Content, "▾ arr")
	sendMouse(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.contextMenu != nil || m.selected() != fact {
		t.Fatalf("outside click menu=%+v selected=%+v", m.contextMenu, m.selected())
	}

	view = m.View()
	x, y = textPosition(t, view.Content, "• token")
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseRight})
	view = m.View()
	x, y = textPosition(t, view.Content, "Stage fact deletion")
	cmd := sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if cmd == nil || m.mode != waiting {
		t.Fatalf("context deletion did not enter lock flow: mode=%v", m.mode)
	}
	complete(m, cmd)
	if !m.deleted[fact] || m.mode != browsing {
		t.Fatalf("context deletion not staged: deleted=%v mode=%v", m.deleted[fact], m.mode)
	}
}

func TestMouseContextMenuClampsToCompactScreen(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(m, "x")
	view := m.View()
	_, y := textPosition(t, view.Content, "• token")
	sendMouse(t, m, tea.MouseClickMsg{X: 78, Y: y, Button: tea.MouseRight})
	view = m.View()
	if plain := ansi.Strip(view.Content); !strings.Contains(plain, "ACTIONS") || !strings.Contains(plain, "Esc close") {
		t.Fatalf("compact context menu missing: %s", plain)
	}
	assertViewFits(t, view.Content, 80, 24)
}

func TestMouseContextMenuNonActionAreaDoesNotClickThrough(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	press(m, "x")
	fact := node{role: "plex", instance: "main", key: "token"}
	m.handleMouseAction(mouseAction{kind: mouseOpenContext, node: fact, x: 5, y: 4})

	// The title overlays another tree row, but it is not an action itself.
	sendMouse(t, m, tea.MouseClickMsg{X: 8, Y: 5, Button: tea.MouseLeft})
	if m.contextMenu == nil || m.selected() != fact {
		t.Fatalf("context menu click-through menu=%+v selected=%+v", m.contextMenu, m.selected())
	}
	// The border next to an action row is decoration, not part of the button.
	sendMouse(t, m, tea.MouseClickMsg{X: 6, Y: 7, Button: tea.MouseLeft})
	if m.contextMenu == nil || m.mode != browsing {
		t.Fatalf("context menu border activated action: menu=%+v mode=%v", m.contextMenu, m.mode)
	}
}

func TestMouseRightClickReplacesOpenContextMenu(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	press(m, "x")
	view := m.View()
	x, y := textPosition(t, view.Content, "• token")
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseRight})

	view = m.View()
	x, y = textPosition(t, view.Content, "▾ arr")
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseRight})
	if m.contextMenu == nil || m.contextMenu.node != (node{role: "arr"}) || m.selected() != (node{role: "arr"}) {
		t.Fatalf("replacement context menu=%+v selected=%+v", m.contextMenu, m.selected())
	}
}

func TestMouseReviewButtonsExecuteImmediately(t *testing.T) {
	t.Run("return", func(t *testing.T) {
		m, _ := fixture(t)
		editValue(t, m, "plex", "main", "token", "changed")
		press(m, "s")
		clickText(t, m, "[ Return ]")
		if m.mode != browsing || len(m.changes()) != 1 {
			t.Fatal("clicked Return did not preserve pending change")
		}
	})
	t.Run("discard", func(t *testing.T) {
		m, _ := fixture(t)
		editValue(t, m, "plex", "main", "token", "changed")
		press(m, "s")
		clickText(t, m, "[ Discard ]")
		if m.mode != browsing || len(m.changes()) != 0 {
			t.Fatal("clicked Discard did not clear pending change")
		}
	})
	t.Run("apply", func(t *testing.T) {
		m, session := fixture(t)
		editValue(t, m, "plex", "main", "token", "changed")
		session.apply = func(_ context.Context, _ []facts.Change) (facts.ApplyResult, error) {
			return facts.ApplyResult{Applied: []string{"plex"}}, nil
		}
		press(m, "s")
		cmd := clickText(t, m, "[ Apply ]")
		if cmd == nil || m.mode != applying {
			t.Fatalf("clicked Apply mode=%v cmd=%v", m.mode, cmd)
		}
		complete(m, cmd)
		if m.mode != browsing || len(m.changes()) != 0 {
			t.Fatal("clicked Apply did not apply pending change")
		}
	})
	t.Run("discard and exit", func(t *testing.T) {
		m, _ := fixture(t)
		editValue(t, m, "plex", "main", "token", "changed")
		press(m, "q")
		cmd := clickText(t, m, "[ Discard-and-exit ]")
		if cmd == nil || len(m.changes()) != 0 {
			t.Fatalf("clicked Discard-and-exit cmd=%v pending=%+v", cmd, m.changes())
		}
	})
}

func TestMouseReviewIgnoresOtherButtonsAndValueTextThatLooksLikeButton(t *testing.T) {
	m, session := fixture(t)
	m.catalog.Roles[0].Instances[0].Facts[0].Value = "[ Apply ]"
	editValue(t, m, "plex", "main", "token", "changed")
	session.apply = func(_ context.Context, _ []facts.Change) (facts.ApplyResult, error) {
		return facts.ApplyResult{Applied: []string{"plex"}}, nil
	}
	press(m, "s")

	view := m.View()
	x, y := textPosition(t, view.Content, "[ Apply ]")
	if cmd := sendMouse(t, m, tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft}); cmd != nil || m.mode != reviewing {
		t.Fatalf("value text activated Apply: mode=%v cmd=%v", m.mode, cmd)
	}
	view = m.View()
	x, y = textPositionLast(t, view.Content, "[ Apply ]")
	if cmd := sendMouse(t, m, tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseMiddle}); cmd != nil || m.mode != reviewing {
		t.Fatalf("middle click activated Apply: mode=%v cmd=%v", m.mode, cmd)
	}
	cmd := sendMouse(t, m, tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
	if cmd == nil || m.mode != applying {
		t.Fatalf("footer Apply did not activate: mode=%v cmd=%v", m.mode, cmd)
	}
}

func TestMouseReviewWheelOnlyScrollsInsideModal(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	editValue(t, m, "plex", "main", "token", "changed")
	press(m, "s")
	sendMouse(t, m, tea.MouseWheelMsg{X: 0, Y: 0, Button: tea.MouseWheelDown})
	sendMouse(t, m, tea.MouseWheelMsg{X: 119, Y: 10, Button: tea.MouseWheelDown})
	if m.scroll != 0 {
		t.Fatalf("wheel outside review modal changed scroll to %d", m.scroll)
	}
	view := m.View()
	x, y := textPosition(t, view.Content, "REVIEW 1 PENDING CHANGE(S)")
	sendMouse(t, m, tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown})
	if m.scroll != 1 {
		t.Fatalf("wheel inside review modal scroll = %d, want 1", m.scroll)
	}
}

func TestMouseSearchSelectionAndContextMenu(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	press(m, "/")
	press(m, "plain-secret")
	view := m.View()
	x, y := textPosition(t, view.Content, "• token")
	fact := node{role: "plex", instance: "main", key: "token"}
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if m.mode != searching || m.selected() != fact {
		t.Fatalf("search click mode=%v selected=%+v", m.mode, m.selected())
	}

	view = m.View()
	x, y = textPosition(t, view.Content, "• token")
	sendMouse(t, m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseRight})
	if m.mode != browsing || m.filter != "plain-secret" || m.contextMenu == nil {
		t.Fatalf("search context click mode=%v filter=%q menu=%+v", m.mode, m.filter, m.contextMenu)
	}
}

func TestMouseIgnoresReleaseOutOfBoundsAndEditorClicks(t *testing.T) {
	m, _ := fixture(t)
	before := m.selected()
	if cmd := sendMouse(t, m, tea.MouseReleaseMsg{X: 2, Y: 4, Button: tea.MouseLeft}); cmd != nil || m.selected() != before {
		t.Fatal("mouse release changed browsing state")
	}
	if cmd := sendMouse(t, m, tea.MouseClickMsg{X: -1, Y: -1, Button: tea.MouseLeft}); cmd != nil || m.selected() != before {
		t.Fatal("out-of-bounds click changed browsing state")
	}

	selectNode(t, m, "plex", "main", "token")
	complete(m, press(m, "e"))
	if cmd := sendMouse(t, m, tea.MouseClickMsg{X: 10, Y: 10, Button: tea.MouseLeft}); cmd != nil || m.mode != editing {
		t.Fatalf("editor click changed mode=%v cmd=%v", m.mode, cmd)
	}
	m.mode = applying
	m.scroll = 0
	if cmd := sendMouse(t, m, tea.MouseWheelMsg{X: 40, Y: 15, Button: tea.MouseWheelDown}); cmd != nil || m.scroll != 0 {
		t.Fatalf("applying screen accepted wheel: scroll=%d cmd=%v", m.scroll, cmd)
	}
}

func sendMouse(t *testing.T, m *Model, msg tea.MouseMsg) tea.Cmd {
	t.Helper()
	handler := m.View().OnMouse
	if handler == nil {
		return nil
	}
	cmd := handler(msg)
	if _, updateCmd := m.Update(msg); updateCmd != nil {
		t.Fatal("raw mouse event unexpectedly returned an Update command")
	}
	return cmd
}

func clickText(t *testing.T, m *Model, text string) tea.Cmd {
	t.Helper()
	view := m.View()
	x, y := textPositionLast(t, view.Content, text)
	return sendMouse(t, m, tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
}

func textPosition(t *testing.T, content, text string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(content), "\n") {
		if before, _, ok := strings.Cut(line, text); ok {
			return ansi.StringWidth(before), y
		}
	}
	t.Fatalf("view does not contain %q:\n%s", text, ansi.Strip(content))
	return 0, 0
}

func textPositionLast(t *testing.T, content, text string) (int, int) {
	t.Helper()
	lines := strings.Split(ansi.Strip(content), "\n")
	for y, line := range slices.Backward(lines) {
		if byteIndex := strings.LastIndex(line, text); byteIndex >= 0 {
			return ansi.StringWidth(line[:byteIndex]), y
		}
	}
	t.Fatalf("view does not contain %q:\n%s", text, ansi.Strip(content))
	return 0, 0
}
