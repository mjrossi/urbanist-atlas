import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { Link } from 'react-router';
import { PageBreadcrumb } from '../components/PageBreadcrumb.tsx';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import {
  buildIssueBody,
  SUBMIT_FORM_DEFAULTS,
  titlePrefix,
  type SubmitForm,
} from '../lib/submitForm.ts';

// Brief lockout after a successful window.open so a triple-click on the
// submit button doesn't open three identical GitHub-issue draft tabs.
const SUBMIT_COOLDOWN_MS = 1500;

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
    defaultValues: SUBMIT_FORM_DEFAULTS,
  });

  const [justOpened, setJustOpened] = useState(false);
  const [blockedUrl, setBlockedUrl] = useState<string | null>(null);

  // Lift the cooldown timer into an effect so unmounting mid-cooldown
  // tears the setTimeout down via the cleanup, and so the timer
  // doesn't leak across renders.
  useEffect(() => {
    if (!justOpened) return;
    const id = setTimeout(() => setJustOpened(false), SUBMIT_COOLDOWN_MS);
    return () => clearTimeout(id);
  }, [justOpened]);

  const onValid = (form: SubmitForm) => {
    if (justOpened) return;
    const title = `${titlePrefix(form.type)} ${form.name}`;
    const body = buildIssueBody(form);
    // GitHub silently drops `body=` when `template=` is also present,
    // so we deliberately omit the template param and ship the full
    // pre-filled body instead.
    const params = new URLSearchParams({ title, body });
    const url = `https://github.com/mjrossi/urbanist-atlas/issues/new?${params.toString()}`;
    const opened = window.open(url, '_blank', 'noopener');
    if (opened) {
      setBlockedUrl(null);
      setJustOpened(true);
    } else {
      // Pop-up blocked. Surface the URL inline so the user can copy /
      // open it themselves; don't lock the button so they can retry
      // after relaxing the browser's pop-up settings.
      setBlockedUrl(url);
    }
  };

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
              stale entry? Tell us here. Submitting opens a pre-filled GitHub
              issue in a new tab — review what&rsquo;s there, edit anything,
              and post.
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
                    City, county, metro, or state. Be specific.
                  </span>
                </label>
              </div>
              <div>
                <input
                  id="submit-region"
                  type="text"
                  className="input"
                  placeholder="Seattle, WA"
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
                Filing this opens a pre-filled GitHub issue in a new tab. You can
                review it, edit anything, and post — or close the tab to start
                over. <strong>Nothing is sent yet.</strong>
              </p>
              {blockedUrl ? (
                <p className="field-error" role="alert">
                  Your browser blocked the pop-up.{' '}
                  <a href={blockedUrl} target="_blank" rel="noopener noreferrer">
                    Open the pre-filled issue manually
                  </a>
                  , or allow pop-ups from urbanistatlas.com and try again.
                </p>
              ) : null}
              <button
                type="submit"
                className="btn-primary"
                disabled={!isValid || justOpened}
              >
                Open as GitHub issue <span className="arrow">→</span>
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
                  A pre-filled GitHub issue opens in a new tab. Takes about a
                  minute to read and post.
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
                  Either way you get a reply on the issue. We always say why if
                  we decline, so you can re-file with more.
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

