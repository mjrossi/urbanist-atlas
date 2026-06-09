package ca

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// dbfField describes one column in a dBASE III-style DBF file.
type dbfField struct {
	Name   string
	Type   byte // 'C' character, 'N' numeric, etc. We only consume 'C' below.
	Length int
}

// dbfReader is a minimal stdlib-only DBF reader for the StatsCan
// boundary file attribute tables. The 1980s dBASE III format is small
// enough that pulling in a library would dwarf the parser. Only the
// fields we actually consume ('C'-typed string columns) are returned
// as strings; numeric ('N') columns are returned as the raw string
// representation (the caller parses if needed).
type dbfReader struct {
	src         io.ReadSeeker
	fields      []dbfField
	headerSize  int
	recordSize  int
	recordCount int
	consumed    int
}

func newDBFReader(src io.ReadSeeker) (*dbfReader, error) {
	header := make([]byte, 32)
	if _, err := io.ReadFull(src, header); err != nil {
		return nil, fmt.Errorf("dbf: read header: %w", err)
	}
	recordCount := int(binary.LittleEndian.Uint32(header[4:8]))
	headerSize := int(binary.LittleEndian.Uint16(header[8:10]))
	recordSize := int(binary.LittleEndian.Uint16(header[10:12]))

	var fields []dbfField
	for {
		fd := make([]byte, 32)
		if _, err := io.ReadFull(src, fd); err != nil {
			return nil, fmt.Errorf("dbf: read field descriptor: %w", err)
		}
		if fd[0] == 0x0D {
			break
		}
		name := strings.TrimRight(string(fd[:11]), "\x00")
		fields = append(fields, dbfField{
			Name:   name,
			Type:   fd[11],
			Length: int(fd[16]),
		})
	}
	if _, err := src.Seek(int64(headerSize), io.SeekStart); err != nil {
		return nil, fmt.Errorf("dbf: seek to records: %w", err)
	}
	// Guard recordSize against the field layout before next() trusts it:
	// each record is a 1-byte deletion flag followed by every field's
	// fixed-width bytes, so a recordSize smaller than 1+Σ field.Length
	// means a truncated/corrupt header that would otherwise make next()
	// slice past the record buffer and panic. Fail with context instead.
	minRecordSize := 1
	for _, f := range fields {
		minRecordSize += f.Length
	}
	if recordSize < minRecordSize {
		return nil, fmt.Errorf("dbf: record size %d smaller than field layout requires (1 deletion flag + %d field bytes = %d)", recordSize, minRecordSize-1, minRecordSize)
	}
	return &dbfReader{
		src:         src,
		fields:      fields,
		headerSize:  headerSize,
		recordSize:  recordSize,
		recordCount: recordCount,
	}, nil
}

// next returns the next record as a name→trimmed-value map. Returns
// io.EOF when records are exhausted. Deleted records (first byte ==
// '*') are skipped automatically.
//
// Values are decoded from Latin-1 to UTF-8. The dBASE III file format
// predates Unicode and the StatsCan boundary files use the original
// Latin-1 encoding for accented characters (Montréal's "é" arrives
// as the single byte 0xE9 rather than the UTF-8 two-byte sequence).
// Decoding to UTF-8 here keeps the rest of the pipeline UTF-8
// throughout — TOML output, slug generation, all of it.
func (r *dbfReader) next() (map[string]string, error) {
	for r.consumed < r.recordCount {
		rec := make([]byte, r.recordSize)
		if _, err := io.ReadFull(r.src, rec); err != nil {
			return nil, fmt.Errorf("dbf: read record %d: %w", r.consumed, err)
		}
		r.consumed++
		if rec[0] == '*' {
			continue
		}
		row := make(map[string]string, len(r.fields))
		offset := 1
		for _, f := range r.fields {
			// Character fields are conventionally space-padded, but some
			// encoders NUL-pad (StatsCan's real files use spaces; go-shp's
			// writer, used by the test fixtures, uses NUL) — trim both.
			val := strings.Trim(latin1ToUTF8(rec[offset:offset+f.Length]), " \x00")
			row[f.Name] = val
			offset += f.Length
		}
		return row, nil
	}
	return nil, io.EOF
}

// latin1ToUTF8 reinterprets a Latin-1 byte slice as UTF-8 by promoting
// each byte to its corresponding Unicode codepoint (0x00..0xFF map
// directly to U+0000..U+00FF).
func latin1ToUTF8(b []byte) string {
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}
