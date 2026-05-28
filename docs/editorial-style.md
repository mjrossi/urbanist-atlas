# Editorial style

How the directory wants to sound. This is a short reference for
anyone writing a new org description — submitters, the editor who
reviews the queue, contributors filing corrections through the
issue tracker. The companion publication, *Urbanist Lexicon* at
[mjrossi.com](https://mjrossi.com), is the canonical example of
the voice; this file distills the rules that have shown up
repeatedly on review.

If you're filling out the submission form, the field helper text
already nudges you the right way — this doc is the long form of
what that helper text is asking for.

## Voice cheatsheet

- **Specific over abstract.** Name the campaign, the route, the
  council vote. A reader wants to know what this group has actually
  done, not how it characterizes itself.
- **Editorial, not corporate.** This is a directory written by an
  editor, not a press release written by a comms team.
- **Active voice.** "Pushes for protected bike lanes" — not "is
  committed to the advancement of protected bike infrastructure".
- **Personal and inviting.** A reader in Toledo should feel the
  directory was written for them, not at them.
- **Slightly formal, never stiff.** Sentence case, ending periods,
  full sentences. No marketing capitalization.
- **Transparent about the workflow.** The directory is curated by
  hand; when a description is provisional, say so rather than
  papering over it.

## Org description rules

The one-line description in `orgs.toml` is the line a reader sees
first. It does almost all the work.

- **Verb-first, not adjective-first.** "Pushes for…" beats
  "Leading…". The verb is doing the lifting; the adjective is just
  asking the reader to take your word for it.
- **Length.** ~140 characters is the target. ~260 is the ceiling.
  If you're past 200 and still feel like you're missing something,
  the surplus is almost always filler.
- **Expand acronyms on first mention.** "Metropolitan Planning
  Organization (MPO)", not bare "MPO". A reader from another metro
  shouldn't have to guess.
- **Sentence case, ending period.** Like a sentence in a newspaper.
  Not Title Case. Not no-final-punctuation.
- **Concrete numbers and campaigns over vague claims.** "Won a
  $40M state appropriation for sidewalk repair in 2024" reads
  more truthfully than "advances pedestrian infrastructure
  funding".

## Words to avoid

These are the words that show up in the descriptions that get
rewritten on review. They almost always replace specifics the
reader would actually want.

- *leading*, *premier*, *world-class*, *innovative*
- *passionate*, *dedicated*, *committed to*, *on a mission to*
- *empower*, *transform*, *revolutionize*
- *ecosystem* (as metaphor for a community), *synergy*

None of these are banned outright — sometimes a transit federation
genuinely *is* the leading one in its region. But the bar for
using them is "the specific claim is more accurate than the
specific alternative," and that bar is high. The default is to
strike them.

## Examples

**Before:** *Leading nonprofit on a mission to transform city
streets into safer, more equitable spaces for all road users.*

**After:** *Pushes for protected bike lanes, slower posted speeds,
and bus priority. Ran the 2024 campaign that closed two blocks of
Main Street to cars.*

The "before" is generic enough to describe four out of every five
orgs in the directory. The "after" tells a reader why this group,
specifically, is worth their time.

**Before:** *Greater Centerville's premier safe-streets coalition,
committed to advocating for innovative solutions that empower
residents and transform the urban ecosystem.*

**After:** *Coalition of neighborhood groups across Greater
Centerville. Testifies at every city traffic-safety meeting;
ran the petition campaign that won the 25 mph default on
residential streets in 2023.*

The pattern is the same: strike the adjectives, name the work.

## Where this fits

- The submission form at `/submit` carries the short version of
  these rules in its field helper text — read those if you're
  filling out the form for the first time.
- `CONTRIBUTING.md` links back here as the first read for anyone
  proposing an org through the issue tracker or via PR.
- `api/seed/README.md` references this doc for editors working
  directly on `orgs.toml`.
