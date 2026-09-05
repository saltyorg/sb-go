package factui

import (
	"image"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type mouseTarget struct {
	bounds             image.Rectangle
	left, right        mouseAction
	wheelUp, wheelDown mouseAction
	menu               bool
}

type mouseScreen struct {
	content    string
	targets    []mouseTarget
	menuBounds image.Rectangle
}

type mainGeometry struct {
	tree, inspector image.Rectangle
}

type treeWindow struct {
	rows         []node
	start, end   int
	prefixHeight int
}

func calculateMainGeometry(width, height int) mainGeometry {
	bodyHeight := max(1, height-5)
	if width >= 96 {
		treeWidth := min(56, max(40, width/2-5))
		return mainGeometry{
			tree:      image.Rect(0, 1, treeWidth, 1+bodyHeight),
			inspector: image.Rect(treeWidth, 1, width, 1+bodyHeight),
		}
	}
	treeHeight := min(max(8, (bodyHeight+1)/2), bodyHeight)
	return mainGeometry{
		tree:      image.Rect(0, 1, width, 1+treeHeight),
		inspector: image.Rect(0, 1+treeHeight, width, 1+bodyHeight),
	}
}

func (m *Model) treeWindow(width, height int) treeWindow {
	rows := m.rows()
	if len(rows) == 0 {
		return treeWindow{}
	}
	m.cursor = min(max(0, m.cursor), len(rows)-1)
	prefixHeight := 0
	if m.mode == searching || m.filter != "" {
		search := m.search.View()
		if m.mode != searching {
			search = mutedStyle.Render("Search  " + display(m.filter))
		}
		prefixHeight = lipgloss.Height(ansi.Hardwrap(search, max(1, width), false)) + 2
	}
	start, end := visibleWindow(m.cursor, len(rows), height-prefixHeight)
	return treeWindow{rows: rows, start: start, end: end, prefixHeight: prefixHeight}
}

func (m *Model) mainMouseTargets(geometry mainGeometry) []mouseTarget {
	targets := []mouseTarget{
		{
			bounds:    geometry.tree,
			wheelUp:   mouseAction{kind: mouseMoveTree, delta: -1},
			wheelDown: mouseAction{kind: mouseMoveTree, delta: 1},
		},
	}
	if geometry.inspector.Dx() > 0 && geometry.inspector.Dy() > 0 {
		targets = append(targets, mouseTarget{
			bounds:    geometry.inspector,
			wheelUp:   mouseAction{kind: mouseScrollContent, delta: -1},
			wheelDown: mouseAction{kind: mouseScrollContent, delta: 1},
		})
	}
	contentWidth := max(1, geometry.tree.Dx()-4)
	window := m.treeWindow(contentWidth, max(1, geometry.tree.Dy()-4))
	contentX := geometry.tree.Min.X + 2
	rowY := geometry.tree.Min.Y + 3 + window.prefixHeight
	for index := window.start; index < window.end; index++ {
		n := window.rows[index]
		y := rowY + index - window.start
		if y < geometry.tree.Min.Y || y >= geometry.tree.Max.Y {
			continue
		}
		open := mouseAction{kind: mouseOpenContext, node: n}
		targets = append(targets, mouseTarget{
			bounds: image.Rect(geometry.tree.Min.X+1, y, geometry.tree.Max.X-1, y+1),
			left:   mouseAction{kind: mouseSelect, node: n},
			right:  open,
		})
		if n.key == "" && m.mode != searching && m.filter == "" {
			arrowX := contentX
			if n.instance != "" {
				arrowX += 2
			}
			targets = append(targets, mouseTarget{
				bounds: image.Rect(arrowX, y, arrowX+1, y+1),
				left:   mouseAction{kind: mouseToggle, node: n},
				right:  open,
			})
		}
	}
	return targets
}

func (m *Model) contextMenuOverlay(base string, width, height int) (string, []mouseTarget, image.Rectangle) {
	menu := m.contextMenuContent(max(4, width-2))
	if menu == "" {
		return base, nil, image.Rectangle{}
	}
	menuWidth, menuHeight := lipgloss.Width(menu), lipgloss.Height(menu)
	x := min(max(0, m.contextMenu.x+1), max(0, width-menuWidth))
	y := min(max(0, m.contextMenu.y), max(0, height-menuHeight))
	bounds := image.Rect(x, y, x+menuWidth, y+menuHeight)
	baseLayer := lipgloss.NewLayer(base)
	menuLayer := lipgloss.NewLayer(menu).X(x).Y(y).Z(1)
	content := lipgloss.NewCompositor(baseLayer, menuLayer).Render()
	items := m.contextItems(m.contextMenu.node)
	targets := make([]mouseTarget, 0, len(items))
	for index := range items {
		itemY := y + 3 + index
		targets = append(targets, mouseTarget{
			bounds: image.Rect(x+1, itemY, x+menuWidth-1, itemY+1),
			left:   mouseAction{kind: mouseRunContextChoice, choice: index},
			menu:   true,
		})
	}
	return content, targets, bounds
}

func modalBounds(content string) image.Rectangle {
	plain := ansi.Strip(content)
	lines := strings.Split(plain, "\n")
	for y, line := range lines {
		start := strings.Index(line, "╭")
		if start < 0 {
			continue
		}
		x := ansi.StringWidth(line[:start])
		width := ansi.StringWidth(line[start:])
		for bottom := len(lines) - 1; bottom >= y; bottom-- {
			if strings.Contains(lines[bottom], "╰") {
				return image.Rect(x, y, x+width, bottom+1)
			}
		}
	}
	return image.Rectangle{}
}

func textBounds(content, text string) (image.Rectangle, bool) {
	for y, line := range strings.Split(ansi.Strip(content), "\n") {
		before, _, ok := strings.Cut(line, text)
		if !ok {
			continue
		}
		x := ansi.StringWidth(before)
		return image.Rect(x, y, x+ansi.StringWidth(text), y+1), true
	}
	return image.Rectangle{}, false
}

func (screen mouseScreen) onMouse(msg tea.MouseMsg) tea.Cmd {
	event := msg.Mouse()
	if event.Mod != 0 {
		return nil
	}
	point := image.Pt(event.X, event.Y)
	switch msg.(type) {
	case tea.MouseWheelMsg:
		if !screen.menuBounds.Empty() {
			return nil
		}
		for _, target := range slices.Backward(screen.targets) {

			if !point.In(target.bounds) {
				continue
			}
			action := mouseAction{}
			if event.Button == tea.MouseWheelUp {
				action = target.wheelUp
			}
			if event.Button == tea.MouseWheelDown {
				action = target.wheelDown
			}
			if action.kind != 0 {
				return mouseCommand(action)
			}
		}
	case tea.MouseClickMsg:
		if !screen.menuBounds.Empty() {
			if point.In(screen.menuBounds) {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return screen.clickCommand(point, event.Button, false)
			}
			if event.Button == tea.MouseLeft {
				return mouseCommand(mouseAction{kind: mouseDismissContext})
			}
		}
		return screen.clickCommand(point, event.Button, true)
	}
	return nil
}

func (screen mouseScreen) clickCommand(point image.Point, button tea.MouseButton, includeUnderlying bool) tea.Cmd {
	for _, target := range slices.Backward(screen.targets) {

		if !includeUnderlying && !target.menu {
			continue
		}
		if !point.In(target.bounds) {
			continue
		}
		action := target.left
		if button == tea.MouseRight {
			action = target.right
			action.x, action.y = point.X, point.Y
		}
		if action.kind != 0 {
			return mouseCommand(action)
		}
		if !includeUnderlying {
			return nil
		}
	}
	return nil
}

func mouseCommand(action mouseAction) tea.Cmd {
	if action.kind == 0 {
		return nil
	}
	return func() tea.Msg { return mouseActionMsg{action: action} }
}
