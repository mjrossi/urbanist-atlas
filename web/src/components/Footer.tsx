/**
 * Page-bottom footer. Mirrors the portfolio's `.broadsheet-footer`
 * shape — a centered italic colophon flanked by smaller meta rows —
 * but with Atlas wording. The colophon links the companion publication;
 * the bottom row carries the brand domain and a link to the source
 * repository so contributors have a one-click path to the code.
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
        <div className="broadsheet-footer-row">
          <span className="footer-left">urbanistatlas.com</span>
          <span className="footer-right">
            <a href="https://github.com/mjrossi/urbanist-atlas">
              Source on GitHub →
            </a>
          </span>
        </div>
      </div>
    </footer>
  );
}
