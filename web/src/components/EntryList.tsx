import type { Org } from '../lib/api.ts';
import { Entry } from './Entry.tsx';

/**
 * Classified-section list grouped into Local (city / county) and
 * Regional (metro / state / province / multi-state) blocks. Each
 * section renders its own small-caps label even when empty so the
 * reader sees which tier turned up nothing.
 */
export function EntryList({ local, regional }: { local: Org[]; regional: Org[] }) {
  return (
    <div className="entry-list-wrap">
      <Section label="Local" orgs={local} emptyHint="No local groups indexed yet." />
      <Section
        label="Regional"
        orgs={regional}
        emptyHint="No regional groups indexed yet."
      />
    </div>
  );
}

function Section({
  label,
  orgs,
  emptyHint,
}: {
  label: string;
  orgs: Org[];
  emptyHint: string;
}) {
  return (
    <section className="results-section" aria-labelledby={`section-${label.toLowerCase()}`}>
      <h2 id={`section-${label.toLowerCase()}`} className="section-label">
        {label}
      </h2>
      {orgs.length === 0 ? (
        <p className="results-section-empty">{emptyHint}</p>
      ) : (
        <ul className="entry-list">
          {orgs.map((org) => (
            <Entry key={org.id} org={org} />
          ))}
        </ul>
      )}
    </section>
  );
}
