/**
 * The newsroom cats. Every print shop of consequence kept a cat; the
 * Atlas keeps two, and these are their woodcut portraits — flat fills,
 * heavy ink outlines, drawn entirely from the design-token palette via
 * the `.cat-fill-*` / `.cat-stroke-*` classes in global.css (SVG
 * `fill`/`stroke` *attributes* can't carry `var()`, but the CSS
 * properties can).
 *
 * All three are decorative: `aria-hidden`, unfocusable, sized by the
 * container through CSS (no width/height attributes). Pad Thai's tail
 * is grouped as `.cat-tail` so the 404 page can give it a slow sway.
 */

/**
 * Pad Thai, an all-black tom, in his signature pose: flat on his back,
 * belly up, paws in the air — sprawled across the page he was asked
 * to proofread.
 */
export function PadThaiIllustration({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 200 110" aria-hidden="true" focusable="false" className={className}>
      {/* Ground rule. */}
      <line
        x1="10"
        y1="96"
        x2="190"
        y2="96"
        className="cat-stroke-rule cat-fill-none"
        strokeWidth="1.5"
      />
      {/* The missing page, tucked under his back. */}
      <g transform="rotate(-8 47 91)">
        <rect
          x="18"
          y="80"
          width="58"
          height="22"
          className="cat-fill-paper-2 cat-stroke-rule"
          strokeWidth="1.5"
        />
        <path
          d="M18 80 l7 0 0 7 z"
          className="cat-fill-paper cat-stroke-rule"
          strokeWidth="1"
        />
        <path
          d="M30 86 h38 M30 91 h38 M30 96 h28"
          className="cat-stroke-ink cat-fill-none"
          strokeWidth="1"
          opacity="0.45"
        />
      </g>
      {/* Tail, sweeping off along the ground. */}
      <g className="cat-tail">
        <path
          d="M52 86 C36 92 22 90 16 82 C12 76 16 69 23 71"
          className="cat-stroke-ink cat-fill-none"
          strokeWidth="7"
          strokeLinecap="round"
        />
      </g>
      {/* Body: one ink silhouette built from overlapping shapes. */}
      <g className="cat-fill-ink">
        <ellipse cx="100" cy="70" rx="58" ry="24" />
        <circle cx="162" cy="74" r="19" />
        {/* Ears point groundward — the head is tipped all the way back. */}
        <polygon points="172,84 184,96 164,92" />
        <polygon points="156,89 163,99 146,92" />
        {/* Four paws, lazily splayed. */}
        <rect x="68" y="27" width="9" height="28" rx="4.5" transform="rotate(-8 72 41)" />
        <rect x="86" y="24" width="9" height="30" rx="4.5" transform="rotate(3 90 39)" />
        <rect
          x="120"
          y="25"
          width="9"
          height="29"
          rx="4.5"
          transform="rotate(-4 124 39)"
        />
        <rect
          x="136"
          y="30"
          width="9"
          height="27"
          rx="4.5"
          transform="rotate(9 140 43)"
        />
      </g>
      {/* Fur sheen: sparse paper hatching across the belly. */}
      <path
        d="M82 58 q7 3 13 2 M98 53 q7 3 13 2 M116 55 q6 3 12 2 M92 66 q6 2 11 1"
        className="cat-stroke-paper cat-fill-none"
        strokeWidth="1.2"
        strokeLinecap="round"
        opacity="0.7"
      />
      {/* Face: one blissfully closed eye, nose, whiskers. */}
      <path
        d="M163 68 q5 5 10 3"
        className="cat-stroke-amber-soft cat-fill-none"
        strokeWidth="2"
        strokeLinecap="round"
      />
      <circle cx="177" cy="62" r="2" className="cat-fill-amber-soft" />
      <path
        d="M174 60 q-7 -7 -12 -9 M177 58 q-4 -9 -8 -13"
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="1"
        strokeLinecap="round"
        opacity="0.8"
      />
    </svg>
  );
}

/**
 * Mrs Peacock, a dilute tortoiseshell, at her post: seated squarely on
 * the keyboard, in front of whatever you were trying to read. A dilute
 * (blue-cream) tortie reads as three softly blended coats, not bold
 * patches: a blue-grey base, muted fawn-brown brindled through it, and
 * cream on the chest and paws. The color zones are clipped to her
 * silhouette so the brindle never spills past the outline.
 */
