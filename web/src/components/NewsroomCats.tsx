/**
 * The newsroom cats' portraits. Colours come from the `.cat-fill-*` and
 * `.cat-stroke-*` classes in global.css, because SVG `fill`/`stroke`
 * attributes can't carry `var()` but the CSS properties can. Each `<svg>`
 * is decorative and sized by its container's CSS.
 */

/** Pad Thai, an all-black tom, belly up on the page he was to proofread. */
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

/** Mrs Peacock, a dilute tortoiseshell, seated on the keyboard. */
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
      {/* Tail wrapped along the keyboard. */}
      <path
        d="M130 100 C150 101 162 107 159 115 C156 121 142 120 130 113"
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="11"
        strokeLinecap="round"
      />
      <path
        d="M130 100 C150 101 162 107 159 115 C156 121 142 120 130 113"
        className="cat-stroke-coat cat-fill-none"
        strokeWidth="7"
        strokeLinecap="round"
      />
      <path
        d="M150 104 C155 106 158 109 157 113"
        className="cat-stroke-coat-dark cat-fill-none"
        strokeWidth="7"
        strokeLinecap="round"
      />
      {/* Base coat. */}
      <path d={body} className="cat-fill-coat" />
      {/* Coat detail, clipped to her silhouette. */}
      <g clipPath="url(#mrs-peacock-body)">
        {/* Crown and ears. */}
        <ellipse cx="100" cy="47" rx="27" ry="19" className="cat-fill-coat-dark" />
        <polygon points="78,46 72,30 88,36" className="cat-fill-coat-dark" />
        <polygon points="112,36 126,30 122,46" className="cat-fill-coat-dark" />
        {/* Brindle down the flanks. */}
        <g className="cat-fill-coat-dark" fillOpacity="0.4">
          <ellipse cx="80" cy="88" rx="5" ry="16" transform="rotate(-8 80 88)" />
          <ellipse cx="100" cy="92" rx="5" ry="17" />
          <ellipse cx="120" cy="88" rx="5" ry="16" transform="rotate(8 120 88)" />
        </g>
        {/* Cream muzzle, chest, belly and paw fronts. */}
        <ellipse cx="100" cy="65" rx="12" ry="6" className="cat-fill-paper-2" />
        <path
          d="M100 66 C111 72 114 88 109 100 C104 108 92 108 88 100 C84 88 90 72 100 66 Z"
          className="cat-fill-paper-2"
        />
        <ellipse cx="93" cy="106" rx="6" ry="5" className="cat-fill-paper-2" />
        <ellipse cx="107" cy="106" rx="6" ry="5" className="cat-fill-paper-2" />
        {/* Faint warm fawn. */}
        <ellipse
          cx="82"
          cy="56"
          rx="7"
          ry="9"
          transform="rotate(-18 82 56)"
          className="cat-fill-amber"
          fillOpacity="0.22"
        />
        <ellipse
          cx="116"
          cy="80"
          rx="6"
          ry="9"
          transform="rotate(14 116 80)"
          className="cat-fill-amber"
          fillOpacity="0.2"
        />
        {/* Fine ticking across the back. */}
        <path
          d="M84 40 q3 5 1 9 M96 38 q3 5 1 9 M108 39 q3 5 1 9 M118 44 q3 5 1 8"
          className="cat-stroke-ink-2 cat-fill-none"
          strokeWidth="1.2"
          strokeLinecap="round"
          opacity="0.28"
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
 * Cera, a calico, mid-stride across a crosswalk. Her white, ginger and
 * black are existing tokens, so unlike Mrs Peacock she needs no coat
 * tokens.
 */
