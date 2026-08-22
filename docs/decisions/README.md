# Writing an ADR

This file sets the writing standard for ADR-004 and every ADR after it. ADR-000 through ADR-003 predate this standard. They are historical records. Do not rewrite them to match this standard.

## 1. Structure

Use RFC-style structure:

- Numbered sections with short, descriptive headings.
- A **Terms** section when the ADR introduces words with a specific meaning (see ADR-004 §1 for an example). Skip it when the ADR introduces no new terms.
- One decision or claim per paragraph. Split a paragraph that carries two claims.
- A decisions table, an alternatives-considered section, and a consequences section (positive / negative / neutral), matching ADR-000 through ADR-003.
- A future-work section when the ADR sets rules for something it does not itself implement.
- A references section linking to related ADRs and source issues.

## 2. Normative language

Use RFC 2119 keywords — MUST, MUST NOT, SHOULD, SHOULD NOT, MAY — for every binding design rule. Reserve them for rules; do not use them in descriptive prose.

Bad: "Darna should probably not write to refs."
Good: "Darna MUST NOT write to any ref."

State each rule once, as its own labelled item (for example, "**Rule V1**: ..."), so later sections and later ADRs can reference it by name.

## 3. Prose style: Simplified Technical English (STE)

Write body prose in STE:

- One idea per sentence.
- Short sentences. Split a sentence joined by "and," "which," or a comma clause into two sentences.
- Active voice. "Darna writes the cache file," not "the cache file is written by darna."
- No nominalizations where a verb reads more plainly. "Darna verifies the tree," not "darna performs verification of the tree."
- Consistent terminology. Pick one term per concept and reuse it. Do not vary a term for style (e.g. always "tree," never "snapshot" for the same thing).

## 4. Conciseness

- Prefer a table or a list over a paragraph when the content is a set of items, not an argument.
- Cut a sentence that restates a point made in an adjacent sentence.
- Favor the shortest phrasing that stays correct. An ADR that covers more open questions than ADR-001 SHOULD still read shorter than ADR-001, because structure, not prose volume, carries the explanation.

## 5. Scope

This standard applies to `docs/decisions/*.md` from ADR-004 onward. It does not apply to README.md, CLAUDE.md, or other project documentation, which follow their own conventions.
