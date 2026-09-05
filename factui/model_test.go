package factui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/saltyorg/sb-go/facts"
)

type fakeSession struct {
	catalog  facts.Catalog
	lock     func(context.Context, string) (*facts.Drift, error)
	reload   func(string) error
	apply    func(context.Context, []facts.Change) (facts.ApplyResult, error)
	released []string
	closed   bool
}

func (f *fakeSession) Catalog() facts.Catalog { return f.catalog }
func (f *fakeSession) LockRole(ctx context.Context, role string) (*facts.Drift, error) {
	if f.lock != nil {
		return f.lock(ctx, role)
	}
	return nil, nil
}
func (f *fakeSession) ReloadRole(role string) error {
	if f.reload != nil {
		return f.reload(role)
	}
	return nil
}
func (f *fakeSession) ReleaseRole(role string) error {
	f.released = append(f.released, role)
	return nil
}
func (f *fakeSession) Apply(ctx context.Context, c []facts.Change) (facts.ApplyResult, error) {
	return f.apply(ctx, c)
}
func (f *fakeSession) Close() error { f.closed = true; return nil }

func fixture(t *testing.T) (*Model, *fakeSession) {
	t.Helper()
	f := &fakeSession{catalog: facts.Catalog{Roles: []facts.Role{
		{Name: "plex", Instances: []facts.Instance{
			{Name: "main", Facts: []facts.Fact{{Key: "token", Value: "plain-secret"}, {Key: "host", Value: "localhost"}}},
			{Name: "4k", Facts: []facts.Fact{{Key: "token", Value: "second"}}},
		}},
		{Name: "arr", Instances: []facts.Instance{{Name: "arr", Facts: []facts.Fact{{Key: "port", Value: "8000"}}}}},
	}}}
	return New(t.Context(), f), f
}

func press(m *Model, key string) tea.Cmd {
	k := tea.Key{Text: key}
	switch key {
	case "enter":
		k.Code = tea.KeyEnter
	case "esc":
		k.Code = tea.KeyEscape
	case "up":
		k.Code = tea.KeyUp
	case "down":
		k.Code = tea.KeyDown
	case "left":
		k.Code = tea.KeyLeft
	case "right":
		k.Code = tea.KeyRight
	case "tab":
		k.Code = tea.KeyTab
	case "ctrl+c":
		k.Code = 'c'
		k.Mod = tea.ModCtrl
		k.Text = ""
	default:
		if len(key) == 1 {
			k.Code = rune(key[0])
		}
	}
	_, cmd := m.Update(tea.KeyPressMsg(k))
	return cmd
}

func complete(m *Model, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	_, next := m.Update(cmd())
	return next
}
func selectNode(t *testing.T, m *Model, role, instance, key string) {
	t.Helper()
	press(m, "x")
	for i, n := range m.rows() {
		if n.role == role && n.instance == instance && n.key == key {
			m.cursor = i
			return
		}
	}
	t.Fatalf("missing node %s/%s/%s", role, instance, key)
}
func editValue(t *testing.T, m *Model, role, instance, key, value string) {
	t.Helper()
	selectNode(t, m, role, instance, key)
	complete(m, press(m, "e"))
	m.value.SetValue(value)
	press(m, "enter")
}

func TestTreeSearchAndInspector(t *testing.T) {
	m, _ := fixture(t)
	if got := m.rows(); len(got) != 2 || got[0].role != "arr" {
		t.Fatalf("sorted collapsed roles: %+v", got)
	}
	press(m, "right")
	press(m, "down")
	press(m, "right")
	press(m, "down")
	if n := m.selected(); n.key != "port" {
		t.Fatalf("arrow navigation: %+v", n)
	}
	press(m, "left")
	if n := m.selected(); n.instance != "arr" || n.key != "" {
		t.Fatalf("parent navigation: %+v", n)
	}
	selectNode(t, m, "plex", "main", "token")
	v := m.inspector()
	for _, want := range []string{"Role:     plex", "Instance: main", "Key:      token", "Status:   unchanged", "plain-secret"} {
		if !strings.Contains(v, want) {
			t.Fatalf("inspector missing %q: %s", want, v)
		}
	}
	if strings.Contains(m.treeView(50, 20), "plain-secret") {
		t.Fatal("value leaked into tree")
	}
	press(m, "/")
	press(m, "plain-secret")
	press(m, "enter")
	got := m.rows()
	if len(got) != 3 || got[0].role != "plex" || got[1].instance != "main" || got[2].key != "token" {
		t.Fatalf("filtered ancestors: %+v", got)
	}
}

