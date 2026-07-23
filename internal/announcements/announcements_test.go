package announcements

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadMigrationApprovalRetriesInvalidInput(t *testing.T) {
	var output bytes.Buffer
	approved, err := readMigrationApproval(strings.NewReader("maybe\nyes\n"), &output)
	if err != nil {
		t.Fatalf("read approval: %v", err)
	}
	if !approved {
		t.Fatal("expected migration approval")
	}
	if !strings.Contains(output.String(), "Invalid input") {
		t.Fatalf("missing invalid-input feedback: %q", output.String())
	}
}

func TestReadMigrationApprovalReturnsEOF(t *testing.T) {
	_, err := readMigrationApproval(strings.NewReader(""), io.Discard)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestReadMigrationApprovalReturnsScannerError(t *testing.T) {
	wantErr := errors.New("input failed")
	_, err := readMigrationApproval(errorReader{err: wantErr}, io.Discard)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected scanner error, got %v", err)
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
