import { useMutation } from '@tanstack/react-query';
import { Controller, type FieldPath, useForm, useWatch } from 'react-hook-form';
import { Link } from 'react-router';

import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { RegionCombobox } from '../components/RegionCombobox.tsx';
import { ApiError, createSubmission, type Submission } from '../lib/api.ts';
import {
  buildIssueUrl,
  buildNewSubmissionRequest,
  type SubmissionType,
  SUBMIT_FORM_DEFAULTS,
  type SubmitForm,
} from '../lib/submitForm.ts';
import { useCooldown, useCountdown } from '../lib/timers.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

// Brief lockout after a successful submit so a triple-click can't
// trigger duplicate POSTs.
const SUBMIT_COOLDOWN_MS = 1500;

// Map from API field names (snake-case wire shape, see
// SubmissionPayload + NewSubmissionRequest in openapi.yaml and the
// API-side `errors` extension keys emitted by
// seedfiles.ValidateSubmissionPayload) to the matching
// react-hook-form field names in SubmitForm. Server fields the form
// doesn't render are intentionally absent — they fall through to the
// top-level `detail` banner instead of being silently swallowed.
//
// Caveat: `region_slugs` and `short_desc` map to inputs that the form
// HIDES for correction/removal submissions. In practice both API
// errors are unreachable for those types (the SPA sends `[]` for
// region_slugs and a short synthetic placeholder for short_desc), so
// the hidden-field path doesn't surface. If a future API change makes
// either reachable, render the top-level banner alongside the
// invisible field error.
const FIELD_NAME_MAP: Readonly<Record<string, FieldPath<SubmitForm>>> = {
  name: 'name',
  website_url: 'website',
  region_slugs: 'regionSlugs',
  short_desc: 'oneLineDesc',
  contact_url: 'contact',
  submitter_email: 'contact',
  submitter_name: 'contact',
  submitter_note: 'why',
};

/**
 * Per-submission-type copy. The three flavors share one layout but swap
 * hints, labels, and placeholders; a keyed table keeps the variants
 * side-by-side instead of five parallel ternary chains.
 */
interface SubmissionCopy {
  nameHint: string;
  editorialLabel: string;
  editorialHint: string;
  editorialPlaceholder: string;
  sourcesHint: string;
}

const SUBMISSION_COPY: Record<SubmissionType, SubmissionCopy> = {
  new: {
    nameHint: 'As it appears on their site, not what locals call them.',
    editorialLabel: 'Why this org belongs',
    editorialHint:
      'What have they worked on recently? Who do they organize? Who do they push? Concrete examples help us more than superlatives.',
    editorialPlaceholder:
      'Defended the local fare against a hike; runs candidate forums.',
    sourcesHint:
      "Links to news coverage, their social accounts, or campaigns they've worked on. One per line.",
  },
  correction: {
    nameHint: 'The entry you want corrected. Use the name as it appears in the Atlas.',
    editorialLabel: 'What needs correcting',
    editorialHint: 'What is incorrect, what should it say instead, and how do you know?',
    editorialPlaceholder:
      'Listed as rail-focused, but they pivoted to bus rapid transit in 2025.',
    sourcesHint:
      "Anything that shows what's changed: a press release, an organizational chart, a recent article.",
  },
  removal: {
    nameHint: 'The entry you want removed. Use the name as it appears in the Atlas.',
    editorialLabel: 'Why this organization should be removed',
    editorialHint:
      'Shut down, merged, domain hijacked — what happened, and roughly when?',
    editorialPlaceholder: 'Domain expired in March 2025; last post was October 2024.',
    sourcesHint:
      'Anything that confirms the change: a dead site, an archived final post, a merger announcement.',
  },
};

interface SubmitErrorView {
  isRateLimited: boolean;
  isServerErr: boolean;
  showTopLevelValidationErr: boolean;
  validationMessage: string;
  unmappedFieldErrors: { field: string; message: string }[];
}

