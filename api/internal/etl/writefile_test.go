package etl

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// closeEarly closes the underlying file so WriteFile's own Close fails
// with os.ErrClosed: the one Close failure a test can provoke portably.
func closeEarly(t *testing.T, w io.Writer) {
	t.Helper()
	c, ok := w.(io.Closer)
	if !ok {
		t.Fatalf("WriteFile handed write a %T, want an io.Closer", w)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("early close: %v", err)
	}
}

func TestWriteFile_WritesContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	err := WriteFile(path, "etl test", func(w io.Writer) error {
		_, err := io.WriteString(w, "hello\n")
		return err
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hello\n" {
		t.Errorf("content = %q, want %q", got, "hello\n")
	}
}

func TestWriteFile_ReturnsCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	err := WriteFile(path, "etl test", func(w io.Writer) error {
		closeEarly(t, w)
		return nil
	})
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("WriteFile err = %v, want the Close error (os.ErrClosed)", err)
	}
}

func TestWriteFile_JoinsWriteAndCloseErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	errWrite := errors.New("write failed")
	err := WriteFile(path, "etl test", func(w io.Writer) error {
		closeEarly(t, w)
		return errWrite
	})
	if !errors.Is(err, errWrite) {
		t.Errorf("WriteFile err = %v, want it to wrap the write error", err)
	}
	if !errors.Is(err, os.ErrClosed) {
		t.Errorf("WriteFile err = %v, want it to wrap the Close error", err)
	}
}

func TestWriteFile_CreateErrorIsPrefixed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "out.txt")
	err := WriteFile(path, "etl test", func(io.Writer) error {
		t.Fatal("write called after a failed create")
		return nil
	})
	if err == nil {
		t.Fatal("WriteFile into a missing directory: want error, got nil")
	}
	if want := "etl test: create " + path + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("err = %q, want prefix %q", err, want)
	}
}