export function CeraIllustration({ className }: { className?: string }) {
  // The ears are part of the silhouette, so the outline joins them to the
  // head.
  const body =
    'M42 72 C36 58 50 50 64 51 C92 47 118 47 140 51 C146 47 148 46 150 45 L151 26 L162 41 L165 43 L168 41 L177 25 L181 43 C181 47 181 51 180 55 C181 60 178 63 173 64 C168 66 162 66 158 68 C154 72 152 82 150 90 C148 96 140 98 130 97 C108 99 84 99 68 97 C54 95 46 88 44 80 C43 77 42 74 42 72 Z';
  const tail = 'M50 66 C36 62 26 50 30 39 C32 33 38 30 44 33';
  return (
    <svg viewBox="0 0 200 120" aria-hidden="true" focusable="false" className={className}>
      <defs>
        <clipPath id="cera-body">
          <path d={body} />
        </clipPath>
      </defs>
      {/* Crosswalk. */}
      <g className="cat-fill-paper-2 cat-stroke-rule" strokeWidth="1">
        <rect x="14" y="101" width="7" height="6" />
        <rect x="32" y="101" width="7" height="6" />
        <rect x="50" y="101" width="7" height="6" />
        <rect x="68" y="101" width="7" height="6" />
        <rect x="86" y="101" width="7" height="6" />
        <rect x="104" y="101" width="7" height="6" />
        <rect x="122" y="101" width="7" height="6" />
        <rect x="140" y="101" width="7" height="6" />
        <rect x="158" y="101" width="7" height="6" />
        <rect x="176" y="101" width="7" height="6" />
      </g>
      <line
        x1="10"
        y1="107"
        x2="190"
        y2="107"
        className="cat-stroke-rule cat-fill-none"
        strokeWidth="1.5"
      />
      {/* Tail: the ginger core stops short, so the outline is the black tip. */}
      <path
        d={tail}
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="9"
        strokeLinecap="round"
      />
      <path
        d="M50 66 C37 62 28 51 31 41"
        className="cat-stroke-amber-soft cat-fill-none"
        strokeWidth="5"
        strokeLinecap="round"
      />
      <path
        d="M50 66 C46 65 42 63 39 60"
        className="cat-stroke-paper cat-fill-none"
        strokeWidth="5"
        strokeLinecap="round"
      />
      {/* Legs, drawn first so the body covers where they meet it. */}
      <g className="cat-fill-paper-2 cat-stroke-ink" strokeWidth="2">
        <rect x="58" y="86" width="9" height="20" rx="4" />
        <rect x="78" y="86" width="9" height="17" rx="4" />
        <rect x="116" y="86" width="9" height="20" rx="4" />
        <rect x="132" y="84" width="9" height="14" rx="4" transform="rotate(20 136 91)" />
      </g>
      {/* Cream base coat (ears included, since they're part of the path). */}
      <path d={body} className="cat-fill-paper-2" />
      {/* Calico patches, clipped to her silhouette. */}
      <g clipPath="url(#cera-body)">
        <ellipse cx="66" cy="64" rx="22" ry="15" className="cat-fill-amber-soft" />
        <ellipse cx="174" cy="40" rx="12" ry="16" className="cat-fill-amber-soft" />
        <ellipse cx="108" cy="56" rx="24" ry="13" className="cat-fill-ink" />
        <ellipse cx="152" cy="40" rx="12" ry="15" className="cat-fill-ink" />
        <ellipse
          cx="54"
          cy="70"
          rx="11"
          ry="8"
          className="cat-fill-amber"
          fillOpacity="0.55"
        />
        <ellipse cx="50" cy="84" rx="8" ry="9" className="cat-fill-ink" />
      </g>
      {/* Crisp outline, over the coat zones (ears flow from the path). */}
      <path
        d={body}
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="2.5"
        strokeLinejoin="round"
      />
      {/* Inner ears, tucked inside each ear of the silhouette. */}
      <polygon points="153,42 152,31 160,40" className="cat-fill-rule-strong" />
      <polygon points="172,41 177,30 180,42" className="cat-fill-rule-strong" />
      {/* Face: a forward eye set in the ginger cheek, pink nose, whiskers
          sweeping ahead of her, intent on the crossing. */}
      <ellipse cx="170" cy="50" rx="3" ry="2.6" className="cat-fill-ink" />
      <circle cx="171" cy="49" r="0.9" className="cat-fill-paper" />
      <polygon points="176,55 182,55 179,59" className="cat-fill-rule-strong" />
      <path
        d="M179 60 q-3 3 -7 2"
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="1"
        strokeLinecap="round"
      />
      <path
        d="M174 57 q9 -2 16 -1 M174 60 q9 0 15 3"
        className="cat-stroke-ink cat-fill-none"
        strokeWidth="1"
        strokeLinecap="round"
        opacity="0.8"
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
