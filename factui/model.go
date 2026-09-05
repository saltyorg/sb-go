// Package factui provides the interactive editor for existing Saltbox facts.
package factui

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/saltyorg/sb-go/facts"
)

// Session is the persistence boundary consumed by the editor. Run closes it.
type Session interface {
	Catalog() facts.Catalog
	LockRole(context.Context, string) (*facts.Drift, error)
	ReloadRole(string) error
	ReleaseRole(string) error
	Apply(context.Context, []facts.Change) (facts.ApplyResult, error)
	Close() error
}

var _ Session = (*facts.Session)(nil)

type mode uint8

const (
	browsing mode = iota
	searching
	adding
	editing
	waiting
	driftReview
	reviewing
	applying
	reloading
)

// node identifies a tree location without conflating names containing slashes.
type node struct{ role, instance, key string }

func (n node) parent() node {
	if n.key != "" {
		n.key = ""
	} else {
		n.instance = ""
	}
	return n
}

// Model is a Bubble Tea model. New initializes it from an independent catalog.
type Model struct {
	ctx                   context.Context
	session               Session
	catalog               facts.Catalog
	mode                  mode
	expanded              map[node]bool
	edits                 map[node]string
	deleted               map[node]bool
	locks                 map[string]bool
	cursor, width, height int
	search, key           textinput.Model
	value                 textarea.Model
	filter                string
	valueFocus            bool
	escapedValue          bool
	target                node
	action                string
	cancelLock            context.CancelFunc
	lockCanceled          bool
	reviewAfterCancel     bool
	drift                 *facts.Drift
	exitAfter             bool
	reviewCursor, scroll  int
	err                   error
	notice                string
	outcome               string
}

// New creates an editor; session ownership remains with the caller until Run.
func New(ctx context.Context, session Session) *Model {
	m := &Model{ctx: ctx, session: session, catalog: session.Catalog(), expanded: make(map[node]bool), edits: make(map[node]string), deleted: make(map[node]bool), locks: make(map[string]bool), width: 100, height: 30}
	m.search = textinput.New()
	m.search.Prompt = "/ "
	m.key = textinput.New()
	m.key.Prompt = "Key:   "
	m.value = textarea.New()
	m.value.Prompt = ""
	m.value.ShowLineNumbers = false
	m.value.CharLimit = 0
	m.value.MaxHeight = 0
	m.value.MaxWidth = 0
	m.value.SetWidth(m.width - 4)
	m.value.SetHeight(6)
	m.value.KeyMap.InsertNewline.SetKeys("alt+enter")
	return m
}

// Run owns the session until the terminal exits, then cancels any lock wait and
// closes the session. SIGTERM is supplied by the caller through ctx; Ctrl+C is
// delivered as a key event so it participates in the normal review flow.
func Run(ctx context.Context, session Session, input io.Reader, output io.Writer) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() { cancel(); err = errors.Join(err, session.Close()) }()
	_, err = tea.NewProgram(New(ctx, session), tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithoutSignalHandler()).Run()
	return err
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) baseline(n node) (string, bool) {
	for _, r := range m.catalog.Roles {
		if r.Name != n.role {
			continue
		}
		for _, i := range r.Instances {
			if i.Name != n.instance {
				continue
			}
			for _, f := range i.Facts {
				if f.Key == n.key {
					return f.Value, true
				}
			}
		}
	}
	return "", false
}
func (m *Model) factValue(n node) string {
	if value, ok := m.edits[n]; ok {
		return value
	}
	value, _ := m.baseline(n)
	return value
}
func (m *Model) blocked(n node) bool {
	return n.instance != "" && m.deleted[node{role: n.role}] || n.key != "" && m.deleted[node{role: n.role, instance: n.instance}]
}
func (m *Model) marked(n node) bool { return m.deleted[n] || m.blocked(n) }

// changes returns the effective, deterministic commit set. Keeping child edits
// separately lets toggling a parent deletion restore the user's staged work.
func (m *Model) changes() []facts.Change {
	var changes []facts.Change
	for n := range m.deleted {
		if m.blocked(n) {
			continue
		}
		kind := facts.DeleteRole
		if n.instance != "" {
			kind = facts.DeleteInstance
		}
		if n.key != "" {
			if _, exists := m.baseline(n); !exists {
				continue
			}
			kind = facts.DeleteFact
		}
		changes = append(changes, facts.Change{Kind: kind, Role: n.role, Instance: n.instance, Key: n.key})
	}
	for n, value := range m.edits {
		if m.marked(n) {
			continue
		}
		original, exists := m.baseline(n)
		if exists && value == original {
			continue
		}
		changes = append(changes, facts.Change{Kind: facts.SetFact, Role: n.role, Instance: n.instance, Key: n.key, Value: value})
	}
	slices.SortFunc(changes, func(a, b facts.Change) int {
		if c := strings.Compare(a.Role, b.Role); c != 0 {
			return c
		}
		if c := strings.Compare(a.Instance, b.Instance); c != 0 {
			return c
		}
		return strings.Compare(a.Key, b.Key)
	})
	return changes
}

func (m *Model) rows() []node {
	roles := slices.Clone(m.catalog.Roles)
	slices.SortFunc(roles, func(a, b facts.Role) int { return strings.Compare(a.Name, b.Name) })
	query := strings.ToLower(m.filter)
	matches := func(s string) bool { return query == "" || strings.Contains(strings.ToLower(s), query) }
	var rows []node
	for _, role := range roles {
		r := node{role: role.Name}
		roleMatch := matches(role.Name)
		var children []node
		instances := slices.Clone(role.Instances)
		slices.SortFunc(instances, func(a, b facts.Instance) int { return strings.Compare(a.Name, b.Name) })
		for _, instance := range instances {
			i := node{role: role.Name, instance: instance.Name}
			instanceMatch := roleMatch || matches(instance.Name)
			keys := make(map[string]bool)
			for _, fact := range instance.Facts {
				keys[fact.Key] = true
			}
			for n := range m.edits {
				if n.role == i.role && n.instance == i.instance {
					keys[n.key] = true
				}
			}
			var names []string
			for key := range keys {
				names = append(names, key)
			}
			slices.Sort(names)
			var leaves []node
			for _, key := range names {
				n := node{role: i.role, instance: i.instance, key: key}
				if instanceMatch || matches(key) || matches(m.factValue(n)) {
					leaves = append(leaves, n)
				}
			}
			if instanceMatch || len(leaves) > 0 {
				children = append(children, i)
				if query != "" || m.expanded[i] {
					children = append(children, leaves...)
				}
			}
		}
		if roleMatch || len(children) > 0 {
			rows = append(rows, r)
			if query != "" || m.expanded[r] {
				rows = append(rows, children...)
			}
		}
	}
	return rows
}
func (m *Model) selected() node {
	rows := m.rows()
	if len(rows) == 0 {
		return node{}
	}
	m.cursor = max(0, min(m.cursor, len(rows)-1))
	return rows[m.cursor]
}
func (m *Model) selectTarget(n node) {
	for i, row := range m.rows() {
		if row == n {
			m.cursor = i
			return
		}
	}
	m.selected()
}
