/**
 * Small pill rendering one of an org's open-ended tags. Visual style
 * is shared with the portfolio's blog tag chips (warm amber band,
 * thin rule), ported via `.tag-chip` in global.css.
 */
export function TagChip({ label }: { label: string }) {
  return <span className="tag-chip">{label}</span>;
}