func TestEffectiveChangesAndDeletionToggles(t *testing.T) {
	m, _ := fixture(t)
	editValue(t, m, "plex", "main", "token", "changed")
	if c := m.changes(); len(c) != 1 || c[0].Key != "token" || c[0].Value != "changed" {
		t.Fatalf("edit: %+v", c)
	}
	editValue(t, m, "plex", "main", "token", "plain-secret")
	if len(m.changes()) != 0 {
		t.Fatal("reverted edit remains pending")
	}
	selectNode(t, m, "plex", "main", "")
	press(m, "a")
	m.key.SetValue("extra")
	m.value.SetValue("value")
	press(m, "enter")
	selectNode(t, m, "plex", "main", "extra")
	press(m, "d")
	if len(m.changes()) != 0 {
		t.Fatalf("deleted new key remains pending: %+v", m.changes())
	}
	press(m, "d")
	if len(m.changes()) != 1 {
		t.Fatal("new key deletion did not toggle")
	}
	selectNode(t, m, "plex", "main", "")
	press(m, "d")
	if c := m.changes(); len(c) != 1 || c[0].Kind != facts.DeleteInstance {
		t.Fatalf("parent coalescing: %+v", c)
	}
	selectNode(t, m, "plex", "main", "token")
	press(m, "e")
	if m.mode != browsing {
		t.Fatal("marked parent allowed descendant edit")
	}
	press(m, "d")
	if len(m.changes()) != 1 {
		t.Fatal("marked parent allowed descendant delete")
	}
	selectNode(t, m, "plex", "main", "")
	press(m, "d")
	if c := m.changes(); len(c) != 1 || c[0].Key != "extra" {
		t.Fatalf("unmark lost child changes: %+v", c)
	}
	selectNode(t, m, "plex", "", "")
	press(m, "a")
	if m.mode != browsing {
		t.Fatal("role allowed adding instance")
	}
}

func TestEditDoesNotRenameKeyAndAddRejectsDuplicate(t *testing.T) {
	m, _ := fixture(t)
	selectNode(t, m, "plex", "main", "token")
	complete(m, press(m, "e"))
	m.key.SetValue("renamed")
	m.value.SetValue("changed")
	press(m, "enter")
	if c := m.changes(); len(c) != 1 || c[0].Key != "token" {
		t.Fatalf("edit renamed key: %+v", c)
	}
	press(m, "a")
	m.key.SetValue("token")
	m.value.SetValue("duplicate")
	press(m, "enter")
	if m.mode != adding || m.err == nil {
		t.Fatal("duplicate key accepted")
	}
}

func TestLockWaitCancelAndTimeout(t *testing.T) {
	m, f := fixture(t)
	f.lock = func(ctx context.Context, role string) (*facts.Drift, error) { <-ctx.Done(); return nil, ctx.Err() }
	selectNode(t, m, "plex", "main", "token")
	cmd := press(m, "e")
	if cmd == nil || m.mode != waiting || !strings.Contains(m.View().Content, "Esc") {
		t.Fatal("lock did not enter cancellable wait")
	}
	press(m, "esc")
	complete(m, cmd)
	if m.mode != browsing || len(m.changes()) != 0 || len(m.locks) != 0 {
		t.Fatal("cancel staged mutation or retained lock")
	}
	f.lock = func(context.Context, string) (*facts.Drift, error) { return nil, context.DeadlineExceeded }
	complete(m, press(m, "e"))
	if m.err == nil || m.mode != browsing || len(m.changes()) != 0 {
		t.Fatal("timeout not surfaced safely")
	}
}

