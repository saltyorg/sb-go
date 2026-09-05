package factui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/saltyorg/sb-go/facts"
)

var (
	benchmarkView tea.View
	benchmarkCmd  tea.Cmd
)

func BenchmarkMouseViewLargeCatalog(b *testing.B) {
	m := benchmarkModel(b)
	for b.Loop() {
		benchmarkView = m.View()
	}
}

func BenchmarkMouseHitTestLargeCatalog(b *testing.B) {
	view := benchmarkModel(b).View()
	event := tea.MouseClickMsg{X: 10, Y: 10, Button: tea.MouseLeft}
	for b.Loop() {
		benchmarkCmd = view.OnMouse(event)
	}
}

func benchmarkModel(tb testing.TB) *Model {
	tb.Helper()
	roles := make([]facts.Role, 100)
	for roleIndex := range roles {
		role := facts.Role{Name: fmt.Sprintf("role-%03d", roleIndex), Instances: make([]facts.Instance, 3)}
		for instanceIndex := range role.Instances {
			instance := facts.Instance{Name: fmt.Sprintf("instance-%02d", instanceIndex), Facts: make([]facts.Fact, 20)}
			for factIndex := range instance.Facts {
				instance.Facts[factIndex] = facts.Fact{Key: fmt.Sprintf("key-%02d", factIndex), Value: fmt.Sprintf("value-%03d-%02d-%02d", roleIndex, instanceIndex, factIndex)}
			}
			role.Instances[instanceIndex] = instance
		}
		roles[roleIndex] = role
	}
	m := New(tb.Context(), &fakeSession{catalog: facts.Catalog{Roles: roles}})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	press(m, "x")
	return m
}
