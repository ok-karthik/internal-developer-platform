---
name: plan-task
description: Write an execution plan that another agent or a cheaper model will carry out. Use when the user asks for "a plan", "write it up for Sonnet/Flash to execute", or when work is big enough to split into plan → execute → review. Measure first, then write a plan that needs no judgement calls from the executor.
---

# Plan a task for another agent to execute

**Model tier:** the strongest model you have (Claude Opus, or a Gemini Pro-class model in
Antigravity — pick it with `/model` before starting). Planning is where a wrong assumption is
most expensive.

## 1. Measure before writing
- Run the queries, read the code paths, open the real output. Every fact the executor will
  rely on goes in the plan **with its date, population and n**, under "What was measured —
  do not re-derive". The executor should never re-investigate.
- Read real examples in BOTH directions before proposing a rule change (what it catches,
  what it would wrongly catch).
- Note the decisions you made and why, so the executor does not reopen them.

## 2. Write the plan
Path: the repo's plan folder (`docs/plans/YYYY-MM-DD-<slug>.md` if none exists). Sections:

1. **Header** — who executes, branch, one commit per part, what NOT to do (push, publish, merge).
2. **Scope** — the exact files that may change. Everything else is out of bounds.
3. **What was measured** — table of facts, dated.
4. **Parts** — each with the exact change, the anchor/function names, edge cases, and the tests to add.
5. **Rules that this task can break** — copy the 4–6 that matter from the repo rules; do not just link.
6. **Verify sequence** — commands, plus what to LOOK at in the real output (not just "tests pass").
   Dry-run any command you have not run yourself. Known trap: `git worktree add <path> main`
   fails when `main` is checked out anywhere (usually the main checkout) — for a read-only
   baseline use `git worktree add --detach <path> main`.
   For a "bad input writes nothing" test, count files one level ABOVE the output dir —
   path traversal (`../../x`) writes outside it, so counting only inside proves nothing.
   When a limit depends on lengths, compute the longest DERIVED name (e.g. `<tenant>-<app>-<env>`
   in templates), not just each input on its own.
   For any job that creates a branch, PR or other named remote object, plan the RE-RUN:
   what happens when that name already exists from an earlier (failed or closed) run?
7. **Done means** — checkable bullets, and what to report back (commits, what was checked, what was not).
8. **Out of scope** — things discussed and deliberately left out.

## 3. Hand off
Commit the plan (or tell the user it is uncommitted). Name the executor tier in the header.