func TestDriftReloadRequiresRetryAndCancelReleases(t *testing.T) {
	for _, reload := range []bool{true, false} {
		t.Run(map[bool]string{true: "reload", false: "cancel"}[reload], func(t *testing.T) {
			m, f := fixture(t)
			f.lock = func(context.Context, string) (*facts.Drift, error) {
				return &facts.Drift{Before: f.catalog.Roles[0], After: facts.Role{Name: "plex", Instances: []facts.Instance{{Name: "main", Facts: []facts.Fact{{Key: "token", Value: "external"}}}}}}, nil
			}
			f.reload = func(string) error { f.catalog.Roles[0].Instances[0].Facts[0].Value = "external"; return nil }
			selectNode(t, m, "plex", "main", "token")
			complete(m, press(m, "e"))
			if m.mode != driftReview || !strings.Contains(m.View().Content, "external") {
				t.Fatal("semantic drift review absent")
			}
			if reload {
				complete(m, press(m, "r"))
				if len(m.locks) != 1 || len(m.changes()) != 0 || m.mode != browsing {
					t.Fatal("reload must retain lock and cancel attempted edit")
				}
				selectNode(t, m, "plex", "main", "token")
				press(m, "e")
				if m.value.Value() != "external" {
					t.Fatal("manual retry did not use refreshed baseline")
				}
			} else {
				complete(m, press(m, "esc"))
				if len(m.locks) != 0 || !reflect.DeepEqual(f.released, []string{"plex"}) || m.mode != browsing {
					t.Fatal("drift cancel did not release")
				}
			}
		})
	}
}

func TestUnifiedReviewAndMultiInstanceWarning(t *testing.T) {
	for _, trigger := range []string{"s", "q", "ctrl+c"} {
		t.Run(trigger, func(t *testing.T) {
			m, _ := fixture(t)
			selectNode(t, m, "plex", "", "")
			complete(m, press(m, "d"))
			press(m, trigger)
			if m.mode != reviewing || m.exitAfter != (trigger != "s") {
				t.Fatal("wrong review state")
			}
			v := m.View().Content
			for _, want := range []string{"main", "4k", "Apply", "Discard", "Return", "Locks: 1 (plex)"} {
				if !strings.Contains(v, want) {
					t.Fatalf("review missing %q: %s", want, v)
				}
			}
			press(m, "r")
			if m.mode != browsing || len(m.changes()) != 1 {
				t.Fatal("return lost changes")
			}
			press(m, trigger)
			cmd := press(m, "d")
			if len(m.changes()) != 0 {
				t.Fatal("discard kept pending")
			}
			if trigger != "s" && cmd == nil {
				t.Fatal("discard-and-exit did not quit")
			}
		})
	}
}

func TestPartialApplyKeepsOnlyUnappliedRolesAndStaysOpen(t *testing.T) {
	m, f := fixture(t)
	editValue(t, m, "plex", "main", "token", "changed")
	editValue(t, m, "arr", "arr", "port", "9000")
	f.apply = func(context.Context, []facts.Change) (facts.ApplyResult, error) {
		return facts.ApplyResult{Applied: []string{"arr"}, Failed: &facts.RoleFailure{Role: "arr", Err: errors.New("directory fsync")}, Unattempted: []string{"plex"}}, errors.New("directory fsync")
	}
	press(m, "q")
	cmd := complete(m, press(m, "a"))
	if cmd != nil || m.mode != reviewing || m.err == nil {
		t.Fatal("failed apply-and-exit quit or lost error")
	}
	if c := m.changes(); len(c) != 1 || c[0].Role != "plex" {
		t.Fatalf("partial apply pending: %+v", c)
	}
	if len(m.locks) != 2 {
		t.Fatal("apply released role locks")
	}
	f.apply = func(context.Context, []facts.Change) (facts.ApplyResult, error) {
		return facts.ApplyResult{Applied: []string{"plex"}}, nil
	}
	if cmd := complete(m, press(m, "a")); cmd == nil || len(m.changes()) != 0 {
		t.Fatal("successful apply-and-exit did not quit")
	}
}

