package etl

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// WriteFile creates path, runs write on it and closes it. The Close
// error is returned too, since a failed Close can mean the data never
// reached disk.
func WriteFile(path, errPrefix string, write func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("%s: create %s: %w", errPrefix, path, err)
	}
	werr := write(f)
	return errors.Join(werr, f.Close())
}
