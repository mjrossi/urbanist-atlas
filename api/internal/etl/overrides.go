package etl

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// ReadOverrides parses an optional editorial-overrides TOML file whose
// top-level table is an array of [[override]] blocks, returning the
// decoded slice. A missing file is not an error — overrides are
// optional and every country plan auto-generates a sensible default
// for every region — so it returns (nil, nil) in that case.
//
// T is the per-country override schema (us.MSAOverride, ca.CMAOverride,
// …); the `toml:"override"` array name is shared across countries, so
// only the element type varies.
func ReadOverrides[T any](path string) ([]T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	var f struct {
		Overrides []T `toml:"override"`
	}
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse overrides: %w", err)
	}
	return f.Overrides, nil
}
