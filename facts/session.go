// Package facts owns existing Saltbox fact files and their shared writer locks.
package facts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var ErrEditorActive = errors.New("another fact editor is active")
var ErrClosed = errors.New("fact session is closed")

const roleLockTimeout = 30 * time.Second

type Fact struct{ Key, Value string }
type Instance struct {
	Name  string
	Facts []Fact
}
type Role struct {
	Name      string
	Instances []Instance
}
type Catalog struct{ Roles []Role }

type roleState struct {
	data    []byte
	info    os.FileInfo
	catalog Role
	lock    *os.File
	drift   bool
	applied bool
	deleted bool
}

// Session owns the editor lock and retained role locks. Methods are serialized;
// callers must cancel an in-flight LockRole before closing the session.
type Session struct {
	mu     sync.Mutex
	root   string
	dir    *os.File
	editor *os.File
	roles  map[string]*roleState
	rename func(old, new string) error
}

// OpenSession catalogs existing roles without taking any role locks.
func OpenSession(root string) (_ *Session, err error) {
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	dir, err := openDirectory(root)
	if err != nil {
		return nil, err
	}
	s := &Session{root: root, dir: dir, roles: make(map[string]*roleState)}
	s.rename = func(old, new string) error { return unix.Renameat(int(dir.Fd()), old, int(dir.Fd()), new) }
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	s.editor, err = s.openLock(".fact-editor.lock")
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(s.editor.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrEditorActive
		}
		return nil, fmt.Errorf("acquire editor lock: %w", err)
	}
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("catalog facts directory: %w", err)
	}
	for _, name := range names {
		if !strings.HasSuffix(name, ".ini") {
			continue
		}
		role := strings.TrimSuffix(name, ".ini")
		if role == "" {
			return nil, fmt.Errorf("empty role name: %s", name)
		}
		state, readErr := s.readRole(role)
		if readErr != nil {
			return nil, fmt.Errorf("catalog %s: %w", role, readErr)
		}
		s.roles[role] = state
	}
	return s, nil
}

// Catalog returns an independent, sorted snapshot. DEFAULT is retained on disk
// but deliberately hidden from the editable catalog.
func (s *Session) Catalog() Catalog {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Catalog{}
	for _, state := range s.roles {
		if state.deleted {
			continue
		}
		result.Roles = append(result.Roles, cloneRole(state.catalog))
	}
	slices.SortFunc(result.Roles, func(a, b Role) int { return strings.Compare(a.Name, b.Name) })
	return result
}

func cloneRole(role Role) Role {
	role.Instances = slices.Clone(role.Instances)
	for i := range role.Instances {
		role.Instances[i].Facts = slices.Clone(role.Instances[i].Facts)
	}
	return role
}

func parseRole(name string, data []byte) (Role, error) {
	doc, err := parseDocument(data)
	if err != nil {
		return Role{}, err
	}
	return doc.catalog(name), nil
}

func (s *Session) readRole(role string) (*roleState, error) {
	name := role + ".ini"
	file, err := s.openRegular(name, unix.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if err = s.sameFile(name, info); err != nil {
		return nil, err
	}
	catalog, err := parseRole(role, data)
	if err != nil {
		return nil, err
	}
	return &roleState{data: data, info: info, catalog: catalog}, nil
}

// Close releases every lock without writing any pending changes. It is idempotent.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	for _, state := range s.roles {
		if state.lock != nil {
			err = errors.Join(err, state.lock.Close())
			state.lock = nil
		}
	}
	if s.editor != nil {
		err = errors.Join(err, s.editor.Close())
		s.editor = nil
	}
	if s.dir != nil {
		err = errors.Join(err, s.dir.Close())
		s.dir = nil
	}
	return err
}

// openDirectory rejects symlinks in every path component and pins the directory.
func openDirectory(path string) (*os.File, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for part := range strings.SplitSeq(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, fmt.Errorf("facts directory must be an existing directory without symlinks: %w", openErr)
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), path), nil
}

