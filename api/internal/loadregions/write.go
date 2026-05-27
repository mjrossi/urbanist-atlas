package loadregions

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
)

// Summary is the per-run report returned by LoadFile.
type Summary struct {
	Regions     int
	ParentEdges int
	// Batches counts the number of UNNEST-upsert chunks the loader
	// issued (regions + parent-edge batches summed). Exposed so a
	// regression that reverts to per-row Exec calls fails a
	// count-based assertion.
	Batches int
}

// batchSize bounds each UNNEST-upsert chunk so the Postgres parameter
// cap (~65,535 per statement) stays safely below the ceiling — even
// for the regions upsert that ferries six parallel arrays per row,
// 500 × 6 = 3,000 elements is an order of magnitude under. The cap
// also keeps per-statement parse time and memory footprint bounded
// for very large region files (e.g. a future country with 10k+ rows).
//
// Mirrors internal/loadpostal/csv.go's batchSize for symmetry across
// the two seed-import paths.
const batchSize = 500

// upsertRegionsBatchStmt is issued via raw pgx.Query rather than
// sqlc-generated code because sqlc 1.x can't type-check multi-arg
// UNNEST against the parameter signature. pgx handles []string → text[]
// and []int32 → int[] natively, so the call site stays almost as
// clean as a sqlc-generated wrapper.
const upsertRegionsBatchStmt = `
INSERT INTO regions (country, kind, name, slug, scope_tier, sort_priority)
SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::int[])
ON CONFLICT (slug) DO UPDATE
SET country       = EXCLUDED.country,
    kind          = EXCLUDED.kind,
    name          = EXCLUDED.name,
    scope_tier    = EXCLUDED.scope_tier,
    sort_priority = EXCLUDED.sort_priority
RETURNING id, slug`

const deleteRegionParentsBatchStmt = `
DELETE FROM region_parents WHERE region_id = ANY($1::bigint[])`

const insertRegionParentsBatchStmt = `
INSERT INTO region_parents (region_id, parent_region_id)
SELECT * FROM unnest($1::bigint[], $2::bigint[])
ON CONFLICT DO NOTHING`

