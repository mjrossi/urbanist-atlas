import { useMutation } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';
import {
  Controller,
  type FieldErrors,
  type FieldPath,
  useForm,
  useWatch,
} from 'react-hook-form';
import { Link } from 'react-router';

import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { RegionCombobox } from '../components/RegionCombobox.tsx';
import { ApiError, createSubmission, type Submission } from '../lib/api.ts';
import {
  buildIssueUrl,
  buildNewSubmissionRequest,
  contactError,
  isServerOrUnreachable,
  nameError,
  type NoteField,
  noteFieldError,
  shortDescError,
  shortSubmissionId,
  type SubmissionType,
  SUBMIT_FORM_DEFAULTS,
  type SubmitForm,
  websiteError,
} from '../lib/submitForm.ts';
import { useCountdown } from '../lib/timers.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

// API `errors` keys to form fields. A key with no entry here, or one whose
// field is hidden for this submission type, surfaces in the top-level
// banner instead (see deriveSubmitError).
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

// The fields with an inline error slot, top to bottom.
const ERROR_SLOT_FIELDS = [
  'name',
  'website',
  'regionSlugs',
  'region',
  'oneLineDesc',
  'why',
  'sources',
  'contact',
] as const;

// The slots rendered only on a new-org submission.
const NEW_ORG_ONLY_FIELDS: ReadonlySet<FieldPath<SubmitForm>> = new Set([
  'regionSlugs',
  'region',
  'oneLineDesc',
]);

// The fields composed into submitter_note re-check each other, since text
// in one can push the note past its cap for the error shown on another.
function noteFieldRules(field: NoteField) {
  return {
    validate: (_: unknown, form: SubmitForm) => noteFieldError(field, form) ?? true,
    deps: (['region', 'why', 'sources'] as const).filter((f) => f !== field),
  };
}

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
 * Collapses a failed submission into the flags the error banners need.
 * `unmappedFieldErrors` are field errors with no visible slot, which the
 * top-level banner shows so they aren't swallowed.
 */
function deriveSubmitError(
  err: Error | null,
  visibleFieldErrorSlots: ReadonlySet<FieldPath<SubmitForm>>,
): SubmitErrorView {
  const view: SubmitErrorView = {
    isRateLimited: false,
    isServerErr: false,
    showTopLevelValidationErr: false,
    validationMessage: '',
    unmappedFieldErrors: [],
  };
  if (!err) return view;
  if (!(err instanceof ApiError) || isServerOrUnreachable(err)) {
    return { ...view, isServerErr: true };
  }

  const isValidationErr = err.status === 400;
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
    ...view,
    isRateLimited: err.status === 429,
    showTopLevelValidationErr:
      isValidationErr && (!hasMappedFieldErrors || unmappedFieldErrors.length > 0),
    validationMessage: err.problem?.detail ?? err.message,
    unmappedFieldErrors,
  };
}

/** A fieldset's legend; it must be the fieldset's first child to name the group. */
function FieldsetLegend({
  children,
  required,
}: {
  children: string;
  required?: boolean;
}) {
  return (
    <legend className="field-label">
      {children}
      {required ? <span className="required">*</span> : null}
    </legend>
  );
}

/** The inline error under a form control; nothing when the field is clean. */
function FieldErrorSlot({ error }: { error?: { message?: string } }) {
  return error ? (
    <span className="field-error" role="alert">
      {error.message}
    </span>
  ) : null;
}

/**
 * `/submit`: posts an org tip and shows a receipt with its short id. If
 * the API is down, it offers a pre-filled GitHub issue built from the
 * same form values.
 */
