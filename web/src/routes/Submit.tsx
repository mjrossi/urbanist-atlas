import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useMutation } from '@tanstack/react-query';
import { Link } from 'react-router';
import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { ApiError, createSubmission, type Submission } from '../lib/api.ts';
import {
  buildIssueUrl,
  buildNewSubmissionRequest,
  SUBMIT_FORM_DEFAULTS,
  type SubmitForm,
} from '../lib/submitForm.ts';

// Brief lockout after a successful submit so a triple-click can't
// trigger duplicate POSTs.
const SUBMIT_COOLDOWN_MS = 1500;

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
    getValues,
  } = useForm<SubmitForm>({
    mode: 'onBlur',
    defaultValues: SUBMIT_FORM_DEFAULTS,
  });

  const [cooldown, setCooldown] = useState(false);

  useEffect(() => {
    if (!cooldown) return;
    const id = setTimeout(() => setCooldown(false), SUBMIT_COOLDOWN_MS);
    return () => clearTimeout(id);
  }, [cooldown]);

  const mutation = useMutation<Submission, ApiError, SubmitForm>({
    mutationFn: (form) => createSubmission(buildNewSubmissionRequest(form)),
    onSuccess: () => setCooldown(true),
  });

  const onValid = (form: SubmitForm) => {
    if (cooldown || mutation.isPending) return;
    mutation.mutate(form);
  };

  if (mutation.isSuccess) {
    return (
      <SubmissionReceived
        submission={mutation.data}
        onAnother={() => mutation.reset()}
      />
    );
  }

  const submitErr = mutation.error;
  const isRateLimited = submitErr?.status === 429;
  const isValidationErr = submitErr?.status === 400 || submitErr?.status === 422;
  const isServerErr = submitErr?.status !== undefined && submitErr.status >= 500;

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
              № I — Submissions desk<span className="eyebrow-rule" />
            </div>
            <h1>
              File a tip for <span className="accent">the editors.</span>
            </h1>
            <p className="deck">
              Know an advocacy group that should be in the Atlas? Spotted a
              stale entry? Tell us here. Your submission goes straight to
              the editorial queue and we open a pull request against the
              public dataset when we accept it.
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
                  <input type="radio" value="new" {...register('type', { required: true })} />
                  <span className="text">
                    A new organization to add
                    <span className="sub">A group not yet in the Atlas — local or regional.</span>
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
                  <span className="hint">
                    As it appears on their site, not what locals call them.
                  </span>
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
                {errors.name ? (
                  <span className="field-error" role="alert">
                    {errors.name.message}
                  </span>
                ) : null}
              </div>
            </div>

            <div className="field">
              <div>
                <label htmlFor="submit-website" className="field-label">
                  Primary website
                  <span className="required">*</span>
                  <span className="hint">Their own URL, not a coverage article.</span>
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
                {errors.website ? (
                  <span className="field-error" role="alert">
                    {errors.website.message}
                  </span>
                ) : null}
              </div>
            </div>

            <div className="field">
              <div>
                <label htmlFor="submit-region" className="field-label">
                  Region served
                  <span className="required">*</span>
                  <span className="hint">
                    City or metro slug, e.g. <code>nyc</code> or{' '}
                    <code>chicago</code>. We&rsquo;ll finalize it in review.
                  </span>
                </label>
              </div>
              <div>
                <input
                  id="submit-region"
                  type="text"
                  className="input"
                  placeholder="brooklyn-ny"
                  {...register('region', { required: 'Required' })}
                />
                {errors.region ? (
                  <span className="field-error" role="alert">
                    {errors.region.message}
                  </span>
                ) : null}
              </div>
            </div>

            <div className="field stacked">
              <div>
                <label htmlFor="submit-oneline" className="field-label">
                  One-line description of what they actually do
                  <span className="required">*</span>
                  <span className="hint inline">
                    Plain English. ~140 characters.
                  </span>
                </label>
              </div>
              <textarea
                id="submit-oneline"
                className="textarea"
                rows={2}
                placeholder="Pushes for bus service expansion and rider-led transit policy."
                {...register('oneLineDesc', { required: 'Required' })}
              />
              {errors.oneLineDesc ? (
                <span className="field-error" role="alert">
                  {errors.oneLineDesc.message}
                </span>
              ) : null}
            </div>

            <div className="field stacked">
              <div>
                <label htmlFor="submit-why" className="field-label">
                  Why this org belongs
                  <span className="hint inline">
                    Recent campaigns, who they organize, who they push. Specifics beat
                    adjectives.
                  </span>
                </label>
              </div>
              <textarea
                id="submit-why"
                className="textarea tall"
                rows={4}
                placeholder="In 2025 they organized riders to defend the 60-cent fare against a council proposal to double it; ran a candidate forum that 4 of 9 councilmembers attended; testified at every transit board meeting since 2022."
                {...register('why')}
              />
            </div>

            <div className="field stacked">
              <div>
                <label htmlFor="submit-sources" className="field-label">
                  Sources
                  <span className="hint inline">
                    News coverage, social handles, prior wins. One per line.
                  </span>
                </label>
              </div>
              <textarea
                id="submit-sources"
                className="textarea"
                rows={3}
                placeholder="https://seattletimes.com/...
