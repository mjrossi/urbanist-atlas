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
 * Region: `region_slugs` is optional on the wire. The form's region
 * input is free-form text — a typical submitter writes "Brooklyn, NY"
 * or "Seattle", not a canonical slug like `nyc-tri-state` — so we
 * always send an empty array and carry the raw text through in
 * `submitter_note`. Editors finalize the canonical slug during PR
 * review.
 */
export function buildNewSubmissionRequest(form: SubmitForm): NewSubmissionRequest {
  const submitterNote = buildSubmitterNote(form);
  return {
    payload: {
      name: form.name.trim(),
      short_desc: shortDescForWire(form),
      website_url: form.website.trim(),
      tags: [],
      region_slugs: [],
    },
    submitter_note: submitterNote,
    ...(form.contact.trim() ? maybeContact(form.contact) : {}),
  };
}

/**
 * For `new` submissions the form collects a real one-line
 * description. For corrections and removals there is no new
 * description to write — the existing entry's copy is the subject of
 * the request — so we mint a synthetic short_desc that points
 * moderators at submitter_note for the full context. Keeps the wire
 * shape stable (short_desc is a required field) without asking the
 * submitter to invent placeholder copy.
 */
function shortDescForWire(form: SubmitForm): string {
  if (form.type === 'new') return form.oneLineDesc.trim();
  if (form.type === 'correction') return '(correction request — see editor note)';
  return '(removal request — see editor note)';
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
  // For correction/removal the Region input is hidden, so form.region
  // is the SUBMIT_FORM_DEFAULTS empty-string and this line drops out
  // automatically. New-org submissions surface the user's raw text
  // here for editorial slug-finalization.
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
