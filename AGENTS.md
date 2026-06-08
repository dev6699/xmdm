# Agent Rules

## Objective

- XMDM is the long game: blueprint first, then code, tests, ops, and release assets in this repo.
- The win state is the full roadmap implemented, verified, and usable end to end.
- Before calling anything done, ask: "Am I done yet based on the roadmap?"
- If any roadmap item is still open, the answer is no.

## Work Loop

- Inspect the repo before changing it.
- Make the smallest correct change.
- Use existing patterns unless there is a clear reason not to.
- State assumptions when the repo does not answer a question.
- Verify with the narrowest useful check.
- Do not stop at a plan if the task can be completed safely now.

## Blueprint

- The docs under `blueprint/` are the source of truth.
- Keep the reading order stable unless the dependency order changes.
- If a decision is not written in the blueprint, it is not decided.
- If the blueprint conflicts with the code, update the blueprint first.

## Snapshot

- Update `PROJECT_STATUS.md` when a roadmap item changes state, a milestone gains real progress, or the user asks for a status update.
- Keep the snapshot aligned with [blueprint/09-roadmap-checklist.md](blueprint/09-roadmap-checklist.md).
- Use the same milestone names, item names, and ranges as the roadmap.
- Only mark an item complete if the roadmap item itself is complete.
- Keep the snapshot compact and readable.

## Save Points

- Suggest a save point at meaningful stages: roadmap item done, coherent refactor done, or verified behavior change landed.
- Update the project status snapshot with the current completion date before saving.
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
- Then name the next roadmap stage or backlog task.

## Completion

- A task is not done until code, docs, and verification match the roadmap item being claimed.
- If only part of the work is done, say exactly which subset is complete.
- If runtime behavior changed, include validation evidence in the final response.
