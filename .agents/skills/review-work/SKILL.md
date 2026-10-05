---
name: review-work
description: Review another agent's (or model's) finished work with fresh eyes — diff against the plan, then verify the real output, especially what the executor did not check. Use after an executor reports done, before merge/publish, or when the user says "review what Sonnet/Gemini did".
---

# Review finished work

**Model tier:** the strongest model available (Claude Opus / Gemini Pro-class). A different
model or session from the one that wrote the code — fresh context is the point.

## Steps
1. **Diff vs plan.** Read the full diff. Anything outside scope? Anything in the plan missing?
2. **Run the checks yourself.** Tests, lint, build. Do not trust the report's numbers.
3. **Verify the output the user sees.** Render pages, run scripts, read results. Start with what
   the executor said it did NOT check.
4. **Hunt the usual failures:**
   - two numbers on one page that count different populations
   - prose with numbers or lists typed in by hand (they go stale)
   - untrusted strings inserted as HTML; links built from unvalidated data
   - string replaces / config edits that silently matched nothing
   - a "fix" whose real effect was never measured
5. **Measure any claim before repeating it** to the user.

## Then
- Report findings ranked by impact, in plain words, with the evidence.
- Fix small, clear defects yourself; write a plan (plan-task) for bigger ones.
- **Close the loop:** turn each real defect into a test, a rule in AGENTS.md, or a note in the
  relevant skill, so it cannot come back.