/**
 * Collapse a failed submission into the flags the error banners need.
 * Pulled out of the component so the precedence between per-field,
 * top-level-validation, rate-limit, and server-error states stays
 * readable (and unit-testable).
 *
 * `unmappedFieldErrors` are API field errors with no visible home in the
 * form — an unmapped API key, or one mapped to a field this submission
 * type hides (e.g. `region_slugs`/`short_desc` on a correction/removal,
 * or `contact`/`why`, which have no inline error slot). They'd otherwise
 * be swallowed, so the top-level banner surfaces them.
 */
function deriveSubmitError(
  err: ApiError | null,
  visibleFieldErrorSlots: ReadonlySet<FieldPath<SubmitForm>>,
): SubmitErrorView {
  if (!err) {
    return {
      isRateLimited: false,
      isServerErr: false,
      showTopLevelValidationErr: false,
      validationMessage: '',
      unmappedFieldErrors: [],
    };
  }

  const isValidationErr = err.status === 400 || err.status === 422;
  const unmappedFieldErrors = err.fieldErrors
    ? Object.entries(err.fieldErrors)
        .filter(([apiField]) => {
          const formField = FIELD_NAME_MAP[apiField];
          return !formField || !visibleFieldErrorSlots.has(formField);
        })
        .map(([field, message]) => ({ field, message }))
    : [];
  const hasMappedFieldErrors =
    !!err.fieldErrors && Object.keys(err.fieldErrors).length > 0;

  return {
    isRateLimited: err.status === 429,
    isServerErr: err.status >= 500,
    showTopLevelValidationErr:
      isValidationErr && (!hasMappedFieldErrors || unmappedFieldErrors.length > 0),
    validationMessage: err.problem?.detail ?? err.message,
    unmappedFieldErrors,
  };
}

/**
 * The inline `field-error` slot rendered under a form control when
 * react-hook-form has an error for it. Renders nothing when the field
 * is clean — one place for the role="alert" + class contract.
 */
function FieldError({ error }: { error?: { message?: string } }) {
  return error ? (
    <span className="field-error" role="alert">
      {error.message}
    </span>
  ) : null;
}

/**
 * `/submit` — accepts an org tip, POSTs it to `/api/v1/submissions`,
 * and shows a "received" confirmation card with the short submission
 * id moderators will reference in the auto-PR. On API failure we fall
 * back to a manual GitHub-issue link so the submitter isn't stranded.
 *
 * The form schema is shared with the GitHub-issue path so a future
 * regression in the API surface doesn't lose data the submitter
 * already typed.
 */
