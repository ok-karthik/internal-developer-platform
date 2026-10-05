---
name: execute-plan
description: Carry out a written plan (e.g. docs/plans/*.md) exactly, verify it on real output, and report honestly. Use when told "execute <plan file>" or when running as an implementer agent.
---

# Execute a written plan

**Model tier:** a fast, capable model (Claude Sonnet, or a Gemini Flash-class model in
Antigravity). The plan already holds the judgement; your job is precise execution.

## Rules
1. Read the repo rules (CLAUDE.md / AGENTS.md / GEMINI.md and anything they point to), then the
   **whole** plan, before editing anything.
2. Stay inside the plan's **Scope**. Do not "improve" nearby code.
3. Facts under "What was measured" are given — do not re-derive them.
4. One commit per part, tests green before each commit. Never push, merge or publish unless
   the plan says so.
5. If the plan is wrong or ambiguous for a part: do the safe minimum, and report the problem
   and your choice. Do not invent a large change.
6. **Verify the artifact, not the code**: run it, render it, open the output the user will see,
   and read it. Tests passing is necessary, not sufficient.

## Report back
- commit hashes, one line each
- what you checked, and how (command / screenshot path / numbers)
- **what you did NOT check** — say it plainly
- every place you deviated from the plan, with the reason
Never claim a check you did not run.
