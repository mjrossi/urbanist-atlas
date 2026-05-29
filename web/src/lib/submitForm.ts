/**
 * Submit-form helpers — the schema + payload composition for
 * `/submit`. Lives here (not inline in Submit.tsx) so the JSX file
 * stays focused on layout. Exports two payload composers:
 *
 *  - {@link buildNewSubmissionRequest}: the live `POST /api/v1/submissions`
 *    payload (Phase 2 submission queue — slice β).
 *  - {@link buildIssueBody} + {@link buildIssueUrl}: pre-filled GitHub
 *    issue, kept as a fallback when the API is unreachable (5xx) so
 *    a determined submitter still has a path through.
 */
import type { NewSubmissionRequest } from './api.ts';

export type SubmissionType = 'new' | 'correction' | 'removal';
export type Affiliation = 'none' | 'member' | 'other';

export interface SubmitForm {
  type: SubmissionType;
  name: string;
  website: string;
  region: string;
  oneLineDesc: string;
  why: string;
  sources: string;
  affiliation: Affiliation;
  contact: string;
}

export const SUBMIT_FORM_DEFAULTS: SubmitForm = {
  type: 'new',
  name: '',
  website: '',
  region: '',
  oneLineDesc: '',
  why: '',
  sources: '',
  affiliation: 'none',
  contact: '',
};

/** Title prefix that distinguishes the three submission flavors in the GitHub issue list. */
export function titlePrefix(type: SubmissionType): string {
  switch (type) {
    case 'new':
      return '[New org]';
    case 'correction':
      return '[Correction]';
    case 'removal':
      return '[Removal]';
  }
}

function check(condition: boolean): 'x' | ' ' {
  return condition ? 'x' : ' ';
}

/**
 * Composes the Markdown body of the pre-filled GitHub issue from a
 * SubmitForm. The shape mirrors
 * `.github/ISSUE_TEMPLATE/org_correction_or_addition.md` so editorial
 * review reads the same on both manual and form-driven submissions.
 */
export function buildIssueBody(form: SubmitForm): string {
  const why = form.why.trim() || '_Not provided._';
  const sources = form.sources.trim() || '_Not provided._';
  const contact = form.contact.trim();
  const submittedSuffix = contact ? ` by ${contact}` : '';
  return `## Type

- [${check(form.type === 'new')}] New organization to add
- [${check(form.type === 'correction')}] Correction to an existing entry
- [${check(form.type === 'removal')}] Removal request (e.g., org has shut down or rebranded)

## Organization

- **Name:** ${form.name}
- **Primary website:** ${form.website}
- **Region served:** ${form.region}
- **One-line description of what they actually do:** ${form.oneLineDesc}

## Why this org belongs

${why}

## Sources

${sources}

## Disclosure

- [${check(form.affiliation === 'none')}] I have no affiliation with this organization.
- [${check(form.affiliation === 'member')}] I am a member, volunteer, or staff of this organization.
- [${check(form.affiliation === 'other')}] Other.

---

_Submitted via the urbanistatlas.com submit form${submittedSuffix}._
`;
}

/**
 * Composes the URL of a pre-filled GitHub issue from a SubmitForm.
 * Same shape as the old direct-to-GitHub flow — retained for the
 * "API unreachable" fallback link the Submit page surfaces on 5xx.
 */
export function buildIssueUrl(form: SubmitForm): string {
  const title = `${titlePrefix(form.type)} ${form.name}`;
  const body = buildIssueBody(form);
  const params = new URLSearchParams({ title, body });
  return `https://github.com/mjrossi/urbanist-atlas/issues/new?${params.toString()}`;
}

/**
 * Composes the API payload for `POST /api/v1/submissions`. The wire
 * contract carries only the org fields + three optional submitter
 * fields; the form's extra editorial context (why / sources /
 * affiliation / correction or removal request) is folded into a
 * Markdown-formatted `submitter_note` so moderators see the same
 * structured context they would on the old GitHub-issue path.
 *
 * Region matching: the form's `region` is a free-form text input
 * (city / metro / state). The wire `region_slugs` array expects
 * canonical slugs. Until a region-picker autocomplete lands, we
 * carry the raw text through in `submitter_note` and submit an
 * empty slug list IF the user typed something the API won't
 * accept verbatim. For the common-case match (e.g. "brooklyn-ny"),
 * we trust the user. Moderators finalize slugs in PR review.
 */
export function buildNewSubmissionRequest(form: SubmitForm): NewSubmissionRequest {
  const regionRaw = form.region.trim();
  const submitterNote = buildSubmitterNote(form);
  return {
    payload: {
      name: form.name.trim(),
      short_desc: form.oneLineDesc.trim(),
      website_url: form.website.trim(),
      tags: [],
      region_slugs: regionToSlugs(regionRaw),
    },
    submitter_note: submitterNote,
    ...(form.contact.trim() ? maybeContact(form.contact) : {}),
  };
}

/**
 * Splits a free-form region input into candidate slugs. The form
 * collects something like "Brooklyn, NY" or "brooklyn-ny, nyc-metro";
 * the API rejects unknown slugs with a 400. Strategy: take the user's
 * text, split on commas, lowercase, replace spaces with hyphens. If
 * the result yields a slug the API recognizes, great; if not, the
 * 400 surfaces a clear field-level error and moderators can finalize
 * the slug in PR review when they merge the auto-PR.
 */
function regionToSlugs(input: string): string[] {
  if (!input) return [];
  return input
    .split(',')
    .map((s) =>
      s
        .trim()
        .toLowerCase()
        .replace(/\s+/g, '-')
        .replace(/[^a-z0-9-]/g, '')
        .replace(/-+/g, '-')
        .replace(/^-+|-+$/g, ''),
    )
    .filter(Boolean);
}

function maybeContact(raw: string): Partial<NewSubmissionRequest> {
  // An @handle isn't a valid email; the API's optional email field is
  // typed as `format: email`. Carry @handles in submitter_name so
  // they're not lost.
  const trimmed = raw.trim();
  if (trimmed.includes('@') && !trimmed.startsWith('@')) {
    return { submitter_email: trimmed };
  }
  return { submitter_name: trimmed };
}

function buildSubmitterNote(form: SubmitForm): string {
  const lines: string[] = [];
  lines.push(`Submission type: ${labelForType(form.type)}.`);
  if (form.region.trim()) {
    lines.push(`Region served (raw): ${form.region.trim()}`);
  }
  if (form.why.trim()) {
    lines.push('', '### Why this org belongs', '', form.why.trim());
  }
  if (form.sources.trim()) {
    lines.push('', '### Sources', '', form.sources.trim());
  }
  lines.push('', `Affiliation: ${labelForAffiliation(form.affiliation)}.`);
  return lines.join('\n');
}

function labelForType(t: SubmissionType): string {
  switch (t) {
    case 'new':
      return 'new organization';
    case 'correction':
      return 'correction to an existing entry';
    case 'removal':
      return 'removal request';
  }
}

function labelForAffiliation(a: Affiliation): string {
  switch (a) {
    case 'none':
      return 'none';
    case 'member':
      return 'member / volunteer / staff';
    case 'other':
      return 'other';
  }
}
