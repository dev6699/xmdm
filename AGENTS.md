# Agent Rules

## Objective

- XMDM documentation must describe the current implementation exactly.
- Before calling anything done, ask: "Is this backed by source, tests, config, migrations, or an explicit limitation?"
- If a claim is not verifiable from the repo, remove it or mark it unsupported.

## Work Loop

- Inspect the repo before changing it.
- Make the smallest correct change.
- Use existing patterns unless there is a clear reason not to.
- State assumptions when the repo does not answer a question.
- Verify with the narrowest useful check.
- Do not stop at a plan if the task can be completed safely now.

## Blueprint

- The docs under `blueprint/` are durable architecture decisions, not a roadmap or operator manual.
- Current product and operator documentation lives under `docs/`.
- Boundary: `blueprint/` owns product principles, architecture decisions, API/data/security invariants, and rejected alternatives only.
- Boundary: `docs/` owns current capabilities, support boundaries, security overview, architecture overview diagrams, operator runbooks, release procedures, and verification/development guides.
- Do not duplicate capability matrices, route catalogs, service inventories, lifecycle diagrams, release steps, or troubleshooting procedures across both trees.
- If the same fact appears in both trees, keep the user/operator-facing version in `docs/` and reduce the `blueprint/` entry to the durable decision or invariant.
- If the blueprint conflicts with the implementation, verify the implementation first, then update or remove the stale blueprint text.
- Do not use old planning text as evidence for supported behavior.

## Documentation

- Update docs when a user-facing behavior, operational flow, security boundary, or release process changes.
- Keep `README.md` and `docs/README.md` as the public entry points.
- Keep docs implementation-backed and XMDM-specific.
- Delete or rewrite stale planning docs instead of preserving them in the primary reader path.

## Vocabulary

- Use `Android launcher` for the device-side app in docs. Use `agent` only when referring to existing artifact names, endpoint names, or UI labels that already use that word.
- Use `admin dashboard` for the browser operator surface. Do not introduce `admin console` or `web console`.
- Use `control plane` for the server-side system as a product concept, and `Go server` for the implementation runtime.
- Use `PostgreSQL`, not `Postgres`, in documentation prose.
- Use `object storage`, not `object store`, unless naming a concrete service.
- Use `operator` for the human running XMDM. Avoid mixing in `maintainer` or `user` unless the document is specifically about admin accounts.
- Use `self-hosted` and `single-tenant` with hyphens.
- Prefer direct implementation-backed statements over process prose such as "this page describes" or generic product language.

## Save Points

- Suggest a save point at meaningful stages: coherent docs reset done, coherent refactor done, or verified behavior change landed.
- If the user asks for the next step or a similar follow-up at a meaningful checkpoint, suggest saving first.
- Only give the next step after the save is confirmed or completed.
- Use this save prompt pattern:

```text
Save game:
- What changed: <...>
- Why it matters: <...>
- Why now: <...>
Ready to lock it in?
```

- Do not mention the next step in the same message as the save suggestion.
- Use this commit template:

```text
git add -A
git commit -m "<type>(xmdm): <short summary>"
```

- If the user approves, run the commit.
- Then name the next implementation-backed task if one remains.

## Completion

- A task is not done until code, docs, and verification match the behavior being claimed.
- If only part of the work is done, say exactly which subset is complete.
- If runtime behavior changed, include validation evidence in the final response.