export function Submit() {
  useDocumentTitle('Submit an organization — Urbanist Atlas');

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
    control,
    trigger,
    reset,
    setFocus,
  } = useForm<SubmitForm>({
    mode: 'onBlur',
    defaultValues: SUBMIT_FORM_DEFAULTS,
    // RHF focuses in registration order, and the region Controller
    // registers after the inputs that follow it; onInvalid uses page order.
    shouldFocusError: false,
  });

  const submissionType = useWatch({ control, name: 'type', defaultValue: 'new' });
  const isNewOrg = submissionType === 'new';
  const copy = SUBMISSION_COPY[submissionType];

  // The 429 Retry-After countdown, armed in onError below. The submit
  // button stays disabled until it elapses.
  const [retryAfter, setRetryAfter] = useCountdown();

  // Error, not ApiError: a failure with no response may not be an
  // ApiError, and it still has to reach the server-error banner.
  const mutation = useMutation<Submission, Error, SubmitForm>({
    mutationFn: (form) => createSubmission(buildNewSubmissionRequest(form)),
    onError: (err) => {
      if (!(err instanceof ApiError)) return;
      if (err.fieldErrors) {
        for (const [apiField, message] of Object.entries(err.fieldErrors)) {
          const formField = FIELD_NAME_MAP[apiField];
          if (formField) {
            setError(formField, { type: 'server', message }, { shouldFocus: false });
          }
        }
      }
      // Without Retry-After, the static rate-limit copy still renders.
      if (err.status === 429 && err.retryAfterSeconds && err.retryAfterSeconds > 0) {
        setRetryAfter(err.retryAfterSeconds);
      }
    },
  });

  const onValid = (form: SubmitForm) => {
    if (mutation.isPending || retryAfter > 0) return;
    mutation.mutate(form);
  };
  const onInvalid = (fieldErrors: FieldErrors<SubmitForm>) => {
    const first = ERROR_SLOT_FIELDS.find((f) => fieldErrors[f]);
    if (first) setFocus(first);
  };

  // A subscription, not getValues(), so the GitHub-issue link follows
  // edits made after the error: registered inputs don't re-render.
  const fallbackValues = useWatch({ control });

  if (mutation.isSuccess) {
    return (
      <SubmissionReceived
        submission={mutation.data}
        onAnother={() => {
          // Submit stays mounted, so useForm keeps the typed values
          // unless reset() clears them.
          reset();
          mutation.reset();
        }}
      />
    );
  }

  const submitErr = mutation.error;
  const visibleFieldErrorSlots = new Set<FieldPath<SubmitForm>>(
    ERROR_SLOT_FIELDS.filter((f) => isNewOrg || !NEW_ORG_ONLY_FIELDS.has(f)),
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

          <form onSubmit={handleSubmit(onValid, onInvalid)} noValidate className="slip">
            <div className="slip-head">
              <div className="stamp-l">Submission slip</div>
              <div className="title">Tip sheet</div>
              <div className="stamp-r">For editorial review</div>
            </div>

            <fieldset className="field stacked">
              <FieldsetLegend required>Type of submission</FieldsetLegend>
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
                  {...register('name', {
                    required: 'Required',
                    validate: (v) => nameError(v) ?? true,
                  })}
                />
                <FieldErrorSlot error={errors.name} />
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
                  {...register('website', {
                    required: 'Required',
                    validate: (v) => websiteError(v) ?? true,
                  })}
                />
                <FieldErrorSlot error={errors.website} />
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
                          ref={field.ref}
                          id="submit-region"
                          value={field.value}
                          onChange={(next) => {
                            field.onChange(next);
                            // Re-validate now, not on blur, so a shown
                            // error clears the instant a region is picked.
                            void trigger('regionSlugs');
                          }}
                          onBlur={field.onBlur}
                          describedById="submit-region-hint"
                          invalid={!!fieldState.error}
                        />
                      )}
                    />
                    <FieldErrorSlot error={errors.regionSlugs} />
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
                      ...noteFieldRules('region'),
                      // Typing here satisfies the region rule on the
                      // field above, so re-check that field as they type.
                      onChange: () => void trigger('regionSlugs'),
                    })}
                  />
                  <FieldErrorSlot error={errors.region} />
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
                    {...register('oneLineDesc', {
                      required: 'Required',
                      validate: (v) => shortDescError(v) ?? true,
                    })}
                  />
                  <FieldErrorSlot error={errors.oneLineDesc} />
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
                {...register('why', noteFieldRules('why'))}
              />
              <FieldErrorSlot error={errors.why} />
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
                {...register('sources', noteFieldRules('sources'))}
              />
              <FieldErrorSlot error={errors.sources} />
            </div>

            <fieldset className="field stacked">
              <FieldsetLegend required>Disclosure</FieldsetLegend>
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
                  {...register('contact', {
                    validate: (v) => contactError(v) ?? true,
                  })}
                />
                <FieldErrorSlot error={errors.contact} />
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
                disabled={mutation.isPending || retryAfter > 0}
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
  const shortId = shortSubmissionId(submission.id);
  // The receipt replaces the form, unmounting the button that had focus.
  const headingRef = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    headingRef.current?.focus();
  }, []);
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
            <h1 ref={headingRef} tabIndex={-1}>
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
