package etl

import (
	"fmt"
	"io"
	"os"
)

// WriteFile creates path and hands the open file to write, closing it
// when write returns. It factors out the os.Create → wrap error →
// defer Close → delegate shape shared by every per-country seed
// writer. Only the create error is wrapped (as
// "<errPrefix>: create <path>: <err>", preserving the country plans'
// "etl us:" / "etl ca:" prefixes); errors from write pass through
// unwrapped, exactly like the previously inlined wrappers.
func WriteFile(path, errPrefix string, write func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("%s: create %s: %w", errPrefix, path, err)
	}
	defer f.Close()
	return write(f)
}
