package facts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sys/unix"
)

func openFixture(t *testing.T) (*Session, string) {
	t.Helper()
	root := t.TempDir()
	fixture(t, root, "role", "; header\nhidden = secret\n[default]\n; token comment\ntoken = original\nempty =\nnone = None\n[empty]\n[other]\nk = untouched\n")
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, root
}
func lockRole(t *testing.T, s *Session, role string) {
	t.Helper()
	drift, err := s.LockRole(t.Context(), role)
	if err != nil || drift != nil {
		t.Fatalf("lock = %#v, %v", drift, err)
	}
}
func externalLock(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestRoleLockCancellationRetentionAndRelease(t *testing.T) {
	s, root := openFixture(t)
	path := filepath.Join(root, "role.ini.lock")
	held := externalLock(t, path)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	if _, err := s.LockRole(ctx, "role"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock cancellation = %v", err)
	}
	if err := unix.Flock(int(held.Fd()), unix.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	lockRole(t, s, "role")
	lockRole(t, s, "role")
	if err := unix.Flock(int(held.Fd()), unix.LOCK_EX|unix.LOCK_NB); !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("retained lock = %v", err)
	}
	if err := s.ReleaseRole("role"); err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(held.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
}

func TestRoleLockTimeoutAndSidecarReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, root := openFixture(t)
		held := externalLock(t, filepath.Join(root, "role.ini.lock"))
		start := time.Now()
		if _, err := s.LockRole(t.Context(), "role"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout = %v", err)
		}
		if elapsed := time.Since(start); elapsed != 30*time.Second {
			t.Fatalf("waited %s", elapsed)
		}
		done := make(chan error, 1)
		go func() { _, err := s.LockRole(t.Context(), "role"); done <- err }()
		synctest.Wait()
		if err := os.Rename(filepath.Join(root, "role.ini.lock"), filepath.Join(root, "old.lock")); err != nil {
			t.Fatal(err)
		}
		replacement := externalLock(t, filepath.Join(root, "role.ini.lock"))
		defer func() { _ = replacement.Close() }()
		if err := unix.Flock(int(held.Fd()), unix.LOCK_UN); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err == nil {
			t.Fatal("accepted a replaced sidecar lock")
		}
	})
}

func TestApplyMultilineRoundtripRejectsLostCommentLines(t *testing.T) {
	s, _ := openFixture(t)
	lockRole(t, s, "role")
	change := Change{Kind: SetFact, Role: "role", Instance: "default", Key: "token", Value: "first\nsecond"}
	if _, err := s.Apply(t.Context(), []Change{change}); err != nil {
		t.Fatal(err)
	}
	change.Value = "first\n# lost"
	if _, err := s.Apply(t.Context(), []Change{change}); err == nil {
		t.Fatal("multiline comment silently discarded")
	}
}