// LoadFile parses a TOML file at path, validates it (structural +
// cycle check), and writes the resulting regions + region_parents
// rows inside a single transaction. Country is stamped on every
// region row.
//
// Implementation: three batched UNNEST passes to keep round-trips
// bounded:
//
//  1. Bulk-upsert all regions and collect (slug → id) from RETURNING.
//  2. Resolve every cross-file parent slug in one RegionIDsBySlugs
//     query (sqlc-generated) and merge into the rid map.
//  3. Bulk-delete the parent-edge rows for every in-file region in
//     one statement, then bulk-insert the (region_id, parent_id)
//     pairs via UNNEST in batches of `batchSize`.
//
// For a US-states file (~50 rows, 0 parent edges) this is 2
// round-trips. For the US MSAs file (~393 rows, ~393 edges) it's 4.
// For an arbitrarily large country file the worst case is
// ceil(N/batchSize) region batches + 1 cross-file lookup +
// 1 delete + ceil(E/batchSize) edge batches — no per-region work.
//
// Idempotent: re-running with the same input produces the same row
// set (ON CONFLICT … DO UPDATE keeps every column current; the
// wholesale-delete-then-re-insert pattern keeps the parent set in
// lockstep with the file).
func LoadFile(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, path, country string) (Summary, error) {
	f, err := openAndParse(path)
	if err != nil {
		return Summary{}, err
	}
	if err := DetectCycles(f); err != nil {
		return Summary{}, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Summary{}, fmt.Errorf("loadregions: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	summary := Summary{}
	rid := map[string]int64{}

	// Stage 1: bulk-upsert regions and populate rid from RETURNING.
	for start := 0; start < len(f.Regions); start += batchSize {
		end := min(start+batchSize, len(f.Regions))
		chunk := f.Regions[start:end]

		countries := make([]string, len(chunk))
		kinds := make([]string, len(chunk))
		names := make([]string, len(chunk))
		slugs := make([]string, len(chunk))
		tiers := make([]string, len(chunk))
		priorities := make([]int32, len(chunk))
		for i, r := range chunk {
			countries[i] = country
			kinds[i] = r.Kind
			names[i] = r.Name
			slugs[i] = r.Slug
			tiers[i] = r.ScopeTier
			priorities[i] = int32(r.SortPriority)
		}

		rows, err := tx.Query(ctx, upsertRegionsBatchStmt,
			countries, kinds, names, slugs, tiers, priorities)
		if err != nil {
			return Summary{}, fmt.Errorf("loadregions: batch upsert regions [%d:%d): %w", start, end, err)
		}
		// Closure so a single `defer rows.Close()` covers every exit
		// path — including the rows.Err() check, which otherwise
		// returned without an explicit Close. pgx v5 auto-closes when
		// Next() returns false so this is defensive, not a leak fix.
		if err := func() error {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var slug string
				if err := rows.Scan(&id, &slug); err != nil {
					return fmt.Errorf("scan returning: %w", err)
				}
				rid[slug] = id
			}
			return rows.Err()
		}(); err != nil {
			return Summary{}, fmt.Errorf("loadregions: rows iter [%d:%d): %w", start, end, err)
		}

		summary.Regions += len(chunk)
		summary.Batches++
	}

	// Stage 2: resolve cross-file parents in a single round-trip. The
	// canonical use case is a leaf-tier file referencing state-tier
	// regions loaded earlier (e.g. regions_us.toml leaves parenting
	// under states from regions_us_states.toml).
	q := gen.New(tx)
	var crossFileSlugs []string
	seenCF := map[string]bool{}
	for _, r := range f.Regions {
		for _, ps := range r.Parents {
			if _, ok := rid[ps]; ok {
				continue
			}
			if seenCF[ps] {
				continue
			}
			seenCF[ps] = true
			crossFileSlugs = append(crossFileSlugs, ps)
		}
	}
	if len(crossFileSlugs) > 0 {
		cfRows, err := q.RegionIDsBySlugs(ctx, crossFileSlugs)
		if err != nil {
			return Summary{}, fmt.Errorf("loadregions: resolve cross-file parents: %w", err)
		}
		got := make(map[string]int64, len(cfRows))
		for _, row := range cfRows {
			got[row.Slug] = row.ID
			rid[row.Slug] = row.ID
		}
		var missing []string
		for _, s := range crossFileSlugs {
			if _, ok := got[s]; !ok {
				missing = append(missing, s)
			}
		}
		if len(missing) > 0 {
			return Summary{}, fmt.Errorf("loadregions: parent slug(s) not found in file or DB: %v (load the file that defines them first)", missing)
		}
	}

	// Stage 3: bulk-delete existing parent edges for every in-file
	// region. The wholesale-replace pattern matches the prior
	// per-region DeleteRegionParents loop; doing it in one statement
	// keeps the round-trip count constant.
	inFileIDs := make([]int64, 0, len(f.Regions))
	for _, r := range f.Regions {
		inFileIDs = append(inFileIDs, rid[r.Slug])
	}
	if _, err := tx.Exec(ctx, deleteRegionParentsBatchStmt, inFileIDs); err != nil {
		return Summary{}, fmt.Errorf("loadregions: bulk delete parent edges: %w", err)
	}

	// Stage 4: bulk-insert (region_id, parent_id) pairs in
	// batchSize-row chunks. Typical loads (≤500 edges) fit in one
	// batch; the loop is defensive for future bigger files.
	var edgeRegions, edgeParents []int64
	for _, r := range f.Regions {
		childID := rid[r.Slug]
		for _, ps := range r.Parents {
			edgeRegions = append(edgeRegions, childID)
			edgeParents = append(edgeParents, rid[ps])
		}
	}
	for start := 0; start < len(edgeRegions); start += batchSize {
		end := min(start+batchSize, len(edgeRegions))
		if _, err := tx.Exec(ctx, insertRegionParentsBatchStmt,
			edgeRegions[start:end], edgeParents[start:end]); err != nil {
			return Summary{}, fmt.Errorf("loadregions: bulk insert parent edges [%d:%d): %w", start, end, err)
		}
		summary.Batches++
	}
	summary.ParentEdges = len(edgeRegions)

	if err := tx.Commit(ctx); err != nil {
		return Summary{}, fmt.Errorf("loadregions: commit: %w", err)
	}
	if logger != nil {
		logger.Info("loadregions: complete",
			"country", country,
			"regions", summary.Regions,
			"parent_edges", summary.ParentEdges,
			"batches", summary.Batches,
		)
	}
	return summary, nil
}

func openAndParse(path string) (File, error) {
	r, err := openFile(path)
	if err != nil {
		return File{}, err
	}
	defer r.Close()
	return Parse(r)
}

func openFile(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("loadregions: open %s: %w", path, err)
	}
	return f, nil
}