export function Submit() {
  useDocumentTitle('Submit an organization — Urbanist Atlas');

  const {
    register,
    handleSubmit,
    formState: { isValid, errors },
    setError,
    control,
    trigger,
    reset,
  } = useForm<SubmitForm>({
    mode: 'onBlur',
    defaultValues: SUBMIT_FORM_DEFAULTS,
  });

  // The form adapts to the submission type. For "new" we ask for the
  // full org dossier; for "correction" and "removal" the org already
  // exists in the Atlas, so we collect a smaller identification +
  // context payload. Wire-shape stays the same — see
  // submitForm.ts:buildNewSubmissionRequest for how the missing fields
  // are folded into the API contract.
  //
  // useWatch (vs the destructured watch()) is the memoization-safe
  // subscription per react-hook-form's API guidance; `defaultValue`
  // covers the brief window before defaultValues propagate so we render
  // the new-org variant rather than nothing.
  const submissionType = useWatch({ control, name: 'type', defaultValue: 'new' });
  const isNewOrg = submissionType === 'new';
  const copy = SUBMISSION_COPY[submissionType];

  // Brief post-submit lockout (stops triple-click dupes) and the 429
  // Retry-After countdown; both disable the submit button until they
  // elapse. retryAfter is (re)armed from ApiError.retryAfterSeconds in
  // the mutation's onError below.
  const [cooldown, startCooldown] = useCooldown(SUBMIT_COOLDOWN_MS);
  const [retryAfter, setRetryAfter] = useCountdown();

  const mutation = useMutation<Submission, ApiError, SubmitForm>({
    mutationFn: (form) => createSubmission(buildNewSubmissionRequest(form)),
    onSuccess: () => {
      startCooldown();
    },
    onError: (err) => {
      // Per-field validation errors (W1.4): hand each known field
      // to react-hook-form so the input shows its own error. The
      // top-level fallback below renders when no field map matched.
      if (err.fieldErrors) {
        for (const [apiField, message] of Object.entries(err.fieldErrors)) {
          const formField = FIELD_NAME_MAP[apiField];
          if (formField) {
            setError(formField, { type: 'server', message }, { shouldFocus: false });
          }
        }
      }
      // Rate-limit countdown (W1.3): start the per-second timer
      // from the server-provided Retry-After. Static copy still
      // renders when the header is missing.
      if (err.status === 429 && err.retryAfterSeconds && err.retryAfterSeconds > 0) {
        setRetryAfter(err.retryAfterSeconds);
      }
    },
  });

  const onValid = (form: SubmitForm) => {
    if (cooldown || mutation.isPending || retryAfter > 0) return;
    mutation.mutate(form);
  };

  // Subscribe to every form value so the GitHub-issue fallback link
  // (rendered on 5xx) stays current when the user edits between
  // seeing the error and clicking the link. `getValues()` inside
  // render would snapshot at render time and go stale on subsequent
  // keystrokes (the form uses uncontrolled inputs via register, so
  // keystrokes don't otherwise re-render). Must be called before the
  // mutation.isSuccess early return below to keep hook order stable.
  const fallbackValues = useWatch({ control });

  if (mutation.isSuccess) {
    return (
      <SubmissionReceived
        submission={mutation.data}
        onAnother={() => {
          // Clear react-hook-form's field state back to
          // SUBMIT_FORM_DEFAULTS, then reset the mutation so the form
          // (not the receipt) re-renders. mutation.reset() alone leaves
          // the previously-typed values behind — Submit stays mounted,
          // so useForm is never re-initialized.
          reset();
          mutation.reset();
        }}
      />
    );
  }

  const submitErr = mutation.error;
  // Which inline `field-error` slots actually render for this submission
  // type — drives which API field errors have a visible home; the rest
  // surface in the top-level banner (see deriveSubmitError).
  const visibleFieldErrorSlots = new Set<FieldPath<SubmitForm>>(
    isNewOrg ? ['name', 'website', 'regionSlugs', 'oneLineDesc'] : ['name', 'website'],
  );
  const {
    isRateLimited,
    isServerErr,
    showTopLevelValidationErr,
    validationMessage,
    unmappedFieldErrors,
  } = deriveSubmitError(submitErr, visibleFieldErrorSlots);
  const issueFallbackUrl = isServerErr
    ? buildIssueUrl({ ...SUBMIT_FORM_DEFAULTS, ...fallbackValues })
    : '';

  return (
    <>
      <PageBreadcrumb
        prefix={[{ label: 'Atlas', to: '/' }]}
        current="Submissions"
        meta="Open year-round"
      />

      <div className="spread mt-48">
        <div>
          <div className="lede">
            <div className="eyebrow">
              № I — Submissions desk
              <span className="eyebrow-rule" />
            </div>
            <h1>
              File a tip for <span className="accent">the editors.</span>
            </h1>
            <p className="deck">
              Know an advocacy group that should be in the Atlas? Spotted a stale entry?
              Tell us here. Your submission goes straight to the editorial queue and we
              open a pull request against the public dataset when we accept it.
            </p>
          </div>

          <form onSubmit={handleSubmit(onValid)} noValidate className="slip">
            <div className="slip-head">
              <div className="stamp-l">Submission slip</div>
              <div className="title">Tip sheet</div>
              <div className="stamp-r">For editorial review</div>
            </div>

            <fieldset className="field stacked">
              <div>
                <legend className="field-label">
                  Type of submission
                  <span className="required">*</span>
                </legend>
              </div>
              <div className="choices">
                <label className="choice">
                  <input
                    type="radio"
                    value="new"
                    {...register('type', { required: true })}
                  />
                  <span className="text">
                    A new organization to index
                    <span className="sub">
                      A group not yet in the Atlas — local or regional.
                    </span>
                  </span>
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    value="correction"
                    {...register('type', { required: true })}
                  />
                  <span className="text">
                    A correction to an existing entry
                    <span className="sub">
                      Wrong description, broken link, outdated leadership.
                    </span>
                  </span>
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    value="removal"
                    {...register('type', { required: true })}
                  />
                  <span className="text">
                    A removal request
                    <span className="sub">
                      The organization has shut down, rebranded, or merged.
                    </span>
                  </span>
                </label>
              </div>
            </fieldset>

            <div className="field">
              <div>
                <label htmlFor="submit-name" className="field-label">
                  Organization name
                  <span className="required">*</span>
                  <span className="hint">{copy.nameHint}</span>
                </label>
              </div>
              <div>
                <input
                  id="submit-name"
                  type="text"
                  className="input"
                  placeholder="e.g. Transit Riders Union"
                  {...register('name', { required: 'Required' })}
                />
                <FieldError error={errors.name} />
              </div>
            </div>

            <div className="field">
              <div>
                <label htmlFor="submit-website" className="field-label">
                  Primary website
                  <span className="required">*</span>
                  <span className="hint">
                    The organization&rsquo;s own site, not a news article about them.
                  </span>
                </label>
              </div>
              <div>
                <input
                  id="submit-website"
                  type="url"
                  className="input"
                  placeholder="https://"
                  {...register('website', { required: 'Required' })}
                />
                <FieldError error={errors.website} />
              </div>
            </div>

            {isNewOrg ? (
              <>
                <div className="field">
                  <div>
                    <label htmlFor="submit-region" className="field-label">
                      Region served
                      <span className="required">*</span>
                      <span className="hint" id="submit-region-hint">
                        Start typing a city, metro, or borough and pick from the list. Add
                        more than one if they serve several.
                      </span>
                    </label>
                  </div>
                  <div>
                    <Controller
                      control={control}
                      name="regionSlugs"
                      rules={{
                        validate: (val, formValues) =>
                          val.length > 0 ||
                          formValues.region.trim().length > 0 ||
                          'Pick at least one region, or describe a new one below.',
                      }}
                      render={({ field, fieldState }) => (
                        <RegionCombobox
                          id="submit-region"
                          value={field.value}
                          onChange={(next) => {
                            field.onChange(next);
                            // Re-validate so the cross-field rule (and the
                            // submit button's isValid gate) update the
                            // instant a region is picked or cleared.
                            void trigger('regionSlugs');
                          }}
                          onBlur={field.onBlur}
                          describedById="submit-region-hint"
                          invalid={!!fieldState.error}
                        />
                      )}
                    />
                    <FieldError error={errors.regionSlugs} />
                  </div>
                </div>

                <div className="field stacked">
                  <div>
                    <label htmlFor="submit-region-new" className="field-label">
                      Can&rsquo;t find your region?
                      <span className="hint inline">
                        Tell us the city or area and we&rsquo;ll add it. Skip this if you
                        picked one above.
                      </span>
                    </label>
                  </div>
                  <input
                    id="submit-region-new"
                    type="text"
                    className="input"
                    placeholder="e.g. Ann Arbor, Michigan"
                    {...register('region', {
                      // Typing here satisfies the region requirement above
                      // (pick OR describe), so keep that rule's isValid in
                      // sync as they type the fallback.
                      onChange: () => void trigger('regionSlugs'),
                    })}
                  />
                </div>

                <div className="field stacked">
                  <div>
                    <label htmlFor="submit-oneline" className="field-label">
                      One-line description of what they actually do
                      <span className="required">*</span>
                      <span className="hint inline">Plain English. ~140 characters.</span>
                    </label>
                  </div>
                  <textarea
                    id="submit-oneline"
                    className="textarea"
                    rows={2}
                    placeholder="Campaigns for better bus service."
                    {...register('oneLineDesc', { required: 'Required' })}
                  />
                  <FieldError error={errors.oneLineDesc} />
                </div>
              </>
            ) : null}

            <div className="field stacked">
              <div>
                <label htmlFor="submit-why" className="field-label">
                  {copy.editorialLabel}
                  <span className="hint inline">{copy.editorialHint}</span>
                </label>
              </div>
              <textarea
                id="submit-why"
                className="textarea tall"
                rows={4}
                placeholder={copy.editorialPlaceholder}
                {...register('why')}
              />
            </div>

            <div className="field stacked">
              <div>
                <label htmlFor="submit-sources" className="field-label">
                  Sources
                  <span className="hint inline">{copy.sourcesHint}</span>
                </label>
              </div>
              <textarea
                id="submit-sources"
                className="textarea"
                rows={3}
                placeholder="https://newspaper.com/article
https://group.org/about"
                {...register('sources')}
              />
            </div>

            <fieldset className="field stacked">
              <div>
                <legend className="field-label">
                  Disclosure
                  <span className="required">*</span>
                </legend>
              </div>
              <div className="choices inline">
                <label className="choice">
                  <input
                    type="radio"
                    value="none"
                    {...register('affiliation', { required: true })}
                  />
                  <span className="text">I have no affiliation</span>
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    value="member"
                    {...register('affiliation', { required: true })}
                  />
                  <span className="text">I&rsquo;m a member, volunteer, or staff</span>
                </label>
                <label className="choice">
                  <input
                    type="radio"
                    value="other"
                    {...register('affiliation', { required: true })}
                  />
                  <span className="text">Other</span>
                </label>
              </div>
            </fieldset>

            <div className="field">
              <div>
                <label htmlFor="submit-contact" className="field-label">
                  Your email or @handle
                  <span className="hint">
                    Optional — helps if an editor has follow-up questions.
                  </span>
                </label>
              </div>
              <div>
                <input
                  id="submit-contact"
                  type="text"
                  className="input"
                  placeholder="you@example.com or @handle"
                  {...register('contact')}
                />
              </div>
            </div>

            <div className="slip-foot">
              <p className="note">
                We read every tip. The ones we accept open a public pull request anyone
                can follow — usually within a week.
              </p>
              {isRateLimited ? (
                <p className="field-error" role="alert">
                  {retryAfter > 0
                    ? `You've sent a few in a short window. Try again in ${retryAfter} ${retryAfter === 1 ? 'second' : 'seconds'}.`
                    : "You've sent a few in a short window. Take a breather and retry in a few minutes."}
                </p>
              ) : null}
              {showTopLevelValidationErr ? (
                <div className="field-error" role="alert">
                  <p>{validationMessage}</p>
                  {unmappedFieldErrors.length > 0 ? (
                    <ul>
                      {unmappedFieldErrors.map(({ field, message }) => (
                        <li key={field}>{message}</li>
                      ))}
                    </ul>
                  ) : null}
                </div>
              ) : null}
              {isServerErr ? (
                <p className="field-error" role="alert">
                  Our submission queue is having a moment. You can{' '}
                  <a href={issueFallbackUrl} target="_blank" rel="noopener noreferrer">
                    open this as a GitHub issue instead
                  </a>{' '}
                  while we look into it.
                </p>
              ) : null}
              <button
                type="submit"
                className="btn-primary"
                disabled={!isValid || cooldown || mutation.isPending || retryAfter > 0}
              >
                {mutation.isPending ? (
                  'Sending…'
                ) : retryAfter > 0 ? (
                  <>Retry in {retryAfter}s</>
                ) : (
                  <>
                    Send to editorial queue <span className="arrow">→</span>
                  </>
                )}
              </button>
            </div>
          </form>

          <SubmissionProcess />
        </div>

        <SubmitRail />
      </div>
    </>
  );
}

