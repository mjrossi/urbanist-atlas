package ca

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// dbfFieldDef is one character ('C') column for buildDBF.
type dbfFieldDef struct {
	Name string
	Len  int
}

// buildDBF assembles dBASE III bytes from character-field definitions
// and active records (values in field order). It mirrors the on-disk
// layout newDBFReader parses (see dbf.go) and the hand-built fixture in
// TestDBFReader: a 32-byte file header, one 32-byte descriptor per
// field, a 0x0D terminator, then one space-flagged record each. Values
// longer than their field are truncated; shorter are space-padded.
func buildDBF(t *testing.T, fields []dbfFieldDef, rows [][]string) []byte {
	t.Helper()
	recordLen := 1 // deletion flag
	for _, f := range fields {
		recordLen += f.Len
	}

	var buf bytes.Buffer
	header := make([]byte, 32)
	header[0] = 0x03 // dBASE III
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(rows)))
	binary.LittleEndian.PutUint16(header[8:10], uint16(32+32*len(fields)+1))
	binary.LittleEndian.PutUint16(header[10:12], uint16(recordLen))
	buf.Write(header)

	for _, f := range fields {
		fd := make([]byte, 32)
		copy(fd[0:11], f.Name)
		fd[11] = 'C'
		fd[16] = byte(f.Len)
		buf.Write(fd)
	}
	buf.WriteByte(0x0D) // field-descriptor terminator

	for _, row := range rows {
		rec := make([]byte, recordLen)
		rec[0] = ' ' // active
		off := 1
		for i, f := range fields {
			var v string
			if i < len(row) {
				v = row[i]
			}
			cell := make([]byte, f.Len)
			for j := range cell {
				cell[j] = ' '
			}
			copy(cell, v) // copy truncates at f.Len
			copy(rec[off:off+f.Len], cell)
			off += f.Len
		}
		buf.Write(rec)
	}

	// dBASE EOF marker (0x1A) + padding. newDBFReader scans field
	// descriptors in 32-byte chunks and only inspects the first byte for
	// the 0x0D terminator, so it needs >=32 bytes available from the
	// terminator onward — a real DBF always has that (records + EOF), but
	// a tiny fixture can fall short and hit EOF mid-chunk. Trailing bytes
	// past recordCount records are never read, so this padding is inert.
	buf.WriteByte(0x1A)
	buf.Write(make([]byte, 32))
	return buf.Bytes()
}

// writeZipWithDBF writes a zip at zipPath containing a single member
// dbfName holding dbf — matching how StatsCan ships the boundary DBFs
// inside their boundary-file zips (openDBFFromZip reads the first .dbf).
func writeZipWithDBF(t *testing.T, zipPath, dbfName string, dbf []byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(dbfName)
	if err != nil {
		t.Fatalf("zip create member: %v", err)
	}
	if _, err := w.Write(dbf); err != nil {
		t.Fatalf("zip write member: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip %s: %v", zipPath, err)
	}
}
