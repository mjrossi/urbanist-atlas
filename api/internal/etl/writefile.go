package etl

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// WriteFile creates path, hands the open file to write, and closes it.
// Only the create error is wrapped (as "<errPrefix>: create <path>:
// <err>"); write and Close errors are returned joined, unwrapped,
// because a failed Close can mean the data never reached disk.
func WriteFile(path, errPrefix string, write func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("%s: create %s: %w", errPrefix, path, err)
	}
	werr := write(f)
	return errors.Join(werr, f.Close())
}
