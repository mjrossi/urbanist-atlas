# Runbook: nightly Postgres backups → Cloudflare R2

Step-by-step for enabling
[`.github/workflows/backup.yml`](../../.github/workflows/backup.yml).
After this runs end-to-end once, the cron takes over and nightly
`pg_dump`s land in R2 with a 30-day retention window.

Why R2: it's the existing Cloudflare account that already hosts the
SPA, the egress cost is zero on Cloudflare's pricing, and an
S3-compatible API plays nicely with `awscli v1` (which ships on
`ubuntu-latest`). Combined with Fly's automatic 5-day volume
snapshots on the sibling Postgres app, this gives off-Fly durability
plus a deterministic `pg_restore` path.

Estimated time: **15–20 minutes** of dashboard clicking once you
have both accounts open.

## Prereqs

- Maintainer access to the Cloudflare account that owns
  `urbanistatlas.com` (R2 is on the free tier; no card required).
- Maintainer access to the `mjrossi/urbanist-atlas` GitHub repo
  (Settings → Secrets needs write).
- `flyctl` installed and authenticated against an account with
  read access to `urbanist-atlas-db` (the sibling Postgres app).

## Step 1 — Generate the Fly API token

GitHub Actions needs a non-interactive credential to `flyctl ssh
console` into the Postgres app.

```sh
flyctl auth token
```

The output is the token (single line, starts with `FlyV1 ` or
similar). Copy it; you'll paste it into GitHub Secrets in step 6.
Treat this like a password — anyone with it can run `flyctl deploy`
against your apps.

If you'd rather scope a narrower token, generate it from the Fly
dashboard at <https://fly.io/user/personal_access_tokens> — the
workflow only needs `read:apps` + `ssh:console` against
`urbanist-atlas-db`.

## Step 2 — Find your Cloudflare account ID

The R2 S3 endpoint is `<account-id>.r2.cloudflarestorage.com`, so the
workflow needs your account ID as a secret.

1. Open the Cloudflare dashboard.
2. Pick any zone (e.g., `urbanistatlas.com`) from the sidebar.
3. Scroll to the right-hand sidebar — **Account ID** is right there,
   a 32-character hex string. Click the copy icon.

