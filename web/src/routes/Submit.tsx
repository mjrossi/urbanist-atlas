import { useForm } from 'react-hook-form';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

type SubmissionType = 'new' | 'correction' | 'removal';
type Affiliation = 'none' | 'member' | 'other';

interface SubmitForm {
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

/**
 * `/submit` — composes a pre-filled GitHub issue from the form values
 * and opens it in a new tab. The Phase 2 in-app submission queue is
 * the eventual destination; this form is the staffed channel until
 * then. See the issue template at
 * `.github/ISSUE_TEMPLATE/org_correction_or_addition.md` for the
 * canonical section structure mirrored in `buildIssueBody`.
 */
export function Submit() {
  useDocumentTitle('Submit an organization — Urbanist Atlas');

  const {
    register,
    handleSubmit,
    formState: { isValid, errors },
  } = useForm<SubmitForm>({
    mode: 'onBlur',
    defaultValues: {
      type: 'new',
      name: '',
      website: '',
      region: '',
      oneLineDesc: '',
      why: '',
      sources: '',
      affiliation: 'none',
      contact: '',
    },
  });

  const onValid = (form: SubmitForm) => {
    const title = `${titlePrefix(form.type)} ${form.name}`;
    const body = buildIssueBody(form);
    // GitHub silently drops `body=` when `template=` is also present,
    // so we deliberately omit the template param and ship the full
    // pre-filled body instead.
    const url = `https://github.com/mjrossi/urbanist-atlas/issues/new?title=${encodeURIComponent(title)}&body=${encodeURIComponent(body)}`;
    window.open(url, '_blank', 'noopener');
  };

  return (
    <div className="page">
      <header className="page-header">
        <h1>Submit an organization.</h1>
        <p>
          <em>
            Public submissions flow through GitHub today; a moderated in-app
            queue ships with the Phase 2 API-key program. This form composes a
            pre-filled GitHub issue and opens it in a new tab — review what’s
            there, then file it.
          </em>
        </p>
      </header>

      <section>
        <form onSubmit={handleSubmit(onValid)} noValidate>
          <fieldset className="form-field">
            <legend>Type</legend>
            <label>
              <input type="radio" value="new" {...register('type', { required: true })} />
              {' '}New organization to add
            </label>
            <label>
              <input type="radio" value="correction" {...register('type', { required: true })} />
              {' '}Correction to an existing entry
            </label>
            <label>
              <input type="radio" value="removal" {...register('type', { required: true })} />
              {' '}Removal request (e.g., org has shut down or rebranded)
            </label>
          </fieldset>

          <div className="form-field">
            <label htmlFor="submit-name">Organization name</label>
            <input
              id="submit-name"
              type="text"
              {...register('name', { required: 'Required' })}
            />
            {errors.name ? <span role="alert">{errors.name.message}</span> : null}
          </div>

          <div className="form-field">
            <label htmlFor="submit-website">Primary website</label>
            <input
              id="submit-website"
              type="url"
              {...register('website', { required: 'Required' })}
            />
            {errors.website ? <span role="alert">{errors.website.message}</span> : null}
          </div>

          <div className="form-field">
            <label htmlFor="submit-region">
              Region served (city / county / metro / state — be specific)
            </label>
            <input
              id="submit-region"
              type="text"
              {...register('region', { required: 'Required' })}
            />
            {errors.region ? <span role="alert">{errors.region.message}</span> : null}
          </div>

          <div className="form-field">
            <label htmlFor="submit-oneline">
              One-line description of what they actually do
            </label>
            <textarea
              id="submit-oneline"
              rows={2}
              {...register('oneLineDesc', { required: 'Required' })}
            />
            {errors.oneLineDesc ? (
              <span role="alert">{errors.oneLineDesc.message}</span>
            ) : null}
          </div>

          <div className="form-field">
            <label htmlFor="submit-why">
              Why this org belongs (recent campaigns, who they organize, who
              they push)
            </label>
            <textarea id="submit-why" rows={3} {...register('why')} />
          </div>

          <div className="form-field">
            <label htmlFor="submit-sources">
              Sources (news, social, prior wins)
            </label>
            <textarea id="submit-sources" rows={3} {...register('sources')} />
          </div>

          <fieldset className="form-field">
            <legend>Disclosure</legend>
            <label>
              <input
                type="radio"
                value="none"
                {...register('affiliation', { required: true })}
              />
              {' '}I have no affiliation
            </label>
            <label>
              <input
                type="radio"
                value="member"
                {...register('affiliation', { required: true })}
              />
              {' '}I am a member, volunteer, or staff
            </label>
            <label>
              <input
                type="radio"
                value="other"
                {...register('affiliation', { required: true })}
              />
              {' '}Other
            </label>
          </fieldset>

          <div className="form-field">
            <label htmlFor="submit-contact">
              Your email or @handle (optional, helps if the editor has follow-up
              questions)
            </label>
            <input id="submit-contact" type="text" {...register('contact')} />
          </div>

          <button type="submit" disabled={!isValid}>
            Open as a GitHub issue →
          </button>
        </form>
      </section>
    </div>
  );
}

function titlePrefix(type: SubmissionType): string {
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

function buildIssueBody(form: SubmitForm): string {
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