func TestLockRoleAcceptsIdenticalAtomicReplacementBeforeAcquisition(t *testing.T) {
	s, root := openFixture(t)
	name := filepath.Join(root, "role.ini")
	data, _ := os.ReadFile(name)
	if err := os.Rename(name, name+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	lockRole(t, s, "role")
	if _, err := s.Apply(t.Context(), []Change{{Kind: SetFact, Role: "role", Instance: "default", Key: "token", Value: "updated"}}); err != nil {
		t.Fatal(err)
	}
}

func TestRoleDriftRequiresReloadAndManualRetry(t *testing.T) {
	s, root := openFixture(t)
	fixture(t, root, "role", "[default]\ntoken = external\n")
	drift, err := s.LockRole(t.Context(), "role")
	if err != nil || drift == nil {
		t.Fatalf("drift = %#v, %v", drift, err)
	}
	if drift.After.Instances[0].Facts[0].Value != "external" {
		t.Fatalf("after = %#v", drift.After)
	}
	changes := []Change{{Kind: SetFact, Role: "role", Instance: "default", Key: "token", Value: "edited"}}
	if _, err := s.Apply(t.Context(), changes); err == nil {
		t.Fatal("applied without accepting drift")
	}
	if err := s.ReloadRole("role"); err != nil {
		t.Fatal(err)
	}
	if s.Catalog().Roles[0].Instances[0].Facts[0].Value != "external" {
		t.Fatal("reload did not adopt disk")
	}
	if result, err := s.Apply(t.Context(), changes); err != nil || len(result.Applied) != 1 {
		t.Fatalf("retry = %#v, %v", result, err)
	}
	if err := s.ReleaseRole("role"); err == nil {
		t.Fatal("released applied lock")
	}
}

func TestApplyTypedChangesPreservesMetadataAndUnrelatedData(t *testing.T) {
	s, root := openFixture(t)
	name := filepath.Join(root, "role.ini")
	if err := os.Chmod(name, 0670); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	lockRole(t, s, "role")
	result, err := s.Apply(t.Context(), []Change{
		{Kind: SetFact, Role: "role", Instance: "default", Key: "token", Value: "changed"},
		{Kind: SetFact, Role: "role", Instance: "empty", Key: "added", Value: "new"},
		{Kind: DeleteFact, Role: "role", Instance: "default", Key: "empty"},
		{Kind: DeleteInstance, Role: "role", Instance: "other"},
	})
	if err != nil || len(result.Applied) != 1 {
		t.Fatalf("apply = %#v, %v", result, err)
	}
	after, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode() != after.Mode() || os.SameFile(before, after) {
		t.Fatalf("replacement metadata/inode = %v, %v", before, after)
	}
	a, b := before.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
	if a.Uid != b.Uid || a.Gid != b.Gid {
		t.Fatal("ownership changed")
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"; header", "hidden", "secret", "; token comment", "None", "changed", "added"} {
		if !strings.Contains(string(data), text) {
			t.Errorf("lost %q in %s", text, data)
		}
	}
	catalog := s.Catalog().Roles[0]
	if len(catalog.Instances) != 2 || catalog.Instances[0].Name != "default" || catalog.Instances[1].Facts[0] != (Fact{Key: "added", Value: "new"}) {
		t.Fatalf("catalog = %#v", catalog)
	}
	result, err = s.Apply(t.Context(), []Change{{Kind: DeleteRole, Role: "role"}})
	if err != nil || len(result.Applied) != 1 {
		t.Fatalf("delete role = %#v,%v", result, err)
	}
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("role remains = %v", err)
	}
	if len(s.Catalog().Roles) != 0 {
		t.Fatal("deleted role remains in catalog")
	}
}