export function MrsPeacockIllustration({ className }: { className?: string }) {
  const body =
    'M64 106 C63 92 67 80 76 72 C77 68 78 66 80 64 C77 60 76 52 78 46 L72 30 L88 36 C94 32 106 32 112 36 L126 30 L122 46 C124 52 123 60 120 64 C122 66 123 68 124 72 C133 80 137 92 136 106 C120 111 80 111 64 106 Z';
  return (
    <svg viewBox="0 0 200 130" aria-hidden="true" focusable="false" className={className}>
      <defs>
        <clipPath id="mrs-peacock-body">
          <path d={body} />
        </clipPath>
      </defs>
      {/* Monitor, screen copy, stand. */}
      <rect
        x="48"
        y="10"
        width="104"
        height="66"
        rx="4"
        className="cat-fill-paper-2 cat-stroke-ink"
        strokeWidth="2.5"
      />
      <path
        d="M60 26 h80 M60 38 h80 M60 50 h80 M60 62 h56"
        className="cat-stroke-rule cat-fill-none"
        strokeWidth="2"
        strokeLinecap="round"
      />
      <polygon points="92,76 108,76 114,88 86,88" className="cat-fill-ink" />
      {/* Keyboard, two rows of key ticks. */}
      <polygon
        points="30,104 170,104 178,118 22,118"
        className="cat-fill-paper-2 cat-stroke-ink"
        strokeWidth="2"
      />
      <path
        d="M40 107 l1 4 M52 107 l1 4 M64 107 l1 4 M136 107 l1 4 M148 107 l1 4 M160 107 l1 4 M36 113 l1 3 M48 113 l1 3 M152 113 l1 3 M164 113 l1 3"
        className="cat-stroke-rule cat-fill-none"
        strokeWidth="1.5"
      />
      {/* Tail wrapped along the keyboard: ink outline, muted core, a
          lighter brindle band near the tip. */}
      <path
        d="M130 100 C150 101 162 107 159 115 C156 121 142 120 130 113"
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="11"
        strokeLinecap="round"
      />
      <path
        d="M130 100 C150 101 162 107 159 115 C156 121 142 120 130 113"
        className="cat-stroke-muted cat-fill-none"
        strokeWidth="7"
        strokeLinecap="round"
      />
      <path
        d="M150 104 C155 106 158 109 157 113"
        className="cat-stroke-rule cat-fill-none"
        strokeWidth="7"
        strokeLinecap="round"
      />
      {/* Base coat: soft blue-grey. */}
      <path d={body} className="cat-fill-muted" />
      {/* The three coats, clipped to her silhouette. */}
      <g clipPath="url(#mrs-peacock-body)">
        {/* Cream chest and belly — the white in her coat. */}
        <path
          d="M100 62 C113 70 117 88 110 102 C104 110 90 109 86 100 C82 86 88 70 100 62 Z"
          className="cat-fill-rule"
        />
        <ellipse cx="100" cy="100" rx="15" ry="9" className="cat-fill-paper-2" />
        {/* Cream paw fronts at the sitting base. */}
        <ellipse cx="93" cy="106" rx="6" ry="5" className="cat-fill-paper-2" />
        <ellipse cx="107" cy="106" rx="6" ry="5" className="cat-fill-paper-2" />
        {/* Muted fawn-brown brindle — soft and translucent, blended into
            the grey rather than stamped on. */}
        <g className="cat-fill-amber" fillOpacity="0.5">
          <ellipse cx="82" cy="55" rx="9" ry="12" transform="rotate(-18 82 55)" />
          <ellipse cx="119" cy="62" rx="7" ry="10" transform="rotate(16 119 62)" />
          <ellipse cx="95" cy="44" rx="6" ry="5" />
          <ellipse cx="112" cy="90" rx="6" ry="9" transform="rotate(12 112 90)" />
        </g>
        {/* Warm wash over the muzzle. */}
        <ellipse
          cx="100"
          cy="61"
          rx="11"
          ry="5"
          className="cat-fill-amber"
          fillOpacity="0.28"
        />
        {/* Darker grey ticking across the crown and back — the dilute
            tabby brindle. */}
        <path
          d="M80 40 q3 5 1 10 M90 38 q3 5 1 10 M104 38 q3 5 1 10 M114 41 q3 5 1 10 M124 50 q3 5 1 9"
          className="cat-stroke-ink-2 cat-fill-none"
          strokeWidth="1.4"
          strokeLinecap="round"
          opacity="0.35"
        />
      </g>
      {/* Crisp outline, on top of the coat zones. */}
      <path
        d={body}
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="2.5"
        strokeLinejoin="round"
      />
      {/* Face. */}
      <g className="cat-fill-ink">
        <ellipse cx="91" cy="55" rx="4" ry="3" transform="rotate(8 91 55)" />
        <ellipse cx="109" cy="55" rx="4" ry="3" transform="rotate(-8 109 55)" />
      </g>
      <circle cx="92.2" cy="54" r="1" className="cat-fill-paper" />
      <circle cx="110.2" cy="54" r="1" className="cat-fill-paper" />
      {/* Pinkish nose. */}
      <polygon points="97,63 103,63 100,67" className="cat-fill-rule-strong" />
      <path
        d="M86 63 q-11 -2 -17 -1 M86 66 q-11 2 -16 4 M114 63 q11 -2 17 -1 M114 66 q11 2 16 4"
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="1"
        strokeLinecap="round"
      />
    </svg>
  );
}

/**
 * The small mark of the newsroom: a cat curled asleep, ears up just
 * in case, tail tucked around the curl. Single-color (`currentColor`)
 * so it inherits whatever ink its container writes in.
 */
export function NewsroomCatGlyph({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      focusable="false"
      className={className}
      fill="currentColor"
    >
      {/* Curled body with two ears. */}
      <path d="M5 13.5 C5 11 6 9 7.6 7.6 L7.2 3.6 L10.6 6.2 C11 6.05 11.5 6 12 6 C12.5 6 13 6.05 13.4 6.2 L16.8 3.6 L16.4 7.6 C18 9 19 11 19 13.5 C19 17.6 15.9 21 12 21 C8.1 21 5 17.6 5 13.5 Z" />
      {/* Tail wrapping outside the curl. */}
      <path d="M6.5 19.5 C4.5 18.3 3.2 16.2 3 14 C1.6 16.8 2.6 20 5.4 21.6 C7.4 22.7 9.8 22.7 11.5 21.8 C9.6 21.6 7.9 20.8 6.5 19.5 Z" />
    </svg>
  );
}