func TestContextCancellationDoesNotStageMutation(t *testing.T) {
	m, f := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	m.ctx = ctx
	selectNode(t, m, "plex", "main", "token")
	cmd := press(m, "e")
	cancel()
	complete(m, cmd)
	if len(m.changes()) != 0 || m.mode == editing {
		t.Fatal("cancellation opened mutation")
	}
	if f.closed {
		t.Fatal("model unexpectedly owns session close outside Run")
	}
}

func TestCanceledSuccessfulLockBlocksNavigationUntilReleased(t *testing.T) {
	m, f := fixture(t)
	selectNode(t, m, "plex", "main", "token")
	cmd := press(m, "e")
	press(m, "esc")
	release := complete(m, cmd)
	if release == nil {
		t.Fatal("lock won cancellation race but was not released")
	}
	press(m, "up")
	press(m, "d")
	if len(m.changes()) != 0 || m.mode != reloading {
		t.Fatal("new mutation accepted before cancelled lock released")
	}
	complete(m, release)
	if m.mode != browsing || len(f.released) != 1 || f.released[0] != "plex" {
		t.Fatal("cancelled lock did not return to browsing")
	}
}

func TestPartialApplyReportsRoleOutcomes(t *testing.T) {
	m, f := fixture(t)
	editValue(t, m, "plex", "main", "token", "changed")
	editValue(t, m, "arr", "arr", "port", "9000")
	f.apply = func(context.Context, []facts.Change) (facts.ApplyResult, error) {
		return facts.ApplyResult{Applied: []string{"arr"}, Failed: &facts.RoleFailure{Role: "plex", Err: errors.New("permission denied")}}, errors.New("permission denied")
	}
	press(m, "s")
	complete(m, press(m, "a"))
	v := m.View().Content
	for _, want := range []string{"Applied: arr", "Failed: plex"} {
		if !strings.Contains(v, want) {
			t.Fatalf("partial result missing %q: %s", want, v)
		}
	}
}

func TestReviewControlsRemainVisibleAtNormalDimensions(t *testing.T) {
	m, _ := fixture(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	editValue(t, m, "plex", "main", "token", "changed")
	press(m, "q")
	v := m.View().Content
	if !strings.Contains(v, "Return") || !strings.Contains(v, "a Apply") {
		t.Fatalf("review controls clipped: %s", v)
	}
}

func TestQuitDuringLockWaitCancelsThenReviewsPending(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			m, f := fixture(t)
			editValue(t, m, "arr", "arr", "port", "9000")
			f.lock = func(ctx context.Context, _ string) (*facts.Drift, error) { <-ctx.Done(); return nil, ctx.Err() }
			selectNode(t, m, "plex", "main", "token")
			cmd := press(m, "e")
			press(m, key)
			if !m.lockCanceled {
				t.Fatal("exit key did not cancel lock wait")
			}
			complete(m, cmd)
			if m.mode != reviewing || !m.exitAfter || len(m.changes()) != 1 {
				t.Fatal("exit during wait did not preserve pending review")
			}
		})
	}
}

func TestRunCancellationClosesSession(t *testing.T) {
	_, f := fixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var output bytes.Buffer
	err := Run(ctx, f, strings.NewReader(""), &output)
	if err == nil || !f.closed {
		t.Fatalf("cancelled Run returned %v, closed=%v", err, f.closed)
	}
}

func TestModelAppliesThroughRealSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plex.ini")
	if err := os.WriteFile(path, []byte("[main]\ntoken = original\n"), 0640); err != nil {
		t.Fatal(err)
	}
	s, err := facts.OpenSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	m := New(t.Context(), s)
	editValue(t, m, "plex", "main", "token", "changed")
	press(m, "s")
	complete(m, press(m, "a"))
	if m.err != nil || len(m.changes()) != 0 {
		t.Fatalf("apply: %v; %+v", m.err, m.changes())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "changed") || strings.Contains(string(data), "original") {
		t.Fatalf("persisted content: %s", data)
	}
	selectNode(t, m, "plex", "main", "token")
	if !strings.Contains(m.inspector(), "changed") || !strings.Contains(m.inspector(), "unchanged") {
		t.Fatalf("baseline not refreshed: %s", m.inspector())
	}
}