/** The static "what happens after you file" editorial-workflow strip. */
function SubmissionProcess() {
  return (
    <section className="process">
      <div className="process-head">
        <h2>What happens after you file.</h2>
        <div className="aside">Editorial workflow</div>
      </div>
      <div className="process-grid">
        <div className="process-step">
          <div className="num">i.</div>
          <h4>You file the tip.</h4>
          <p>
            Your submission lands in the editorial queue with a reference ID you can hold
            onto.
          </p>
        </div>
        <div className="process-step">
          <div className="num">ii.</div>
          <h4>An editor reads it.</h4>
          <p>
            We check the website, scan recent coverage, and weigh against the inclusion
            criteria.
          </p>
        </div>
        <div className="process-step">
          <div className="num">iii.</div>
          <h4>Verification.</h4>
          <p>
            We confirm with public sources — news coverage, a council record, or a clear
            track of campaigns.
          </p>
        </div>
        <div className="process-step">
          <div className="num">iv.</div>
          <h4>Published or declined.</h4>
          <p>
            Accepted orgs land in a public pull request anyone can read. When the PR
            merges, the org appears in the Atlas.
          </p>
        </div>
      </div>
    </section>
  );
}

/** The submission-page rail: inclusion-criteria pointer + contact routes. */
function SubmitRail() {
  return (
    <aside className="rail">
      <div className="rail-block">
        <div className="rail-kicker">Inclusion criteria</div>
        <p>
          The full list — what we include, what we skip — lives at{' '}
          <Link to="/about#methodology">About / Methodology</Link>.
        </p>
      </div>
      <div className="rail-block muted">
        <div className="rail-kicker">Other ways to reach us</div>
        <p>
          Email <a href="mailto:hello@urbanistatlas.com">hello@urbanistatlas.com</a> for
          anything sensitive, or{' '}
          <a href="https://github.com/mjrossi/urbanist-atlas/issues/new">
            open a GitHub issue
          </a>{' '}
          directly.
        </p>
      </div>
    </aside>
  );
}

