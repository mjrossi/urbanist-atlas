package ca

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// fsaRow is one entry from the StatsCan FSA boundary file's attribute
// table. We keep only the fields we need for the smallest-anchor
// crosswalk.
type fsaRow struct {
	// CFSAUID is the 3-character Forward Sortation Area code (e.g.,
	// "M5V"). The first letter encodes the province; the digit and
	// trailing letter further subdivide.
	CFSAUID string
	// PRUID is the 2-digit Statistics Canada province/territory code
	// (e.g., "35" for Ontario). Maps to a province slug via
	// provinceUIDToSlug in mappings.go.
	PRUID string
}

// parseFSAs reads the StatsCan FSA boundary file zip (downloaded from
// https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lfsa000b21a_e.zip)
// and returns one fsaRow per FSA. The function parses only the DBF
// attribute table inside the zip; the much-larger shapefile geometry
// is ignored.
func parseFSAs(zipPath string) ([]fsaRow, error) {
	dbf, closer, err := openDBFFromZip(zipPath, ".dbf")
	if err != nil {
		return nil, fmt.Errorf("parse fsas: %w", err)
	}
	defer closer()

	rows := make([]fsaRow, 0, 1700)
	for {
		row, err := dbf.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse fsas: %w", err)
		}
		fsa := row["CFSAUID"]
		pruid := row["PRUID"]
		if fsa == "" || pruid == "" {
			continue
		}
		rows = append(rows, fsaRow{CFSAUID: fsa, PRUID: pruid})
	}
	return rows, nil
}

// openDBFFromZip locates the first .dbf entry in zipPath, reads it
// into memory (~150KB; trivial), and returns a dbfReader. The
// returned closer must be called by the caller.
func openDBFFromZip(zipPath, ext string) (*dbfReader, func(), error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open zip %s: %w", zipPath, err)
	}
	for _, f := range zr.File {
		if len(f.Name) < len(ext) || f.Name[len(f.Name)-len(ext):] != ext {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			_ = zr.Close()
			return nil, nil, fmt.Errorf("open dbf %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			_ = zr.Close()
			return nil, nil, fmt.Errorf("read dbf %s: %w", f.Name, err)
		}
		dbf, err := newDBFReader(bytes.NewReader(data))
		if err != nil {
			_ = zr.Close()
			return nil, nil, err
		}
		closer := func() { _ = zr.Close() }
		return dbf, closer, nil
	}
	_ = zr.Close()
	return nil, nil, fmt.Errorf("no %s entry in %s", ext, zipPath)
}
