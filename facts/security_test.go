package facts

import (
	"golang.org/x/sys/unix"
	"testing"
)

func makeFIFO(t *testing.T, name string) {
	t.Helper()
	if err := unix.Mkfifo(name, 0600); err != nil {
		t.Fatal(err)
	}
}
