/**
 * The `/submit` form's shape, validation and payloads: the API request
 * ({@link buildNewSubmissionRequest}) and a pre-filled GitHub issue
 * ({@link buildIssueUrl}), the fallback when the API is down.
 */
import { ApiError, type NewSubmissionRequest } from './api.ts';

export type SubmissionType = 'new' | 'correction' | 'removal';
export type Affiliation = 'none' | 'member' | 'other';

export interface SubmitForm {
  type: SubmissionType;
  name: string;
  website: string;
  /** Region slugs picked from the type-ahead. */
  regionSlugs: string[];
  /** Free-text region for when the type-ahead has no match; goes in the note. */
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
  regionSlugs: [],
  region: '',
  oneLineDesc: '',
  why: '',
  sources: '',
  affiliation: 'none',
  contact: '',
};

/** The GitHub issue title prefix for a submission type. */
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
 * The region fields as they apply to this submission type. react-hook-form
 * keeps a hidden field's value, so a region entered before switching away
 * from "new" must not ride along on a correction or removal.
 */
function regionFieldsFor(form: SubmitForm): { slugs: string[]; freeText: string } {
  if (form.type !== 'new') return { slugs: [], freeText: '' };
  return { slugs: form.regionSlugs, freeText: form.region.trim() };
}

/**
 * True when the submission failed for a reason the submitter can't fix
 * from the form (a 5xx or no response at all), so the GitHub-issue
 * fallback should be offered.
 */
export function isServerOrUnreachable(err: Error): boolean {
  return !(err instanceof ApiError) || err.status === 0 || err.status >= 500;
}

/**
 * The human reference for a submission: the last 8 hex digits of its
 * UUIDv7. The leading digits are a millisecond timestamp, so they repeat
 * across submissions made close together.
 */
export function shortSubmissionId(id: string): string {
  return id.replace(/-/g, '').slice(-8);
}

/**
 * The pre-filled GitHub issue's Markdown body. Keep it in step with
 * `.github/ISSUE_TEMPLATE/org_correction_or_addition.md`.
 */
export function buildIssueBody(form: SubmitForm): string {
  const why = form.why.trim() || '_Not provided._';
  const sources = form.sources.trim() || '_Not provided._';
  const contact = form.contact.trim();
  const submittedSuffix = contact ? ` by ${contact}` : '';
  const { slugs, freeText } = regionFieldsFor(form);
  const regionServed = [...slugs, freeText].filter(Boolean).join(', ');
  const oneLineDesc = form.type === 'new' ? form.oneLineDesc : '';
  return `## Type

- [${check(form.type === 'new')}] New organization to add
- [${check(form.type === 'correction')}] Correction to an existing entry
- [${check(form.type === 'removal')}] Removal request (e.g., org has shut down or rebranded)

## Organization

- **Name:** ${form.name}
- **Primary website:** ${form.website}
- **Region served:** ${regionServed}
- **One-line description of what they actually do:** ${oneLineDesc}

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

/** The URL of a pre-filled GitHub issue for the form. */
export function buildIssueUrl(form: SubmitForm): string {
  const title = `${titlePrefix(form.type)} ${form.name}`;
  const body = buildIssueBody(form);
  const params = new URLSearchParams({ title, body });
  return `https://github.com/mjrossi/urbanist-atlas/issues/new?${params.toString()}`;
}

/**
 * The `POST /api/v1/submissions` body. The wire has no fields for the
 * form's editorial context (type, why, sources, affiliation), so it goes
 * in `submitter_note` as Markdown.
 */
export function buildNewSubmissionRequest(form: SubmitForm): NewSubmissionRequest {
  const submitterNote = buildSubmitterNote(form);
  return {
    payload: {
      name: form.name.trim(),
      short_desc: shortDescForWire(form),
      website_url: form.website.trim(),
      region_slugs: regionFieldsFor(form).slugs,
    },
    submitter_note: submitterNote,
    ...(form.contact.trim() ? maybeContact(form.contact) : {}),
  };
}

/**
 * `short_desc` is required, but a correction or removal has no new
 * description, so those send a placeholder pointing at the note.
 */
function shortDescForWire(form: SubmitForm): string {
  if (form.type === 'new') return form.oneLineDesc.trim();
  if (form.type === 'correction') return '(correction request — see editor note)';
  return '(removal request — see editor note)';
}

// Anything the API wouldn't accept as an email (an @handle, or an
// x@y with no dot in the domain) still reaches the editor as
// submitter_name.
function maybeContact(raw: string): Partial<NewSubmissionRequest> {
  const trimmed = raw.trim();
  if (looksLikeEmail(trimmed)) {
    return { submitter_email: trimmed };
  }
  return { submitter_name: trimmed };
}

