package etl

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// ReadOverrides decodes the [[override]] array of an editorial-overrides
// TOML file into the per-country schema T. A missing file returns
// (nil, nil), since every plan has auto-generated defaults.
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
