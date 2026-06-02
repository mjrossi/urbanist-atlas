package seedfiles_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
	seedfs "github.com/mjrossi/urbanist-atlas/api/seed"
)

func seedDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "seed"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

func TestBuildMemStore_FromDisk(t *testing.T) {
	store, err := seedfiles.BuildMemStore(nil, os.DirFS(seedDir(t)))
	if err != nil {
		t.Fatalf("BuildMemStore: %v", err)
	}
	exerciseStore(t, store)
}

func TestBuildMemStore_FromEmbed(t *testing.T) {
	store, err := seedfiles.BuildMemStore(nil, seedfs.FS)
	if err != nil {
		t.Fatalf("BuildMemStore embed: %v", err)
	}
	exerciseStore(t, store)
}

// TestBuildMemStore_LogsSeedCounts confirms the boot summary line carries
// the loaded-data counts an operator relies on at startup — regions,
// orgs, and the postal-code total (the largest dataset, previously only
// logged per-country at DEBUG).
func TestBuildMemStore_LogsSeedCounts(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil)) // default level: INFO

	if _, err := seedfiles.BuildMemStore(logger, seedfs.FS); err != nil {
		t.Fatalf("BuildMemStore: %v", err)
	}

	rec := findLogRecord(t, buf.Bytes(), "seedfiles: filestore built")
	for _, key := range []string{"regions", "orgs", "postal_codes"} {
		v, ok := rec[key]
		if !ok {
			t.Errorf("summary line missing %q field; line: %v", key, rec)
			continue
		}
		if n, ok := v.(float64); !ok || n <= 0 {
			t.Errorf("%q = %v; want a positive count", key, v)
		}
	}
}

// findLogRecord scans newline-delimited JSON log output for the first
// record whose "msg" equals want and returns it decoded.
func findLogRecord(t *testing.T, out []byte, want string) map[string]any {
	t.Helper()
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("unmarshal log line %q: %v", line, err)
		}
		if rec["msg"] == want {
			return rec
		}
	}
	t.Fatalf("no log record with msg=%q in output:\n%s", want, out)
	return nil
}

// TestBuildMemStore_DiskAndEmbedAgree pins the production guarantee
// that the embed and the on-disk bundle resolve identically. If the
// embed directive ever drifts from the actual seed/ directory (e.g.
// a new file slips in but isn't matched), this catches it.
func TestBuildMemStore_DiskAndEmbedAgree(t *testing.T) {
	disk, err := seedfiles.BuildMemStore(nil, os.DirFS(seedDir(t)))
	if err != nil {
		t.Fatalf("disk: %v", err)
	}
	embed, err := seedfiles.BuildMemStore(nil, seedfs.FS)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	ctx := context.Background()
	for _, tc := range []struct {
		country atlas.Country
		code    string
	}{
		{atlas.CountryUS, "11217"}, // Brooklyn
		{atlas.CountryUS, "94110"}, // SF
		{atlas.CountryUS, "20811"}, // HUD non-ZCTA → Bethesda
		{atlas.CountryCA, "H3A 0G4"},
	} {
		d, errD := disk.ResolveLeafRegion(ctx, tc.country, tc.code)
		e, errE := embed.ResolveLeafRegion(ctx, tc.country, tc.code)
		if errD != nil || errE != nil {
			t.Errorf("resolve %s/%s: disk=%v embed=%v", tc.country, tc.code, errD, errE)
			continue
		}
		if d.Slug != e.Slug {
			t.Errorf("leaf slug differs for %s/%s: disk=%q embed=%q",
				tc.country, tc.code, d.Slug, e.Slug)
		}
	}

	dRegions, _ := disk.ListRegions(ctx)
	eRegions, _ := embed.ListRegions(ctx)
	if len(dRegions) != len(eRegions) {
		t.Errorf("ListRegions count differs: disk=%d embed=%d", len(dRegions), len(eRegions))
	}
	dRecent, _ := disk.ListRecent(ctx)
	eRecent, _ := embed.ListRecent(ctx)
	if len(dRecent) != len(eRecent) {
		t.Errorf("ListRecent count differs: disk=%d embed=%d", len(dRecent), len(eRecent))
	}
}

// exerciseStore is the shared body of the disk/embed BuildMemStore
// smoke tests: it asserts the FileStore answers a real US ZIP and CA
// FSA, returns non-empty browse + recent lists, and that the lookup
// chain (postal → leaf → ancestors → orgs) produces orgs.
func exerciseStore(t *testing.T, store *atlas.MemStore) {
	t.Helper()
	ctx := context.Background()

	leaf, err := store.ResolveLeafRegion(ctx, atlas.CountryUS, "11217")
	if err != nil {
		t.Fatalf("ResolveLeafRegion 11217: %v", err)
	}
	if leaf.Slug == "" {
		t.Fatal("11217 resolved to a region with empty slug")
	}
	ancestors, err := store.AncestorRegions(ctx, leaf.ID)
	if err != nil {
		t.Fatalf("AncestorRegions: %v", err)
	}
	if len(ancestors) < 2 {
		t.Fatalf("want leaf + at least one ancestor, got %d", len(ancestors))
	}
	ancestorIDs := make([]int64, 0, len(ancestors))
	for _, r := range ancestors {
		ancestorIDs = append(ancestorIDs, r.ID)
	}
	orgs, err := store.OrgsForRegions(ctx, ancestorIDs)
	if err != nil {
		t.Fatalf("OrgsForRegions: %v", err)
	}
	if len(orgs) == 0 {
		t.Fatal("expected at least one org for 11217 ancestry, got none")
	}

	if _, err := store.ResolveLeafRegion(ctx, atlas.CountryCA, "H3A 0G4"); err != nil {
		t.Fatalf("ResolveLeafRegion H3A 0G4: %v", err)
	}

	regions, err := store.ListRegions(ctx)
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) == 0 {
		t.Fatal("ListRegions returned no regions with orgs")
	}

	recent, err := store.ListRecent(ctx)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(recent) == 0 {
		t.Fatal("ListRecent returned no orgs")
	}
	if len(recent) > 10 {
		t.Fatalf("ListRecent returned %d > 10 orgs", len(recent))
	}
}