export function buildSubmitterNote(form: SubmitForm): string {
  const lines: string[] = [];
  lines.push(`Submission type: ${labelForType(form.type)}.`);
  const { slugs, freeText } = regionFieldsFor(form);
  if (slugs.length > 0) {
    lines.push(`Regions (picked): ${slugs.join(', ')}.`);
  }
  if (freeText) {
    lines.push(`Region not found — new-region request: ${freeText}`);
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

// The API's caps on submission fields. Character caps count code points
// (the API counts runes, not UTF-16 units); the URL and email caps count
// UTF-8 bytes.
export const MAX_NAME_CHARS = 200;
export const MAX_SHORT_DESC_CHARS = 1000;
export const MAX_URL_BYTES = 500;
export const MAX_SUBMITTER_NAME_CHARS = 200;
export const MAX_SUBMITTER_EMAIL_BYTES = 320;
export const MAX_SUBMITTER_NOTE_CHARS = 2000;

/** Length in code points, not UTF-16 units. */
export function charCount(s: string): number {
  return Array.from(s).length;
}

function utf8Length(s: string): number {
  return new TextEncoder().encode(s).length;
}

function tooLong(max: number): string {
  return `Keep it to ${max.toLocaleString('en-US')} characters or fewer.`;
}

/**
 * The API's loose email shape: one '@' with text on both sides and a
 * dot in the domain.
 */
export function looksLikeEmail(addr: string): boolean {
  const at = addr.indexOf('@');
  if (at <= 0 || at !== addr.lastIndexOf('@') || at === addr.length - 1) return false;
  return addr.slice(at + 1).includes('.');
}

/** Inline error for the organization name, or undefined when it passes. */
export function nameError(raw: string): string | undefined {
  return charCount(raw.trim()) > MAX_NAME_CHARS ? tooLong(MAX_NAME_CHARS) : undefined;
}

/** Inline error for the one-line description, or undefined when it passes. */
export function shortDescError(raw: string): string | undefined {
  return charCount(raw.trim()) > MAX_SHORT_DESC_CHARS
    ? tooLong(MAX_SHORT_DESC_CHARS)
    : undefined;
}

/**
 * Inline error for the website, or undefined when it passes or is empty
 * (the required rule covers that). It needs an absolute http(s) URL with
 * a host, split the way the API's URL parser splits it: fragment and
 * query off first, then the authority after `//`, then any userinfo.
 */
export function websiteError(raw: string): string | undefined {
  const url = raw.trim();
  if (!url) return undefined;
  if (utf8Length(url) > MAX_URL_BYTES) return tooLong(MAX_URL_BYTES);
  const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(url);
  const name = scheme?.[1]?.toLowerCase();
  if (!scheme || (name !== 'http' && name !== 'https')) {
    return 'Start with http:// or https://.';
  }
  const rest = url.slice(scheme[0].length).split('#')[0]?.split('?')[0] ?? '';
  const authority = rest.startsWith('//') ? (rest.slice(2).split('/')[0] ?? '') : '';
  const host = authority.slice(authority.lastIndexOf('@') + 1);
  return host ? undefined : 'Include the site’s address after the https://.';
}

/** Inline error for the contact field, or undefined when it passes. */
export function contactError(raw: string): string | undefined {
  const contact = raw.trim();
  if (looksLikeEmail(contact)) {
    return utf8Length(contact) > MAX_SUBMITTER_EMAIL_BYTES
      ? tooLong(MAX_SUBMITTER_EMAIL_BYTES)
      : undefined;
  }
  return charCount(contact) > MAX_SUBMITTER_NAME_CHARS
    ? tooLong(MAX_SUBMITTER_NAME_CHARS)
    : undefined;
}

/** The free-text fields that feed submitter_note, in page order. */
export type NoteField = 'region' | 'why' | 'sources';

/**
 * Inline error for one of the fields composed into submitter_note, or
 * undefined. When the note is over the cap, the error lands on the
 * first field, in page order, whose text takes the note past it.
 */
export function noteFieldError(field: NoteField, form: SubmitForm): string | undefined {
  const over = charCount(buildSubmitterNote(form)) - MAX_SUBMITTER_NOTE_CHARS;
  if (over <= 0) return undefined;
  const order: NoteField[] = ['region', 'why', 'sources'];
  const partial: SubmitForm = { ...form, region: '', why: '', sources: '' };
  let culprit: NoteField = 'sources';
  for (const f of order) {
    partial[f] = form[f];
    if (charCount(buildSubmitterNote(partial)) > MAX_SUBMITTER_NOTE_CHARS) {
      culprit = f;
      break;
    }
  }
  if (culprit !== field) return undefined;
  const noun = over === 1 ? 'character' : 'characters';
  return `Trim ${over.toLocaleString('en-US')} ${noun}. Your notes to the editor share a ${MAX_SUBMITTER_NOTE_CHARS.toLocaleString('en-US')}-character limit.`;
}