interface SubmissionReceivedProps {
  submission: Submission;
  onAnother: () => void;
}

function SubmissionReceived({ submission, onAnother }: SubmissionReceivedProps) {
  const shortId = submission.id.replace(/-/g, '').slice(0, 8);
  return (
    <>
      <PageBreadcrumb
        prefix={[{ label: 'Atlas', to: '/' }]}
        current="Submissions"
        meta="Tip received"
      />
      <div className="spread mt-48">
        <div>
          <div className="lede">
            <div className="eyebrow">
              № II — Submissions desk
              <span className="eyebrow-rule" />
            </div>
            <h1>
              Tip received — <span className="accent">#{shortId}</span>
            </h1>
            <p className="deck">
              Thanks. Your submission is in the editorial queue. When an editor accepts
              it, you&rsquo;ll see the org appear in the Atlas after the next deploy. If
              we have follow-up questions and you left contact info, we&rsquo;ll reach
              out.
            </p>
          </div>

          <div className="receipt">
            <div className="receipt-row">
              <span className="receipt-label">Reference</span>
              <span className="receipt-value mono">#{shortId}</span>
            </div>
            <div className="receipt-row">
              <span className="receipt-label">Status</span>
              <span className="receipt-value">{submission.status}</span>
            </div>
            <div className="receipt-foot">
              <button type="button" className="btn-primary" onClick={onAnother}>
                File another tip <span className="arrow">→</span>
              </button>
              <Link to="/" className="receipt-link">
                Back to the atlas
              </Link>
            </div>
          </div>
        </div>
        <aside className="rail">
          <div className="rail-block">
            <div className="rail-kicker">What happens next</div>
            <p>
              When we accept your tip, the change shows up in a public pull request
              against the dataset. When the PR merges, the org appears in the Atlas after
              the next deploy — usually a few minutes.
            </p>
          </div>
        </aside>
      </div>
    </>
  );
}
