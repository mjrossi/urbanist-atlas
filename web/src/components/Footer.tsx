/**
 * Page-bottom footer. Mirrors the portfolio's `.broadsheet-footer`
 * shape — a centered italic colophon flanked by smaller meta rows —
 * but with Atlas wording.
 *
 * No real links here yet; submission credits and methodology copy
 * land in slice #15 (About) and the seed/methodology slices.
 */
export function Footer() {
  return (
    <footer className="broadsheet-footer">
      <div className="broadsheet-footer-inner">
        <span className="broadsheet-colophon">
          A companion volume to <i>Urbanist Lexicon</i> &middot; Set in Fraunces &amp;
          Source Serif
        </span>
        <div className="broadsheet-footer-row">
          <span className="footer-left">urbanistatlas.com</span>
          <span className="footer-right">United States &amp; Canada</span>
        </div>
      </div>
    </footer>
  );
}