(Same value appears under **Workers & Pages** → **Account details**
if you'd rather not navigate via a zone.)

## Step 3 — Create the R2 bucket

1. Dashboard → **R2** in the left sidebar.
   - First time using R2? You'll get a Terms acceptance + a
     one-time "enable R2" toggle. No payment method required for
     the free tier.
2. **Create bucket**.
3. Settings:
   - **Bucket name:** `urbanist-atlas-backups` *(must match the
     workflow exactly; the value is hardcoded in `backup.yml` and
     referenced in `just db-restore`)*
   - **Location:** Automatic (Cloudflare picks the closest hint
     region)
   - **Storage class:** Standard
4. **Create bucket**.

You'll land on the bucket's overview page. Leave it open — step 4
adds a lifecycle rule from here.

## Step 4 — Add the 30-day expiration rule

Without this rule, backups accumulate forever. Free tier R2 allows
10 GB of storage; a ~5 MB nightly backup × 30 days = ~150 MB, well
under the cap. But R2 has no built-in expiry default, so we set it
explicitly.

1. From the bucket overview, click **Settings** (top tab).
2. Scroll to **Object lifecycle rules** → **Add rule**.
3. Fill in:
   - **Rule name:** `expire-30d`
   - **Scope:** "Apply to all objects in the bucket"
     *(no prefix or tag filters)*
   - **Action:** **Delete objects** after `30` days from upload
4. **Add rule**.

The rule applies prospectively — existing objects past 30 days get
deleted on the next R2 lifecycle sweep (runs daily).

## Step 5 — Create an R2 API token

The workflow uses an S3-style access-key pair (not a Cloudflare
API token). Generate one scoped to just this bucket.

1. Dashboard → **R2** → **Manage R2 API Tokens** *(button on the
   R2 overview page; also accessible from the left rail
   submenu)*.
2. **Create API token**.
3. Fill in:
   - **Token name:** `urbanist-atlas-backup-uploader`
   - **Permissions:** **Object Read & Write**
     *(do NOT pick "Admin Read & Write" — uploader doesn't need
     bucket-management or other-bucket access)*
   - **Specify bucket(s):** **Apply to specific buckets only**
     → select `urbanist-atlas-backups`
   - **TTL:** Forever *(or set a calendar reminder to rotate; the
     workflow handles a token swap with no code change)*
   - **Client IP Address Filtering:** leave blank (GHA's IP pool
     is too broad to filter)
4. **Create API Token**.

The next screen shows the credentials **exactly once**:

- **Access Key ID** — short, 32-char hex
- **Secret Access Key** — longer, 64-char hex
- An **endpoint** URL (matches `<account-id>.r2.cloudflarestorage.com`)

Copy both keys somewhere temporary. If you close this page without
copying, you'll need to delete and recreate the token.

## Step 6 — Add the secrets to GitHub

GitHub repo → **Settings** → **Secrets and variables** → **Actions**
→ **New repository secret**. Add all four:

| Secret name | Value |
|---|---|
| `FLY_API_TOKEN` | From step 1 |
| `CF_ACCOUNT_ID` | From step 2 (32-char hex) |
| `R2_ACCESS_KEY_ID` | From step 5 (Access Key ID) |
| `R2_SECRET_ACCESS_KEY` | From step 5 (Secret Access Key) |

Spelling matters — `backup.yml` references these exact names. After
all four are saved, the Actions tab will show them as masked.

## Step 7 — Trigger the workflow manually

1. GitHub repo → **Actions** tab.
2. Left sidebar → **backup-postgres** workflow.
3. **Run workflow** → branch `main` → **Run workflow**.
4. Refresh; a new run appears within a few seconds.

Expected timing: the run completes in **30–60 seconds**. The three
steps you'll see:

- *Install flyctl* — ~5 s
- *Capture pg_dump from Fly Postgres* — ~10–30 s depending on data
  size; streams the dump over the Fly SSH tunnel
- *Upload to R2* — ~5 s for the current ~5 MB dump

The Summary at the bottom of a successful run shows the filename
(`YYYY-MM-DD.sql.gz`), the bucket, and the retention policy. Save
this run URL — you'll reference it in step 8.

## Step 8 — Verify the backup actually landed

1. Cloudflare dashboard → **R2** → **urbanist-atlas-backups**.
2. You should see one object: `2026-MM-DD.sql.gz` (today's date in
   UTC, since the workflow uses `date -u`).
3. Click the object → **Download** → save locally.
4. Quick integrity check:

   ```sh
   gunzip -t 2026-MM-DD.sql.gz && echo "gzip OK"
   ```

5. Optional: peek at the dump's schema/content without restoring:

   ```sh
   gunzip -c 2026-MM-DD.sql.gz | head -50
   gunzip -c 2026-MM-DD.sql.gz | wc -l       # ~1k–10k lines for a healthy dump
   gunzip -c 2026-MM-DD.sql.gz | grep -c '^COPY '   # number of tables dumped
   ```

   You should see ≥5 `COPY` statements (regions, region_parents,
   postal_codes, organizations, organization_regions) plus the
   submissions table.

If the file is suspiciously small (< 100 KB) the dump likely failed
silently — the workflow's `test -s` check catches an empty file but
not a partial one. Spot-check the line count.

## Step 9 — Confirm the cron is scheduled

GitHub repo → **Actions** → **backup-postgres** → top right corner
should now show a "next run at" timestamp. Default schedule is
`0 7 * * *` UTC, which is 02:00 America/New_York (EST) or 03:00
during EDT.

GitHub Actions cron has a known quirk: it doesn't fire on a public
repo that's seen no activity for 60+ days. Not a concern for active
projects, but worth knowing.

## Restoring from a backup

The restore path is the inverse: download from R2, stream into the
sibling Postgres app's `psql`.

```sh
# 1. Download a specific date from R2. The aws CLI uses the same
#    credentials you stored in GitHub Secrets — set them locally:
export AWS_ACCESS_KEY_ID=...       # R2_ACCESS_KEY_ID
export AWS_SECRET_ACCESS_KEY=...   # R2_SECRET_ACCESS_KEY
export AWS_DEFAULT_REGION=auto
export CF_ACCOUNT_ID=...

aws s3 cp \
    "s3://urbanist-atlas-backups/2026-05-15.sql.gz" ./ \
    --endpoint-url "https://${CF_ACCOUNT_ID}.r2.cloudflarestorage.com"

# 2. Confirm the file integrity locally.
gunzip -t 2026-05-15.sql.gz

# 3. Restore into the Fly Postgres app. The `just db-restore` recipe
#    wraps the flyctl ssh + psql plumbing.
just db-restore 2026-05-15.sql.gz
```

**Before restoring into production:** this is a destructive
operation. Confirm you're operating against the right app
(`flyctl status -a urbanist-atlas-db`) and consider running against
a fresh ephemeral app first if the dataset is large or the change
isn't easily reversible. The 5-day Fly volume snapshot is your
last-resort rollback.

## Rotation + maintenance

- **Rotate the R2 token** every 6–12 months. Generate a new one
  (step 5), update `R2_ACCESS_KEY_ID` + `R2_SECRET_ACCESS_KEY` in
  GitHub Secrets (step 6), then delete the old token from the R2
  dashboard.
- **Rotate the Fly token** if a maintainer leaves the project, or
  on the same 6–12 month cadence as the R2 token.
- **Audit retention** quarterly: spot-check that the R2 bucket has
  ~30 dated objects and no stragglers older than 31 days. If
  stragglers appear, re-check the lifecycle rule (step 4).
- **Test restore** every 6 months against an ephemeral Fly app
  (`flyctl apps create urbanist-atlas-restore-test --org personal`
  + `flyctl postgres create` + restore). Untested backups are
  half-backups.

## Troubleshooting

**Workflow fails at "Capture pg_dump" with `Error: SSH error`.** The
Fly token doesn't have SSH console permission for
`urbanist-atlas-db`. Regenerate with `flyctl auth token` from an
account that does, or scope the dashboard token to include
`ssh:console`.

**Workflow fails at "Upload to R2" with `An error occurred (403)
when calling the PutObject operation: Access Denied`.** Either the
R2 API token is scoped to a different bucket, or the permissions
were set to "Object Read" instead of "Object Read & Write". Recreate
the token (step 5) and update both secrets.

**Workflow fails at "Upload to R2" with `Could not connect to the
endpoint URL`.** `CF_ACCOUNT_ID` is wrong or has stray whitespace.
Re-copy from the dashboard (step 2) and update the secret.

**Backup file is suspiciously small (< 100 KB).** `pg_dump` partially
succeeded but the Fly SSH tunnel dropped. Re-run the workflow; if it
fails again, run `just db-backup` locally to bisect (that recipe
uses the same `flyctl ssh` plumbing without the R2 upload).

**Bucket has objects older than 30 days.** The lifecycle rule didn't
apply — re-check step 4. Cloudflare's lifecycle sweep runs daily, so
allow 24h between adding the rule and expecting deletions.