func (s *Session) openRegular(name string, flags int, mode uint32) (*os.File, error) {
	var before unix.Stat_t
	err := unix.Fstatat(int(s.dir.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW)
	if err == nil {
		if before.Mode&unix.S_IFMT == unix.S_IFLNK {
			return nil, fmt.Errorf("facts file %s must not be a symlink", name)
		}
		if before.Mode&unix.S_IFMT != unix.S_IFREG {
			return nil, fmt.Errorf("facts file %s must be a regular file", name)
		}
	} else if !errors.Is(err, unix.ENOENT) || flags&unix.O_CREAT == 0 {
		return nil, err
	}
	fd, err := unix.Openat(int(s.dir.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, mode)
	if err != nil {
		return nil, fmt.Errorf("open facts file %s: %w", name, err)
	}
	file := os.NewFile(uintptr(fd), name)
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("facts file %s must be a regular file", name)
	}
	if err == nil {
		err = s.sameFile(name, info)
	}
	if err == nil && before.Ino != 0 {
		var opened unix.Stat_t
		err = unix.Fstat(fd, &opened)
		if err == nil && (before.Ino != opened.Ino || before.Dev != opened.Dev) {
			err = fmt.Errorf("facts file %s changed while opening", name)
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (s *Session) sameFile(name string, info os.FileInfo) error {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(s.dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	original := info.Sys().(*syscall.Stat_t)
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Ino != original.Ino || stat.Dev != original.Dev {
		return fmt.Errorf("facts file %s changed during session", name)
	}
	return nil
}

func (s *Session) openLock(name string) (*os.File, error) {
	return s.openRegular(name, unix.O_CREAT|unix.O_RDWR, 0640)
}

func waitLock(ctx context.Context, file *os.File) error {
	ctx, cancel := context.WithTimeout(ctx, roleLockTimeout)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Drift is the semantic before/after view of externally changed role bytes.
// An empty After role means the role was deleted.
type Drift struct{ Before, After Role }

// LockRole retains the shared sidecar lock. Drift requires explicit ReloadRole
// or ReleaseRole before mutations can be applied.
func (s *Session) LockRole(ctx context.Context, role string) (*Drift, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return nil, ErrClosed
	}
	state, ok := s.roles[role]
	if !ok || state.deleted {
		return nil, fmt.Errorf("unknown role %q", role)
	}
	newlyLocked := state.lock == nil
	if newlyLocked {
		lock, err := s.openLock(role + ".ini.lock")
		if err != nil {
			return nil, err
		}
		if err = waitLock(ctx, lock); err != nil {
			_ = lock.Close()
			return nil, fmt.Errorf("lock role %s: %w", role, err)
		}
		info, err := lock.Stat()
		if err == nil {
			err = s.sameFile(role+".ini.lock", info)
		}
		if err != nil {
			_ = lock.Close()
			return nil, err
		}
		state.lock = lock
	}
	current, err := s.readRole(role)
	if errors.Is(err, os.ErrNotExist) {
		state.drift = true
		return &Drift{Before: cloneRole(state.catalog), After: Role{Name: role}}, nil
	}
	if err != nil {
		if newlyLocked {
			_ = state.lock.Close()
			state.lock = nil
		}
		return nil, err
	}
	if !bytes.Equal(state.data, current.data) {
		state.drift = true
		return &Drift{Before: cloneRole(state.catalog), After: cloneRole(current.catalog)}, nil
	}
	if newlyLocked {
		state.info = current.info
	}
	return nil, nil
}

// ReloadRole adopts disk state as the baseline/catalog and retains its lock.
// The caller cancels the attempted edit and requires a manual retry.
func (s *Session) ReloadRole(role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return ErrClosed
	}
	state, ok := s.roles[role]
	if !ok || state.lock == nil {
		return fmt.Errorf("role %q is not locked", role)
	}
	current, err := s.readRole(role)
	if errors.Is(err, os.ErrNotExist) {
		state.deleted = true
		state.drift = false
		return nil
	}
	if err != nil {
		return err
	}
	current.lock = state.lock
	current.applied = state.applied
	s.roles[role] = current
	return nil
}

// ReleaseRole cancels a newly acquired lock. Callers must discard that role's
// staged changes first. Locks for successfully applied roles last until Close.
func (s *Session) ReleaseRole(role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		return ErrClosed
	}
	state, ok := s.roles[role]
	if !ok {
		return fmt.Errorf("unknown role %q", role)
	}
	if state.applied {
		return fmt.Errorf("role %q has applied changes; lock retained until close", role)
	}
	if state.lock == nil {
		return nil
	}
	err := state.lock.Close()
	state.lock = nil
	state.drift = false
	return err
}