https://kuow.org/stories/..."
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
                Your submission goes straight to the editorial queue. We
                review every tip and open a public pull request when we
                accept one — usually within a week.
              </p>
              {isRateLimited ? (
                <p className="field-error" role="alert">
                  You&rsquo;ve sent a few in a short window. Take a breather and
                  retry in a few minutes.
                </p>
              ) : null}
              {isValidationErr ? (
                <p className="field-error" role="alert">
                  {submitErr?.problem?.detail ?? submitErr?.message ?? 'Validation failed.'}
                </p>
              ) : null}
              {isServerErr ? (
                <p className="field-error" role="alert">
                  Our submission queue is having a moment. You can{' '}
                  <a
                    href={buildIssueUrl(getValues())}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    open this as a GitHub issue instead
                  </a>{' '}
                  while we look into it.
                </p>
              ) : null}
              <button
                type="submit"
                className="btn-primary"
                disabled={!isValid || cooldown || mutation.isPending}
              >
                {mutation.isPending ? (
                  'Sending…'
                ) : (
                  <>
                    Send to editorial queue <span className="arrow">→</span>
                  </>
                )}
              </button>
            </div>
          </form>

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
                  Your submission lands in the editorial queue with a
                  reference ID. No GitHub account needed.
                </p>
              </div>
              <div className="process-step">
                <div className="num">ii.</div>
                <h4>An editor reads it.</h4>
                <p>
                  We check the website, scan recent coverage, and weigh against
                  the inclusion criteria.
                </p>
              </div>
              <div className="process-step">
                <div className="num">iii.</div>
                <h4>Verification.</h4>
                <p>
                  We confirm with public sources — news coverage, a council
                  record, or a clear track of campaigns.
                </p>
              </div>
              <div className="process-step">
                <div className="num">iv.</div>
                <h4>Published or declined.</h4>
                <p>
                  Accepted orgs land in a public pull request anyone can read.
                  When the PR merges, the org appears in the Atlas.
                </p>
              </div>
            </div>
          </section>
        </div>

        <aside className="rail">
          <div className="rail-block">
            <div className="rail-kicker">Inclusion criteria</div>
            <p>
              The full criteria — what we include, what we skip — lives at{' '}
              <Link to="/about#methodology">About / Methodology</Link>.
            </p>
          </div>
          <div className="rail-block muted">
            <div className="rail-kicker">Other ways to reach us</div>
            <p>
              Email{' '}
              <a href="mailto:hello@urbanistatlas.com">
                hello@urbanistatlas.com
              </a>{' '}
              for anything sensitive, or{' '}
              <a href="https://github.com/mjrossi/urbanist-atlas/issues/new">
                open a GitHub issue
              </a>{' '}
              directly.
            </p>
          </div>
        </aside>
      </div>
    </>
  );
}

interface SubmissionReceivedProps {
  submission: Submission;
  onAnother: () => void;
}

function SubmissionReceived({ submission, onAnother }: SubmissionReceivedProps) {
  const shortId = String(submission.id).replace(/-/g, '').slice(0, 8);
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
              № II — Submissions desk<span className="eyebrow-rule" />
            </div>
            <h1>
              Tip received — <span className="accent">#{shortId}</span>
            </h1>
            <p className="deck">
              Thanks. Your submission is in the editorial queue. When an
              editor accepts it, you&rsquo;ll see the org appear in the
              Atlas after the next deploy. If we have follow-up questions
              and you left contact info, we&rsquo;ll reach out.
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
              When we accept your tip, the change shows up in a public
              pull request against the dataset. When the PR merges, the
              org appears in the Atlas after the next deploy — usually a
              few minutes.
            </p>
          </div>
        </aside>
      </div>
    </>
  );
}
