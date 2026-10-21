package ca

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// fsaRow is one entry from the FSA boundary file's DBF table.
type fsaRow struct {
	// CFSAUID is the 3-character Forward Sortation Area code (e.g. "M5V").
	CFSAUID string
	// PRUID is the 2-digit StatsCan province/territory code (e.g. "35").
	PRUID string
}

// parseFSAs reads the FSA boundary zip's DBF table into one fsaRow per
// FSA.
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

// openDBFFromZip reads the first entry ending in ext into memory and
// returns a reader over it. The caller must call the returned closer.
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
