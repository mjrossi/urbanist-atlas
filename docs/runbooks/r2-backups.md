# Runbook: nightly SQLite backups → Cloudflare R2

Step-by-step for enabling
[`.github/workflows/backup-sqlite.yml`](../../.github/workflows/backup-sqlite.yml).
After this runs end-to-end once, the cron takes over and nightly
SQLite dumps land in R2 with a 30-day retention window.

This runbook covers the one-time R2 + secrets setup. The dump and
restore mechanics are documented authoritatively in
[`docs/deploy.md` §Backups](../deploy.md); this page and that section
must stay in sync.

Why R2: it's the existing Cloudflare account that already hosts the
SPA, the egress cost is zero on Cloudflare's pricing, and an
S3-compatible API plays nicely with `rclone` (which the workflow
installs on `ubuntu-latest`). Combined with Fly's automatic volume
snapshots on the `urbanist-atlas` app, this gives off-Fly durability
plus a deterministic restore path.

The submissions DB is the only writable state — reads are served
from the `api/seed/` bundle baked into the image, so there is nothing
else to back up.

Estimated time: **15–20 minutes** of dashboard clicking once you
have both accounts open.

## Prereqs

- Maintainer access to the Cloudflare account that owns
  `urbanistatlas.com` (R2 is on the free tier; no card required).
- Maintainer access to the `mjrossi/urbanist-atlas` GitHub repo
  (Settings → Secrets needs write).
- `flyctl` installed and authenticated against an account with
  SSH-console access to `urbanist-atlas` (the API app that owns the
  `atlas_data` volume).

## Step 1 — Fly API token

GitHub Actions needs a non-interactive credential to `flyctl ssh
console` / `sftp` into the `urbanist-atlas` machine and read
`/data/atlas.db`. The workflow reuses **`FLY_API_TOKEN_DEPLOY`** —
the same secret `ci.yml` already uses for `flyctl deploy` — so if CI
deploys are working, this secret is already set and you can skip
ahead to step 2.

If you need to (re)generate it:

```sh
flyctl auth token
```

The output is the token (single line, starts with `FlyV1 ` or
similar). Treat it like a password — anyone with it can run `flyctl
deploy` against your apps. If you'd rather scope a narrower token,
generate it from the Fly dashboard at
<https://fly.io/user/personal_access_tokens>; the workflow needs
`deploy` + `ssh:console` against `urbanist-atlas`.

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
   - **Bucket name:** `urbanist-atlas-backups` *(this is the value
     you'll store in the `R2_BACKUP_BUCKET` secret in step 6; the
     workflow reads the bucket from that secret)*
   - **Location:** Automatic (Cloudflare picks the closest hint
     region)
   - **Storage class:** Standard
4. **Create bucket**.

You'll land on the bucket's overview page. Leave it open — step 4
adds a lifecycle rule from here.

## Step 4 — Add the 30-day expiration rule

