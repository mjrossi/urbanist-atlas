// Package seed loads the hand-curated organizations dataset
// (api/seed/orgs.yaml) into the organizations + organization_regions
// tables. It is the driver behind the `seed` subcommand.
//
// Region linkage is expressed in the YAML as country + a list of
// postal codes the org covers. The loader resolves those through the
// already-populated postal_codes table to get a deduplicated set of
// region IDs, then upserts the org and replaces its organization_regions
// rows inside a single transaction.
//
// `loadpostal` must run before `seed`: an org whose postal codes don't
// resolve to any region is treated as a hard error (rather than
// silently writing an org no lookup can ever find).
package seed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// File is the root of orgs.yaml.
type File struct {
	Orgs []Org `yaml:"orgs"`
}

// Org mirrors atlas.Org plus a regions list used by the loader to
// resolve postal codes to region IDs.
type Org struct {
	Slug       string       `yaml:"slug"`
	Name       string       `yaml:"name"`
	ShortDesc  string       `yaml:"short_desc"`
	WebsiteURL string       `yaml:"website_url"`
	ContactURL string       `yaml:"contact_url,omitempty"`
	Tags       []string     `yaml:"tags"`
	Regions    []RegionSpec `yaml:"regions"`
}

// RegionSpec is one country + list-of-postal-codes block under an
// org's `regions:` key. The loader resolves each listed postal code to
// the four region IDs (city/county/metro/state) it falls within, then
// unions them across all blocks.
type RegionSpec struct {
	Country     string   `yaml:"country"`
	PostalCodes []string `yaml:"postal_codes"`
}

// LoadFile reads orgs.yaml at path and upserts everything inside a
// single transaction. See package doc for the resolution rules.
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

// Summary is the per-run report returned by LoadFile.
type Summary struct {
	OrgsUpserted int
	RegionLinks  int
}

// Parse decodes orgs.yaml from r and runs structural validation.
// Returns an error pointing at the offending org if anything is missing
// — slug uniqueness, required fields, at least one region.
func Parse(r io.Reader) (File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return File{}, fmt.Errorf("seed: read: %w", err)
	}
	var f File
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("seed: parse yaml: %w", err)
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
	seenSlug := map[string]bool{}
	for i, o := range f.Orgs {
		ctx := fmt.Sprintf("orgs[%d] (slug=%q)", i, o.Slug)
		if o.Slug == "" {
			return fmt.Errorf("%s: slug required", ctx)
		}
		if seenSlug[o.Slug] {
			return fmt.Errorf("%s: duplicate slug", ctx)
		}
		seenSlug[o.Slug] = true
		if o.Name == "" {
			return fmt.Errorf("%s: name required", ctx)
		}
		if o.ShortDesc == "" {
			return fmt.Errorf("%s: short_desc required", ctx)
		}
		if o.WebsiteURL == "" {
			return fmt.Errorf("%s: website_url required", ctx)
		}
		if len(o.Regions) == 0 {
			return fmt.Errorf("%s: at least one regions entry required", ctx)
		}
		for j, r := range o.Regions {
			rctx := fmt.Sprintf("%s.regions[%d]", ctx, j)
			c := atlas.Country(r.Country)
			if c != atlas.CountryUS && c != atlas.CountryCA {
				return fmt.Errorf("%s: country must be US or CA (got %q)", rctx, r.Country)
			}
			if len(r.PostalCodes) == 0 {
				return fmt.Errorf("%s: postal_codes must have at least one entry", rctx)
			}
		}
	}
	return nil
}

func apply(ctx context.Context, tx pgx.Tx, logger *slog.Logger, f File) (Summary, error) {
	q := gen.New(tx)
	summary := Summary{}

	for _, o := range f.Orgs {
		regionIDs, err := resolveRegionIDs(ctx, tx, o)
		if err != nil {
			return Summary{}, err
		}
		if len(regionIDs) == 0 {
			return Summary{}, fmt.Errorf("seed: org %q: no regions resolved (did you run loadpostal first?)", o.Slug)
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

		// Replace org_regions wholesale so a curated edit that removes a
		// region from the YAML actually removes the link in the DB.
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

// resolveRegionIDs walks every postal code in o.Regions, resolves each
// to its region IDs via the postal_codes table, and returns the union
// (deduplicated, deterministic order). An unknown postal code is a
// hard error — silently skipping would let an org "disappear" with no
// warning.
func resolveRegionIDs(ctx context.Context, tx pgx.Tx, o Org) ([]int64, error) {
	seen := map[int64]struct{}{}
	var out []int64
	for _, spec := range o.Regions {
		country := atlas.Country(spec.Country)
		for _, raw := range spec.PostalCodes {
			normalized := normalizePostalCode(country, raw)
			if normalized == "" {
				return nil, fmt.Errorf("org %q: empty postal_code entry", o.Slug)
			}
			row := tx.QueryRow(ctx, `
				SELECT city_region_id, county_region_id, metro_region_id, state_region_id
				FROM postal_codes
				WHERE country = $1 AND postal_code = $2`,
				string(country), normalized)
			var city, county, metro, state pgtype.Int8
			if err := row.Scan(&city, &county, &metro, &state); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, fmt.Errorf("org %q: postal_code %s/%s not found (did loadpostal run for this country?)", o.Slug, country, normalized)
				}
				return nil, fmt.Errorf("org %q: resolve %s/%s: %w", o.Slug, country, normalized, err)
			}
			for _, id := range []pgtype.Int8{city, county, metro, state} {
				if !id.Valid {
					continue
				}
				if _, ok := seen[id.Int64]; ok {
					continue
				}
				seen[id.Int64] = struct{}{}
				out = append(out, id.Int64)
			}
		}
	}
	return out, nil
}

// normalizePostalCode mirrors pkg/atlas's rules (see memstore.go).
// Duplicated to avoid the seed package importing memstore.
func normalizePostalCode(country atlas.Country, code string) string {
	c := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if country == atlas.CountryCA && len(c) > 3 {
		c = c[:3]
	}
	return c
}
