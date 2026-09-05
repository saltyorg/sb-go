package facts

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
	"unicode"

	"golang.org/x/sys/unix"
)

type ChangeKind uint8

const (
	SetFact ChangeKind = iota + 1
	DeleteFact
	DeleteInstance
	DeleteRole
)

// Change is one effective mutation. Apply rejects duplicate targets and parent
// deletions combined with descendant edits. SetFact adds or updates a key only
// within an existing instance; it cannot create roles or instances.
type Change struct {
	Kind                       ChangeKind
	Role, Instance, Key, Value string
}
type RoleFailure struct {
	Role string
	Err  error
}
type ApplyResult struct {
	Applied     []string
	Failed      *RoleFailure
	Unattempted []string
}

type replacement struct {
	role      string
	temporary string
	data      []byte
	info      os.FileInfo
	delete    bool
	noop      bool
	catalog   Role
}

// Apply preflights and fsyncs all replacements before committing in sorted role
// order. No rollback is attempted. A role may be both Applied and Failed when
// its rename/delete succeeded but directory fsync failed: the visible mutation
// must not be retried. All other failed and unattempted changes remain pending
// in the caller. Apply never releases role locks.
func (s *Session) Apply(ctx context.Context, changes []Change) (ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := ApplyResult{}
	if s.dir == nil {
		return result, ErrClosed
	}
	grouped := make(map[string][]Change)
	for _, change := range changes {
		grouped[change.Role] = append(grouped[change.Role], change)
	}
	roles := make([]string, 0, len(grouped))
	for role := range grouped {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	prepared := make([]replacement, 0, len(roles))
	defer func() {
		for _, item := range prepared {
			if item.temporary != "" {
				_ = unix.Unlinkat(int(s.dir.Fd()), item.temporary, 0)
			}
		}
	}()
	fail := func(role string, err error, unattempted []string) (ApplyResult, error) {
		result.Failed = &RoleFailure{Role: role, Err: err}
		result.Unattempted = unattempted
		return result, fmt.Errorf("apply role %s: %w", role, err)
	}
	for _, role := range roles {
		item, err := s.prepare(ctx, role, grouped[role])
		if err != nil {
			pending := slices.Clone(roles)
			pending = slices.DeleteFunc(pending, func(name string) bool { return name == role })
			return fail(role, err, pending)
		}
		prepared = append(prepared, item)
	}
	for i, item := range prepared {
		if err := ctx.Err(); err != nil {
			return fail(item.role, err, slices.Clone(roles[i+1:]))
		}
		state := s.roles[item.role]
		if err := s.verifyBaseline(item.role, state); err != nil {
			return fail(item.role, err, slices.Clone(roles[i+1:]))
		}
		if !item.noop {
			var err error
			if item.delete {
				err = unix.Unlinkat(int(s.dir.Fd()), item.role+".ini", 0)
			} else {
				err = s.rename(item.temporary, item.role+".ini")
			}
			if err != nil {
				return fail(item.role, err, slices.Clone(roles[i+1:]))
			}
			state.applied = true
			state.deleted = item.delete
			if !item.delete {
				state.data = item.data
				state.info = item.info
				state.catalog = item.catalog
			}
		}
		result.Applied = append(result.Applied, item.role)
		if !item.noop {
			if err := s.dir.Sync(); err != nil {
				return fail(item.role, err, slices.Clone(roles[i+1:]))
			}
		}
	}
	return result, nil
}

func (s *Session) verifyBaseline(role string, state *roleState) error {
	if err := s.sameFile(role+".ini", state.info); err != nil {
		return err
	}
	current, err := s.readRole(role)
	if err != nil {
		return err
	}
	if !bytes.Equal(current.data, state.data) {
		return fmt.Errorf("role changed on disk; reload required")
	}
	info, err := state.lock.Stat()
	if err != nil {
		return err
	}
	return s.sameFile(role+".ini.lock", info)
}

func (s *Session) prepare(ctx context.Context, role string, changes []Change) (replacement, error) {
	item := replacement{role: role}
	if err := ctx.Err(); err != nil {
		return item, err
	}
	state, ok := s.roles[role]
	if !ok || state.deleted {
		return item, fmt.Errorf("unknown role %q", role)
	}
	if state.lock == nil {
		return item, fmt.Errorf("role is not locked")
	}
	if state.drift {
		return item, fmt.Errorf("role has unresolved drift; reload required")
	}
	if err := s.verifyBaseline(role, state); err != nil {
		return item, err
	}
	doc, err := parseDocument(state.data)
	if err != nil {
		return item, err
	}
	if err := validateChanges(doc, changes); err != nil {
		return item, err
	}
	if changes[0].Kind == DeleteRole {
		item.delete = true
		return item, nil
	}
	item.data, err = doc.apply(changes)
	if err != nil {
		return item, err
	}
	if bytes.Equal(item.data, state.data) {
		item.noop = true
		return item, nil
	}
	item.catalog, err = parseRole(role, item.data)
	if err != nil {
		return item, err
	}
	parsed, err := parseDocument(item.data)
	if err != nil {
		return item, err
	}
	for _, change := range changes {
		if change.Kind != SetFact {
			continue
		}
		section := parsed.findSection(change.Instance)
		if section == nil {
			return item, fmt.Errorf("instance lost during encoding")
		}
		key := section.findKey(change.Key)
		if key == nil || key.value != change.Value {
			return item, fmt.Errorf("value for %s/%s cannot be represented without loss", change.Instance, change.Key)
		}
	}
	temporary := "." + role + "-" + rand.Text() + ".tmp"
	file, err := s.openRegular(temporary, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY, 0600)
	if err != nil {
		return item, err
	}
	success := false
	defer func() {
		_ = file.Close()
		if !success {
			_ = unix.Unlinkat(int(s.dir.Fd()), temporary, 0)
		}
	}()
	// Preserve metadata captured from disk during preflight, not startup. Chown
	// precedes chmod because ownership changes can clear special mode bits.
	current, err := s.readRole(role)
	if err != nil {
		return item, err
	}
	owner := current.info.Sys().(*syscall.Stat_t)
	if err := file.Chown(int(owner.Uid), int(owner.Gid)); err != nil {
		return item, fmt.Errorf("preserve facts ownership: %w", err)
	}
	if err := file.Chmod(current.info.Mode()); err != nil {
		return item, err
	}
	if _, err := file.Write(item.data); err != nil {
		return item, err
	}
	if err := file.Sync(); err != nil {
		return item, err
	}
	item.info, err = file.Stat()
	if err != nil {
		return item, err
	}
	if err := file.Close(); err != nil {
		return item, err
	}
	item.temporary = temporary
	success = true
	return item, nil
}

func validateChanges(doc *document, changes []Change) error {
	for i, change := range changes {
		switch change.Kind {
		case DeleteRole:
			if change.Instance != "" || change.Key != "" || change.Value != "" {
				return fmt.Errorf("role deletion cannot specify descendants or a value")
			}
		case SetFact, DeleteFact, DeleteInstance:
			if change.Instance == "DEFAULT" || change.Instance == "" || doc.findSection(change.Instance) == nil {
				return fmt.Errorf("unknown or reserved instance %q", change.Instance)
			}
			if change.Kind == DeleteInstance {
				if change.Key != "" || change.Value != "" {
					return fmt.Errorf("instance deletion cannot specify a key or value")
				}
				break
			}
			if err := validateKey(change.Key); err != nil {
				return err
			}
			if change.Kind == DeleteFact {
				if change.Value != "" {
					return fmt.Errorf("fact deletion cannot specify a value")
				}
				if doc.findSection(change.Instance).findKey(change.Key) == nil {
					return fmt.Errorf("unknown key %q", change.Key)
				}
			} else if strings.ContainsAny(change.Value, "\r\x00") {
				return fmt.Errorf("fact values must not contain carriage returns or NUL")
			}
		default:
			return fmt.Errorf("unknown change kind %d", change.Kind)
		}
		for _, prior := range changes[:i] {
			if change.Kind == DeleteRole || prior.Kind == DeleteRole {
				return fmt.Errorf("role deletion conflicts with other changes")
			}
			if change.Instance != prior.Instance {
				continue
			}
			if change.Kind == DeleteInstance || prior.Kind == DeleteInstance || change.Key == prior.Key {
				return fmt.Errorf("conflicting changes in instance %q", change.Instance)
			}
		}
	}
	return nil
}

func validateKey(key string) error {
	if key == "" || strings.TrimSpace(key) != key || strings.IndexFunc(key, unicode.IsControl) >= 0 || strings.ContainsAny(key, "=[]") || strings.HasPrefix(key, "#") || strings.HasPrefix(key, ";") {
		return fmt.Errorf("key name %q contains unsupported characters", key)
	}
	return nil
}
