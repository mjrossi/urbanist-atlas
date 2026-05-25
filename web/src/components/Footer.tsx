/**
 * Page-bottom footer. Mirrors the portfolio's `.broadsheet-footer`
 * shape — a centered italic colophon flanked by smaller meta rows —
 * but with Atlas wording. The colophon links the companion publication;
 * the disclaimer below it frames the Atlas as an independent
 * informational directory; the bottom row carries the brand domain and
 * outbound links for reporting issues or browsing the source.
 */
export function Footer() {
  return (
    <footer className="broadsheet-footer">
      <div className="broadsheet-footer-inner">
        <span className="broadsheet-colophon">
          A companion volume to{' '}
          <a href="https://mjrossi.com/blog">
            <i>Urbanist Lexicon</i>
          </a>{' '}
          &middot; Set in Fraunces, Source Serif &amp; Inter
        </span>
        <span className="broadsheet-disclaimer">
          An independent informational directory. Not affiliated with the
          organizations listed.
        </span>
        <div className="broadsheet-footer-row">
          <span className="footer-left">urbanistatlas.com</span>
          <span className="footer-right">
            <a href="/colophon">Colophon →</a>
            {' · '}
            <a href="/about#for-developers">Developer preview →</a>
            {' · '}
            <a href="https://github.com/mjrossi/urbanist-atlas/issues/new">
              Report an issue →
            </a>
            {' · '}
            <a href="https://github.com/mjrossi/urbanist-atlas">
              Source on GitHub →
            </a>
          </span>
        </div>
      </div>
    </footer>
  );
}