Without this rule, backups accumulate forever. Free tier R2 allows
10 GB of storage; a small nightly `.sql.gz` × 30 days stays well
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
     *(do NOT pick "Admin Read & Write" — the uploader doesn't need
     bucket-management or other-bucket access. The workflow already
     sets `RCLONE_CONFIG_R2_NO_CHECK_BUCKET=true` so rclone won't
     attempt a HeadBucket the scoped token can't perform.)*
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
→ **New repository secret**. The workflow reads these names:

| Secret name | Value |
|---|---|
| `FLY_API_TOKEN_DEPLOY` | From step 1 *(already set if CI deploys work)* |
| `CF_ACCOUNT_ID` | From step 2 (32-char hex) |
| `R2_ACCESS_KEY_ID` | From step 5 (Access Key ID) |
| `R2_SECRET_ACCESS_KEY` | From step 5 (Secret Access Key) |
| `R2_BACKUP_BUCKET` | `urbanist-atlas-backups` (from step 3) |

Spelling matters — `backup-sqlite.yml` references these exact names.
After they're saved, the Actions tab will show them as masked.

> ⚠️ **Strip trailing newlines if you set secrets via `gh secret set`.**
> The gh CLI stores stdin verbatim and does **not** trim the newline
> that `flyctl auth token` (and most CLI output) emits. A token with
> a stray `\n` causes flyctl to fail with
> `net/http: invalid header field value for "Authorization"` — the
> token looks correct in the dashboard but the workflow can't use
> it. Use `tr -d '\n'` or `printf '%s'` to strip:
>
> ```sh
> flyctl auth token | tr -d '\n' | gh secret set FLY_API_TOKEN_DEPLOY
> printf '%s' '<paste-account-id>' | gh secret set CF_ACCOUNT_ID
> ```
>
> Setting secrets via the web UI is paste-safe — the form strips the
> newline at submit time. Only `gh secret set` (and other stdin-piped
> tools) carry this hazard.

## Step 7 — Trigger the workflow manually

1. GitHub repo → **Actions** tab.
2. Left sidebar → **SQLite nightly backup** workflow.
3. **Run workflow** → branch `main` → **Run workflow**.
4. Refresh; a new run appears within a few seconds.

Expected timing: the run completes in **under a minute**. The steps
you'll see:

- *Set up flyctl* — ~5 s
- *Snapshot SQLite via flyctl ssh* — writes
  `sqlite3 /data/atlas.db .dump | gzip` to a temp file on the machine,
  `sftp get`s it back as `atlas-<stamp>.sql.gz`, then `gunzip -t`s it
  locally (the dump is streamed through an inner `sh -eu -o pipefail`
  so a failed `sqlite3` propagates instead of leaving an empty gzip)
- *Set up rclone* + *Upload to R2* — ~5 s for the small dump

Save the run URL — you'll reference it in step 8.

## Step 8 — Verify the backup actually landed

1. Cloudflare dashboard → **R2** → **urbanist-atlas-backups**.
2. You should see one object: `atlas-YYYY-MM-DD-HHMM.sql.gz` (today's
   UTC date + time, since the workflow uses `date -u`).
3. Click the object → **Download** → save locally.
4. Quick integrity check:

   ```sh
   gunzip -t atlas-2026-MM-DD-HHMM.sql.gz && echo "gzip OK"
   ```

5. Optional: peek at the dump without restoring. A SQLite `.dump` is
   plain SQL text — `PRAGMA`/`BEGIN`/`CREATE TABLE`/`INSERT`:

   ```sh
   gunzip -c atlas-2026-MM-DD-HHMM.sql.gz | head -30
   gunzip -c atlas-2026-MM-DD-HHMM.sql.gz | grep -c 'CREATE TABLE'    # ≥1 (submissions)
   gunzip -c atlas-2026-MM-DD-HHMM.sql.gz | grep -c 'INSERT INTO submissions'  # row count
   ```

   On a fresh DB with no submissions yet, expect the schema lines
   (`CREATE TABLE submissions`, its index, the goose version table)
   and zero `INSERT`s — that's still a valid backup.

The workflow's `test "$(wc -c < ...)" -ge 100` guard rejects an empty
dump (a SQLite `.dump` always emits PRAGMA + BEGIN/COMMIT, so anything
under ~100 bytes means the ssh step itself failed), but it can't catch
a partial dump — spot-check the schema lines are present.

## Step 9 — Confirm the cron is scheduled

GitHub repo → **Actions** → **SQLite nightly backup** → top right
corner should now show a "next run at" timestamp. The schedule is
`17 9 * * *` UTC (09:17 — off-peak for `iad`, off the cron herd).

GitHub Actions cron has a known quirk: it doesn't fire on a public
repo that's seen no activity for 60+ days. Not a concern for active
projects, but worth knowing.

## Restoring from a backup

The full restore procedure — stopping the machine so SQLite releases
its file handle, pushing the rebuilt DB onto the volume via `sftp`,
and smoke-testing — lives in
[`docs/deploy.md` §Backups → Restore](../deploy.md). The shape:

```sh
# 1. Pull the snapshot from R2 and confirm the gzip stream is intact.
rclone copy r2:urbanist-atlas-backups/atlas-2026-05-28-0917.sql.gz .
gunzip -t atlas-2026-05-28-0917.sql.gz

# 2. Reconstruct a fresh DB from the SQL dump.
gunzip -c atlas-2026-05-28-0917.sql.gz | sqlite3 /tmp/atlas.db.new

# 3. Stop the machine, sftp the file onto /data, swap it in, restart.
#    (see deploy.md for the exact flyctl machines stop / sftp / mv steps)
```

**Before restoring into production:** this is a destructive
operation. Confirm you're operating against the right app
(`flyctl status -a urbanist-atlas`) and that the machine is stopped
before you swap the file — moving `/data/atlas.db` under a running
binary leaves the kernel holding the old inode, so submissions
written in between land in the old file and disappear. The Fly volume
snapshot is your last-resort rollback.

## Rotation + maintenance

- **Rotate the R2 token** every 6–12 months. Generate a new one
  (step 5), update `R2_ACCESS_KEY_ID` + `R2_SECRET_ACCESS_KEY` in
  GitHub Secrets (step 6), then delete the old token from the R2
  dashboard.
- **Rotate the Fly token** if a maintainer leaves the project, or
  on the same 6–12 month cadence as the R2 token. Since the backup
  reuses `FLY_API_TOKEN_DEPLOY`, coordinate the rotation with CI.
- **Audit retention** quarterly: spot-check that the R2 bucket has
  ~30 dated objects and no stragglers older than 31 days. If
  stragglers appear, re-check the lifecycle rule (step 4).
- **Test restore** every 6 months against a throwaway DB (restore the
  latest dump into a local `sqlite3` file and run a few queries).
  Untested backups are half-backups.

## Troubleshooting

**Workflow fails at "Snapshot SQLite via flyctl ssh" with `Error: SSH
error`.** The Fly token doesn't have SSH console permission for
`urbanist-atlas`. Regenerate with `flyctl auth token` from an account
that does, or scope the dashboard token to include `ssh:console`.

**Snapshot step fails with `sqlite3: not found` or the dump is
empty.** The inner `sh -eu -o pipefail` is there precisely to surface
this — a non-zero exit propagates instead of producing a well-formed
empty gzip. Check that `/data/atlas.db` exists on the machine
(`flyctl ssh console -a urbanist-atlas -C 'ls -l /data'`); if the
volume isn't mounted, the app didn't boot with submissions enabled.

**Workflow fails at "Upload to R2" with `403` / `Access Denied`.**
Either the R2 API token is scoped to a different bucket, or the
permissions were set to "Object Read" instead of "Object Read &
Write". Recreate the token (step 5) and update both secrets.

**Upload fails with `Could not connect to the endpoint URL`.**
`CF_ACCOUNT_ID` is wrong or has stray whitespace. Re-copy from the
dashboard (step 2) and update the secret.

**Backup file is suspiciously small.** The `test -s` / `≥100 byte`
guard catches a fully-empty dump but not a partial one. Spot-check
that the expected `CREATE TABLE submissions` line is present (step 8).

**Bucket has objects older than 30 days.** The lifecycle rule didn't
apply — re-check step 4. Cloudflare's lifecycle sweep runs daily, so
allow 24h between adding the rule and expecting deletions.
