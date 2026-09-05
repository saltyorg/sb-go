package facts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, root, role, data string) string {
	t.Helper()
	name := filepath.Join(root, role+".ini")
	if err := os.WriteFile(name, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestCatalogPreservesLiteralAndEmptyInstances(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "zeta", "[last]\nk = v\n")
	fixture(t, root, "alpha", "; retained\nsecret = default-value\n[zulu]\nz = None\na =\n[empty]\n")
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	catalog := s.Catalog()
	if len(catalog.Roles) != 2 || catalog.Roles[0].Name != "alpha" || catalog.Roles[1].Name != "zeta" {
		t.Fatalf("catalog = %#v", catalog)
	}
	instances := catalog.Roles[0].Instances
	if len(instances) != 2 || instances[0].Name != "empty" || len(instances[0].Facts) != 0 || instances[1].Name != "zulu" {
		t.Fatalf("instances = %#v", instances)
	}
	got := instances[1].Facts
	if len(got) != 2 || got[0] != (Fact{Key: "a", Value: ""}) || got[1] != (Fact{Key: "z", Value: "None"}) {
		t.Fatalf("facts = %#v", got)
	}
	catalog.Roles[0].Instances[1].Facts[0].Value = "mutated"
	if s.Catalog().Roles[0].Instances[1].Facts[0].Value != "" {
		t.Fatal("caller mutated session catalog")
	}
}

func TestOpenSessionRejectsUnsafeInputs(t *testing.T) {
	for _, kind := range []string{"missing root", "symlink root", "symlink ancestor", "file root", "symlink role", "directory role", "fifo role", "malformed role", "symlink editor lock", "directory editor lock"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "facts")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing root":
				root = filepath.Join(base, "missing")
			case "symlink root":
				root = filepath.Join(base, "link")
				if err := os.Symlink(filepath.Join(base, "facts"), root); err != nil {
					t.Fatal(err)
				}
			case "symlink ancestor":
				if err := os.Symlink(base, filepath.Join(base, "link")); err != nil {
					t.Fatal(err)
				}
				root = filepath.Join(base, "link", "facts")
			case "file root":
				root = fixture(t, base, "not-dir", "x")
			case "symlink role":
				target := fixture(t, base, "target", "[x]\nk=v\n")
				if err := os.Symlink(target, filepath.Join(root, "role.ini")); err != nil {
					t.Fatal(err)
				}
			case "directory role":
				if err := os.Mkdir(filepath.Join(root, "role.ini"), 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo role":
				makeFIFO(t, filepath.Join(root, "role.ini"))
			case "malformed role":
				fixture(t, root, "role", "[broken\nk=v\n")
			case "symlink editor lock":
				target := fixture(t, base, "target", "unchanged")
				if err := os.Symlink(target, filepath.Join(root, ".fact-editor.lock")); err != nil {
					t.Fatal(err)
				}
			case "directory editor lock":
				if err := os.Mkdir(filepath.Join(root, ".fact-editor.lock"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			s, err := OpenSession(root)
			if err == nil {
				_ = s.Close()
				t.Fatal("unsafe input accepted")
			}
		})
	}
}

func TestEditorLockSingletonAndFailedStartupCleanup(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "role", "[x]\nk=v\n")
	s, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := OpenSession(root); !errors.Is(err, ErrEditorActive) {
		if other != nil {
			_ = other.Close()
		}
		t.Fatalf("second editor = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	fixture(t, root, "role", "[broken")
	if other, err := OpenSession(root); err == nil {
		_ = other.Close()
		t.Fatal("malformed role accepted")
	}
	fixture(t, root, "role", "[x]\nk=v\n")
	other, err := OpenSession(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = other.Close()
}
