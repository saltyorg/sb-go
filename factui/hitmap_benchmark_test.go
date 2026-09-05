package factui

import tea "charm.land/bubbletea/v2"

func benchmarkMouseResolver(m *Model) func(tea.MouseMsg) mouseAction {
	geometry := calculateMainGeometry(m.width, m.height)
	screen := mouseScreen{targets: m.mainMouseTargets(geometry)}
	return screen.action
}