func TestExitFromDriftReleasesNewLockThenReviewsOtherChanges(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			m, f := fixture(t)
			editValue(t, m, "arr", "arr", "port", "9000")
			f.lock = func(context.Context, string) (*facts.Drift, error) {
				return &facts.Drift{Before: f.catalog.Roles[0], After: facts.Role{Name: "plex"}}, nil
			}
			selectNode(t, m, "plex", "main", "token")
			complete(m, press(m, "e"))
			complete(m, press(m, key))
			if m.mode != reviewing || !m.exitAfter || len(m.locks) != 1 || len(m.changes()) != 1 {
				t.Fatal("exit from drift lost pending review or retained drift lock")
			}
		})
	}
}

func TestDiscardKeepsRoleLocksUntilSessionClose(t *testing.T) {
	m, f := fixture(t)
	editValue(t, m, "plex", "main", "token", "changed")
	press(m, "s")
	press(m, "d")
	if len(m.changes()) != 0 || len(m.locks) != 1 || len(f.released) != 0 {
		t.Fatal("discard violated retained role lock ownership")
	}
}

func TestInspectorEscapesTerminalControlBytes(t *testing.T) {
	m, _ := fixture(t)
	m.catalog.Roles[0].Instances[0].Facts[0].Value = "plain\x1b[2Jsecret"
	selectNode(t, m, "plex", "main", "token")
	if value := m.inspector(); strings.Contains(value, "\x1b") || !strings.Contains(value, "plain\\u001b[2Jsecret") {
		t.Fatalf("unsafe or masked value: %q", value)
	}
}

func TestEditPreservesMultilineValues(t *testing.T) {
	m, _ := fixture(t)
	m.catalog.Roles[0].Instances[0].Facts[0].Value = "first\nsecond"
	selectNode(t, m, "plex", "main", "token")
	complete(m, press(m, "e"))
	if got := m.value.Value(); got != "first\nsecond" {
		t.Fatalf("edit flattened value: %q", got)
	}
	press(m, "enter")
	if len(m.changes()) != 0 {
		t.Fatal("unchanged multiline value became pending")
	}
	press(m, "e")
	m.value.SetValue("first\nthird")
	press(m, "enter")
	if c := m.changes(); len(c) != 1 || c[0].Value != "first\nthird" {
		t.Fatalf("multiline edit: %+v", c)
	}
}

func TestAddAcceptsBracketAndColonKeys(t *testing.T) {
	for _, key := range []string{"mount[/data]", "url:port"} {
		t.Run(key, func(t *testing.T) {
			m, _ := fixture(t)
			selectNode(t, m, "plex", "main", "")
			complete(m, press(m, "a"))
			m.key.SetValue(key)
			m.value.SetValue("value")
			press(m, "enter")
			if c := m.changes(); len(c) != 1 || c[0].Key != key {
				t.Fatalf("valid INI key rejected: %+v, %v", c, m.err)
			}
		})
	}
}

func TestEditingTabValueDoesNotSilentlyChangeItsBytes(t *testing.T) {
	m, _ := fixture(t)
	m.catalog.Roles[0].Instances[0].Facts[0].Value = "first\tsecond"
	selectNode(t, m, "plex", "main", "token")
	complete(m, press(m, "e"))
	press(m, "enter")
	if len(m.changes()) != 0 {
		t.Fatalf("opening and staging replaced tab: %+v", m.changes())
	}
	press(m, "e")
	m.value.SetValue("first\\tthird")
	press(m, "enter")
	if c := m.changes(); len(c) != 1 || c[0].Value != "first\tthird" {
		t.Fatalf("escaped tab edit: %+v", c)
	}
}
