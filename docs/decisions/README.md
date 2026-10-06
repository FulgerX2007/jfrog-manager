# Architecture Decision Records (ADRs)

This folder records **significant, hard-to-reverse decisions** — one file per
decision. ADRs capture the context and consequences of a choice so future readers
(and AI agents) understand *why* the system is the way it is.

## When to write an ADR

Write one when a decision is costly to reverse or shapes the architecture:
- choosing a datastore, framework, protocol, or major dependency
- a cross-cutting pattern (auth model, error handling, concurrency approach)
- a deliberate trade-off someone will later ask "why did we do it this way?"

Do **not** write one for routine, easily-reversible changes.

## How to add one

1. Copy [`adr-template.md`](adr-template.md) to `NNNN-short-title.md` (zero-padded, next number).
   No ADR exists yet, so the first one is `0001-short-title.md`.
2. Fill it in. Set status to `Proposed`, then `Accepted` once agreed.
3. Link it from [`ARCHITECTURE.md`](../../ARCHITECTURE.md) §8 if it affects the architecture.
4. Never rewrite history: to reverse a decision, add a new ADR that
   *supersedes* the old one and update the old one's status.

## Index

No ADRs have been recorded. This directory did not exist before the documentation
set was scaffolded; the only earlier design document in the repository is the
original implementation plan,
[`docs/plans/completed/20260330-jfrog-manager.md`](../plans/completed/20260330-jfrog-manager.md),
which is a plan and not an ADR.

| ADR | Title | Status |
|-----|-------|--------|

TODO: a maintainer should decide whether any past decisions warrant a retroactive ADR and add them here. None are listed because the repository does not record the reasoning behind its existing choices.
