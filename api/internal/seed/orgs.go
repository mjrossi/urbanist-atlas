// Package seed loads the hand-curated organizations dataset
// (api/seed/orgs.toml) into the organizations + organization_regions
// tables. It is the driver behind the `seed` subcommand.
//
// Each [[org]] entry creates one row in organizations and replaces
// the entire org_regions row set for that org wholesale, so removing
// a slug from the file actually unlinks the region.
//
// `loadregions` must run before `seed`: every region_slug must
// resolve to an existing region row. An unknown slug is a hard error
// (with a "did you mean" hint where cheap).
package seed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pelletier/go-toml/v2"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
)

// File is the root of orgs.toml.
type File struct {
	Orgs []Org `toml:"org"`
}

// Org is one [[org]] entry. Mirrors the wire/storage shape; the
// loader resolves RegionSlugs to region IDs via the regions table.
type Org struct {
	Slug        string   `toml:"slug"`
	Name        string   `toml:"name"`
	ShortDesc   string   `toml:"short_desc"`
	WebsiteURL  string   `toml:"website_url"`
	ContactURL  string   `toml:"contact_url,omitempty"`
	Tags        []string `toml:"tags"`
	RegionSlugs []string `toml:"region_slugs"`
}

// Summary is the per-run report returned by LoadFile.
type Summary struct {
	OrgsUpserted int
	RegionLinks  int
}

// LoadFile reads orgs.toml at path and upserts everything inside a
// single transaction.
func LoadFile(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, path string) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("seed: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	file, err := Parse(f)
	if err != nil {
		return Summary{}, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Summary{}, fmt.Errorf("seed: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	summary, err := apply(ctx, tx, logger, file)
	if err != nil {
		return Summary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Summary{}, fmt.Errorf("seed: commit: %w", err)
	}
	return summary, nil
}

// Parse decodes orgs.toml from r and runs structural validation.
func Parse(r io.Reader) (File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return File{}, fmt.Errorf("seed: read: %w", err)
	}
	if len(data) == 0 {
		return File{}, errors.New("seed: empty file")
	}
	var f File
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("seed: parse toml: %w", err)
	}
	if err := validate(f); err != nil {
		return File{}, err
	}
	return f, nil
}

func validate(f File) error {
	if len(f.Orgs) == 0 {
		return errors.New("seed: no orgs in file")
	}
	seen := map[string]bool{}
	for i, o := range f.Orgs {
		ctx := fmt.Sprintf("orgs[%d] (slug=%q)", i, o.Slug)
		if o.Slug == "" {
			return fmt.Errorf("%s: slug required", ctx)
		}
		if seen[o.Slug] {
			return fmt.Errorf("%s: duplicate slug", ctx)
		}
		seen[o.Slug] = true
		if o.Name == "" {
			return fmt.Errorf("%s: name required", ctx)
		}
		if o.ShortDesc == "" {
			return fmt.Errorf("%s: short_desc required", ctx)
		}
		if o.WebsiteURL == "" {
			return fmt.Errorf("%s: website_url required", ctx)
		}
		if len(o.RegionSlugs) == 0 {
			return fmt.Errorf("%s: region_slugs must have at least one entry", ctx)
		}
	}
	return nil
}

func apply(ctx context.Context, tx pgx.Tx, logger *slog.Logger, f File) (Summary, error) {
	q := gen.New(tx)
	summary := Summary{}
	for _, o := range f.Orgs {
		regionIDs, err := resolveRegionSlugs(ctx, q, o)
		if err != nil {
			return Summary{}, err
		}
		var contact pgtype.Text
		if o.ContactURL != "" {
			contact = pgtype.Text{String: o.ContactURL, Valid: true}
		}
		orgID, err := q.UpsertOrganization(ctx, gen.UpsertOrganizationParams{
			Slug:       o.Slug,
			Name:       o.Name,
			ShortDesc:  o.ShortDesc,
			WebsiteUrl: o.WebsiteURL,
			ContactUrl: contact,
			Tags:       o.Tags,
		})
		if err != nil {
			return Summary{}, fmt.Errorf("seed: upsert org %q: %w", o.Slug, err)
		}
		if err := q.DeleteOrganizationRegions(ctx, orgID); err != nil {
			return Summary{}, fmt.Errorf("seed: clear regions for %q: %w", o.Slug, err)
		}
		for _, rid := range regionIDs {
			if err := q.InsertOrganizationRegion(ctx, gen.InsertOrganizationRegionParams{
				OrganizationID: orgID,
				RegionID:       rid,
			}); err != nil {
				return Summary{}, fmt.Errorf("seed: link org %q to region %d: %w", o.Slug, rid, err)
			}
			summary.RegionLinks++
		}
		summary.OrgsUpserted++
		if logger != nil {
			logger.Info("seed: upserted org", "slug", o.Slug, "regions", len(regionIDs))
		}
	}
	return summary, nil
}

func resolveRegionSlugs(ctx context.Context, q *gen.Queries, o Org) ([]int64, error) {
	rows, err := q.RegionIDsBySlugs(ctx, o.RegionSlugs)
	if err != nil {
		return nil, fmt.Errorf("seed: resolve region slugs for %q: %w", o.Slug, err)
	}
	gotBySlug := make(map[string]int64, len(rows))
	for _, r := range rows {
		gotBySlug[r.Slug] = r.ID
	}
	var missing []string
	out := make([]int64, 0, len(o.RegionSlugs))
	seen := map[int64]struct{}{}
	for _, s := range o.RegionSlugs {
		id, ok := gotBySlug[s]
		if !ok {
			missing = append(missing, s)
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("seed: org %q references unknown region slug(s) %v — did you run `just loadregions` for the right country?", o.Slug, missing)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
