package factui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestBrowseViewGoldenLayouts(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height int
	}{
		{name: "desktop", width: 120, height: 36},
		{name: "compact", width: 80, height: 24},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, _ := fixture(t)
			m.Update(tea.WindowSizeMsg{Width: test.width, Height: test.height})
			press(m, "x")
			selectNode(t, m, "plex", "main", "token")
			got := normalizeView(m.View().Content)
			golden, err := os.ReadFile(filepath.Join("testdata", "browse_"+test.name+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSpace(string(golden))
			if got != want {
				t.Fatalf("layout mismatch\n--- want\n%s\n--- got\n%s", want, got)
			}
		})
	}
}

func TestContextMenuGoldenLayouts(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height int
	}{
		{name: "desktop", width: 120, height: 36},
		{name: "compact", width: 80, height: 24},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, _ := fixture(t)
			m.Update(tea.WindowSizeMsg{Width: test.width, Height: test.height})
			press(m, "x")
			m.handleMouseAction(mouseAction{
				kind: mouseOpenContext,
				node: node{role: "plex", instance: "main", key: "token"},
				x:    12,
				y:    8,
			})
			got := normalizeView(m.View().Content)
			golden, err := os.ReadFile(filepath.Join("testdata", "context_"+test.name+".golden"))
			if err != nil {
				t.Fatalf("%v\n--- got\n%s", err, got)
			}
			want := strings.TrimSpace(string(golden))
			if got != want {
				t.Fatalf("context layout mismatch\n--- want\n%s\n--- got\n%s", want, got)
			}
		})
	}
}

func normalizeView(view string) string {
	lines := strings.Split(ansi.Strip(view), "\n")
	for index := range lines {
		lines[index] = strings.Join(strings.Fields(lines[index]), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func TestBrowseViewKeepsApprovedDesktopLayout(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	press(m, "x")
	selectNode(t, m, "plex", "main", "token")

	view := m.View().Content
	plain := ansi.Strip(view)
	for _, want := range []string{
		"◆ Saltbox Facts",
		"2 roles  ·  3 instances  ·  4 facts",
		"FACT TREE",
		"INSPECTOR",
		"Role:      plex",
		"Instance:  main",
		"Key:       token",
		"Status:    unchanged",
		"PLAINTEXT VALUE",
		"plain-secret",
		"PENDING CHANGES",
		"↑/↓ navigate   → expand   ← collapse/parent",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("approved desktop layout missing %q", want)
		}
	}
	if !strings.Contains(plain, "╭") || !strings.Contains(plain, "╰") {
		t.Error("approved rounded panes are missing")
	}
	if !strings.Contains(view, "48;2;125;211;252") {
		t.Error("selected tree row is not filled with the approved cyan background")
	}
	assertViewFits(t, view, 120, 36)

	lines := strings.SplitSeq(plain, "\n")
	for line := range lines {
		if strings.Count(line, "╭") != 2 {
			continue
		}
		first := strings.Index(line, "╭")
		second := strings.Index(line[first+len("╭"):], "╭") + first + len("╭")
		leftWidth := ansi.StringWidth(line[first:second])
		rightWidth := ansi.StringWidth(line[second:])
		if delta := leftWidth - rightWidth; delta < -12 || delta > 12 {
			t.Errorf("desktop panes are not approximately equal: left=%d right=%d", leftWidth, rightWidth)
		}
		return
	}
	t.Error("desktop view does not place the tree and inspector panes side by side")
}

func TestBrowseViewKeepsApprovedCompactLayout(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(m, "x")
	selectNode(t, m, "plex", "main", "token")

	view := m.View().Content
	plain := ansi.Strip(view)
	for _, want := range []string{
		"◆ Saltbox Facts",
		"FACT TREE",
		"INSPECTOR",
		"Role:      plex",
		"Instance:  main",
		"plain-secret",
		"↑/↓ navigate",
		"[/] inspect",
		"s review changes",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("approved compact layout missing %q", want)
		}
	}
	if strings.Count(plain, "╭") < 2 || strings.Count(plain, "╰") < 2 {
		t.Error("compact view does not retain both framed panes")
	}
	assertViewFits(t, view, 80, 24)
}

func TestCompactInspectorCanScrollToPendingChanges(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	selectNode(t, m, "plex", "main", "token")
	complete(m, press(m, "e"))
	m.value.SetValue("changed")
	press(m, "enter")
	for range 3 {
		press(m, "]")
	}

	view := m.View().Content
	plain := ansi.Strip(view)
	for _, want := range []string{"PENDING CHANGES", "PRESS S TO REVIEW", "Set plex / main / token"} {
		if !strings.Contains(plain, want) {
			t.Errorf("scrolled compact inspector missing %q", want)
		}
	}
	assertViewFits(t, view, 80, 24)
}

func TestEditorAndReviewKeepApprovedModalLayout(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	selectNode(t, m, "plex", "main", "token")
	complete(m, press(m, "e"))

	form := m.View().Content
	formPlain := ansi.Strip(form)
	for _, want := range []string{"EDIT FACT VALUE", "Role:      plex", "Instance:  main", "Key:       token", "PLAINTEXT VALUE", "Enter stage", "Esc cancel"} {
		if !strings.Contains(formPlain, want) {
			t.Errorf("approved editor modal missing %q", want)
		}
	}
	if !strings.Contains(formPlain, "╭") || !strings.Contains(formPlain, "╰") {
		t.Error("editor modal is not framed")
	}
	assertViewFits(t, form, 80, 24)

	m.value.SetValue("changed")
	press(m, "enter")
	press(m, "s")
	review := m.View().Content
	reviewPlain := ansi.Strip(review)
	for _, want := range []string{"REVIEW 1 PENDING CHANGE(S)", "Before: plain-secret", "After:  changed", "Apply", "Discard", "Return", "a Apply"} {
		if !strings.Contains(reviewPlain, want) {
			t.Errorf("approved review modal missing %q", want)
		}
	}
	if !strings.Contains(reviewPlain, "╭") || !strings.Contains(reviewPlain, "╰") {
		t.Error("review modal is not framed")
	}
	assertViewFits(t, review, 80, 24)
}

func assertViewFits(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Errorf("view has %d lines, terminal height is %d", len(lines), height)
	}
	for lineNumber, line := range lines {
		if got := ansi.StringWidth(line); got > width {
			t.Errorf("line %d is %d cells wide, terminal width is %d", lineNumber+1, got, width)
		}
	}
}