func TestApplyKeepsEmptySectionsAndNoopBytes(t *testing.T) {
	s, root := openFixture(t)
	lockRole(t, s, "role")
	name := filepath.Join(root, "role.ini")
	before, _ := os.ReadFile(name)
	_, err := s.Apply(t.Context(), []Change{{Kind: SetFact, Role: "role", Instance: "default", Key: "token", Value: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(name)
	if string(before) != string(after) {
		t.Fatal("no-op rewrote bytes")
	}
	_, err = s.Apply(t.Context(), []Change{{Kind: DeleteFact, Role: "role", Instance: "other", Key: "k"}})
	if err != nil {
		t.Fatal(err)
	}
	catalog := s.Catalog()
	if len(catalog.Roles[0].Instances) != 3 || len(catalog.Roles[0].Instances[2].Facts) != 0 {
		t.Fatalf("empty section lost: %#v", catalog)
	}
}

func TestApplyAdjacentInsertionAndDeletionIsIndependentOfChangeOrder(t *testing.T) {
	s, _ := openFixture(t)
	lockRole(t, s, "role")
	_, err := s.Apply(t.Context(), []Change{{Kind: DeleteInstance, Role: "role", Instance: "other"}, {Kind: SetFact, Role: "role", Instance: "empty", Key: "added", Value: "new"}})
	if err != nil {
		t.Fatal(err)
	}
	role := s.Catalog().Roles[0]
	if len(role.Instances) != 2 || role.Instances[1].Facts[0].Value != "new" {
		t.Fatalf("catalog = %#v", role)
	}
}

func TestEditingMultilineFactPreservesInterveningComment(t *testing.T) {
	root := t.TempDir()
	name := fixture(t, root, "role", "[default]\nk = first\n# preserve me\n\tsecond\n")
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	lockRole(t, s, "role")
	if _, err := s.Apply(t.Context(), []Change{{Kind: SetFact, Role: "role", Instance: "default", Key: "k", Value: "new"}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(name)
	if !strings.Contains(string(data), "# preserve me\n") {
		t.Fatalf("lost comment: %s", data)
	}
}

func TestApplyRejectsInvalidAndConflictingChanges(t *testing.T) {
	cases := [][]Change{
		{{Kind: SetFact, Role: "missing", Instance: "default", Key: "k", Value: "v"}},
		{{Kind: SetFact, Role: "role", Instance: "missing", Key: "k", Value: "v"}},
		{{Kind: SetFact, Role: "role", Instance: "DEFAULT", Key: "k", Value: "v"}},
		{{Kind: SetFact, Role: "role", Instance: "default", Key: ";hidden", Value: "v"}},
		{{Kind: SetFact, Role: "role", Instance: "default", Key: "[injected]", Value: "v"}},
		{{Kind: SetFact, Role: "role", Instance: "default", Key: "k\ninjected", Value: "v"}},
		{{Kind: SetFact, Role: "role", Instance: "default", Key: "k", Value: "v\x00injected"}},
		{{Kind: DeleteRole, Role: "role"}, {Kind: SetFact, Role: "role", Instance: "default", Key: "k", Value: "v"}},
		{{Kind: DeleteInstance, Role: "role", Instance: "default"}, {Kind: SetFact, Role: "role", Instance: "default", Key: "k", Value: "v"}},
		{{Kind: SetFact, Role: "role", Instance: "default", Key: "k", Value: "v"}, {Kind: SetFact, Role: "role", Instance: "default", Key: "k", Value: "other"}},
		{{Kind: DeleteFact, Role: "role", Instance: "default", Key: "missing"}},
		{{Kind: ChangeKind(99), Role: "role"}},
	}
	for i, changes := range cases {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			s, root := openFixture(t)
			lockRole(t, s, "role")
			name := filepath.Join(root, "role.ini")
			before, _ := os.ReadFile(name)
			if _, err := s.Apply(t.Context(), changes); err == nil {
				t.Fatal("invalid change accepted")
			}
			after, _ := os.ReadFile(name)
			if string(before) != string(after) {
				t.Fatal("invalid apply changed disk")
			}
		})
	}
}

func TestApplyBracketContainingKeys(t *testing.T) {
	for _, tt := range []struct {
		name, original, want string
		kind                 ChangeKind
	}{
		{name: "update existing", original: "[default]\nheaders[Authorization] = old\n", want: "[default]\nheaders[Authorization] = new\n", kind: SetFact},
		{name: "delete existing", original: "[default]\nheaders[Authorization] = old\n", want: "[default]\n", kind: DeleteFact},
		{name: "add new", original: "[default]\n", want: "[default]\n\nheaders[Authorization] = new\n", kind: SetFact},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := fixture(t, root, "role", tt.original)
			s, err := OpenSession(root)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			lockRole(t, s, "role")
			change := Change{Kind: tt.kind, Role: "role", Instance: "default", Key: "headers[Authorization]"}
			if tt.kind == SetFact {
				change.Value = "new"
			}
			if _, err := s.Apply(t.Context(), []Change{change}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.want {
				t.Fatalf("facts = %q, want %q", data, tt.want)
			}
		})
	}
}

func TestApplyPreservesPythonLiteralValuesAndUntouchedBytes(t *testing.T) {
	root := t.TempDir()
	const original = "# header\n[DEFAULT]\nhidden = `literal`\n[default]\nquoted = \"quoted\"\nbacktick = `literal`\ntriple = \"\"\"literal\"\"\"\nslash = end\\\ncolon:key = literal # ; value\nmultiline = first\n\tsecond\nchange=old\n\n[empty]\n"
	name := fixture(t, root, "role", original)
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	got := s.Catalog().Roles[0].Instances[0].Facts
	want := map[string]string{"quoted": "\"quoted\"", "backtick": "`literal`", "triple": "\"\"\"literal\"\"\"", "slash": "end\\", "colon:key": "literal # ; value", "multiline": "first\nsecond", "change": "old"}
	for _, fact := range got {
		if fact.Value != want[fact.Key] {
			t.Errorf("%s = %q, want %q", fact.Key, fact.Value, want[fact.Key])
		}
		delete(want, fact.Key)
	}
	if len(want) != 0 {
		t.Fatalf("missing facts: %v", want)
	}
	lockRole(t, s, "role")
	if _, err := s.Apply(t.Context(), []Change{{Kind: SetFact, Role: "role", Instance: "default", Key: "change", Value: "new"}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(name)
	if string(data) != strings.Replace(original, "change=old", "change=new", 1) {
		t.Fatalf("unrelated bytes changed:\n%s", data)
	}
}

func TestApplyMatchesSaltboxPythonReader(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python consumer check requires python3")
	}
	root := t.TempDir()
	name := fixture(t, root, "role", "# preserved\n[DEFAULT]\nhidden = `literal`\n[default]\nquoted = \"quoted\"\nbacktick = `literal`\ncolon:key = literal # ; value\nchange = old\n[empty]\n")
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	lockRole(t, s, "role")
	changes := []Change{{Kind: SetFact, Role: "role", Instance: "default", Key: "change", Value: "first\nsecond"}, {Kind: SetFact, Role: "role", Instance: "empty", Key: "new", Value: "None"}}
	if _, err := s.Apply(t.Context(), changes); err != nil {
		t.Fatal(err)
	}
	script := `import configparser, json, sys
c = configparser.ConfigParser(interpolation=None, comment_prefixes=('#',), inline_comment_prefixes=None, default_section='DEFAULT', delimiters=('=',), empty_lines_in_values=False)
c.optionxform = str
with open(sys.argv[1]) as f: c.read_file(f)
print(json.dumps({s: dict(c[s]) for s in c.sections()}))
`
	output, err := exec.CommandContext(t.Context(), python, "-c", script, name).CombinedOutput()
	if err != nil {
		t.Fatalf("Python consumer: %v: %s", err, output)
	}
	var got map[string]map[string]string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"quoted": "\"quoted\"", "backtick": "`literal`", "colon:key": "literal # ; value", "change": "first\nsecond", "hidden": "`literal`"} {
		if got["default"][key] != want {
			t.Errorf("Python %s = %q, want %q", key, got["default"][key], want)
		}
	}
	if got["empty"]["new"] != "None" {
		t.Fatalf("new value = %q", got["empty"]["new"])
	}
}

func TestCatalogAcceptsDefaultSectionAfterAnInstance(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "role", "[default]\nk=v\n[DEFAULT]\nhidden=secret\n")
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if len(s.Catalog().Roles[0].Instances) != 1 {
		t.Fatal("DEFAULT was exposed")
	}
}

func TestCatalogRejectsDuplicateKeysAndInstances(t *testing.T) {
	for _, data := range []string{"[default]\nk=a\nk=b\n", "[default]\nk=a\n[default]\nj=b\n"} {
		root := t.TempDir()
		fixture(t, root, "role", data)
		if s, err := OpenSession(root); err == nil {
			_ = s.Close()
			t.Fatal("ambiguous INI accepted")
		}
	}
}

func TestApplyPreflightsAllRolesAndReportsPartialCommit(t *testing.T) {
	root := t.TempDir()
	for _, role := range []string{"a", "b", "c"} {
		fixture(t, root, role, "[default]\nk = old\n")
	}
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var changes []Change
	for _, role := range []string{"c", "a", "b"} {
		lockRole(t, s, role)
		changes = append(changes, Change{Kind: SetFact, Role: role, Instance: "default", Key: "k", Value: "new"})
	}
	fixture(t, root, "c", "[default]\nk = external\n")
	result, err := s.Apply(t.Context(), changes)
	if err == nil || len(result.Applied) != 0 {
		t.Fatalf("preflight = %#v,%v", result, err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "a.ini"))
	if !strings.Contains(string(data), "old") {
		t.Fatal("committed before all roles preflighted")
	}
	if err := s.ReloadRole("c"); err != nil {
		t.Fatal(err)
	}
	rename := s.rename
	s.rename = func(old, new string) error {
		if new == "b.ini" {
			return syscall.EIO
		}
		return rename(old, new)
	}
	result, err = s.Apply(t.Context(), changes)
	if err == nil || strings.Join(result.Applied, ",") != "a" || result.Failed == nil || result.Failed.Role != "b" || strings.Join(result.Unattempted, ",") != "c" {
		t.Fatalf("partial = %#v,%v", result, err)
	}
	for _, item := range []struct{ role, value string }{{"a", "new"}, {"b", "old"}, {"c", "external"}} {
		data, _ := os.ReadFile(filepath.Join(root, item.role+".ini"))
		if !strings.Contains(string(data), item.value) {
			t.Fatalf("%s = %s", item.role, data)
		}
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
	s.rename = rename
	if result, err := s.Apply(t.Context(), []Change{changes[0], changes[2]}); err != nil || strings.Join(result.Applied, ",") != "b,c" {
		t.Fatalf("retry = %#v,%v", result, err)
	}
}

func TestApplyRejectsUnheldLockAndChangedInode(t *testing.T) {
	s, root := openFixture(t)
	change := []Change{{Kind: DeleteRole, Role: "role"}}
	if _, err := s.Apply(t.Context(), change); err == nil {
		t.Fatal("apply accepted without role lock")
	}
	lockRole(t, s, "role")
	name := filepath.Join(root, "role.ini")
	data, _ := os.ReadFile(name)
	if err := os.Rename(name, name+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(t.Context(), change); err == nil {
		t.Fatal("deleted a replaced inode")
	}
}
