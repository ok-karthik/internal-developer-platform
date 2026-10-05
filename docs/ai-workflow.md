# AI-assisted workflow: plan → execute → review

How changes to this repo are made with AI agents, why it is set up this way, and the real
defects it caught. Written so it doubles as interview preparation.

---

## 1. The loop in one picture

```
            ┌──────────────────────────────┐
            │ 1. PLAN  (strongest model)   │  measure the code, write docs/plans/<date>-<slug>.md
            └──────────────┬───────────────┘  facts · scope · rules · verify steps · "done means"
                           ▼
            ┌──────────────────────────────┐
            │ 2. EXECUTE  (faster model)   │  follow the plan exactly, one commit per part,
            └──────────────┬───────────────┘  honest report incl. "what I did NOT check"
                           ▼
            ┌──────────────────────────────┐
            │ 3. REVIEW  (strongest model, │  re-run checks, break the code on purpose,
            │    fresh context)            │  verify real output, fix small things
            └──────────────┬───────────────┘
                           ▼
            ┌──────────────────────────────┐
            │ 4. CLOSE THE LOOP            │  every real defect → a test, a repo rule,
            └──────────────┬───────────────┘  or a skill note, so it can't come back
                           ▼
                 human pushes / merges
```

| Role | Model | Job | Why that model |
|---|---|---|---|
| Planner | Opus | Measure, decide, write the plan | Wrong assumptions are most expensive here |
| Implementer | Sonnet | Execute the plan precisely | Cheaper and fast; the plan removes the judgement calls |
| Reviewer | Opus, new context | Check the work as if someone else wrote it | Fresh eyes catch what the author can't |
| Owner | human | Decide, push, merge | Agents never push or merge on their own |

## 2. Where it lives in this repo

| File | What it is |
|---|---|
| `.claude/agents/implementer.md` | Claude Code subagent definition: model `sonnet`, hard limits, points at the execute skill |
| `.claude/agents/reviewer.md` | Subagent definition: model `opus`, mutation checks, closes the loop |
| `.agents/skills/plan-task/SKILL.md` | How to write a plan (sections, measured facts, verify steps, known traps) |
| `.agents/skills/execute-plan/SKILL.md` | How to execute a plan and report honestly |
| `.agents/skills/review-work/SKILL.md` | How to review: diff vs plan, re-run, hunt usual failures |
| `docs/plans/*.md` | Every plan that was executed, kept as a record |
| `docs/plans/EXECUTION_LOG.md` · `PLAN.md` | What was done · what is open |
| `.agents/AGENTS.md` | Repo rules every agent reads first (`CLAUDE.md` points here) |

The skills are plain Markdown, so the same files also work for other tools (e.g. Gemini in
Antigravity reads `.agents/`). The owner's machine keeps the master copy in `~/.agents/skills/`;
the copies here are snapshots so readers of the repo can see them — re-copy after a skill changes.

## 3. Guardrails (the "harness")

The agents are only as safe as the rules around them. The ones that mattered here:

- **Executors never push, merge or publish.** A human does, after review.
- **Scope is a file list.** Anything outside it is a deviation that must be reported.
- **Work stays in the repo folder.** No worktrees or files elsewhere; temp output in temp dirs.
- **Never trust "done".** Real exit codes, real file trees, real rendered output.
- **Generated files are never hand-edited.** Change the template, regenerate.
- **Stage files by name**, so an agent never commits someone else's unrelated edits.

This setup work — agent definitions, skills, permissions, verify steps, git as the audit
trail — is sometimes called **harness engineering**.

## 4. What the reviews actually caught

Real defects from this repo's history. Each became a test, a rule or a skill note.

| Phase | What the reviewer found | How it was proven | Loop closed by |
|---|---|---|---|
| 21 | Gemini's draft: wrong ArgoCD app name, wrong service URL, per-env link in a per-service file, and a "regenerate" step that would silently do nothing (the CLI skips existing files) | Read the real ApplicationSet, ingress and `render.go` | Plan rewritten; `docs/plans/2026-10-04-backstage-catalog-metadata.md` lists each |
| 21 | The new Backstage test missed an empty `system:` line and a missing `/` on source-location | Broke the template 5 ways; 2 passed the test | Test hardened (`500e904`) |
| 22 | Command examples wrote generated files into the Go source folder | Ran them | README fixed (`2f12fa0`) |
| 24 | The "bad name writes nothing" test was blind to `../../x` — it only counted files inside the output folder | Moved validation after writing: 18 files leaked outside, test stayed green | Test counts one level higher (`cf64241`); skill note |
| 24 | The planner's "40 chars per name is enough" was wrong: three 40-char names → a **122-char** S3 bucket name (AWS max 63) | Rendered it | Joined-name cap in Phase 24b (`49aff2c`); skill note "compute the longest derived name" |
| 24 | Unknown capability left a half-made service (4 files) | Ran it | Up-front checks (`3ca70f6`) |
| 23 | Re-running the workflow after a closed test PR would fail with a confusing `git push` rejection | Fake remote in a scratch folder | `git ls-remote` pre-check (`cd2aa49`, PR #41); skill note "plan the re-run" |

The planner (Opus) made mistakes too — the 40-char reasoning and a broken verify command
(`git worktree add ... main` while `main` was checked out). The loop caught both. That is the
point: no single model is trusted; the *process* is.

## 5. Techniques worth knowing

- **Measured facts in the plan.** The executor never re-investigates, so cheap models stay on track.
- **Mutation checks.** Break the code on purpose; if the test still passes, the test is useless.
- **Report what you did NOT check.** Forces honesty and tells the reviewer where to look first.
- **Fresh context for review.** A new session doesn't share the author's blind spots.
- **Close the loop.** A defect fixed once but not turned into a test/rule will come back.

## 6. Agentic patterns in platform engineering (for interviews)

| Pattern | What it is | How it connects to this repo |
|---|---|---|
| Orchestrator–workers / evaluator–optimizer | Anthropic's names for "one model plans and hands out work" and "one model checks, work is fixed until it passes" | This loop is both |
| Agents use the golden path | Agents call the same validated CLI and templates as humans — never raw `kubectl apply` | `2-idp-scaffolder` validates names, runtime, capabilities before writing (Phases 24/24b) |
| MCP tools | Platform actions exposed as typed, permission-scoped tools for agents (Model Context Protocol) | Candidate: a small Go MCP server wrapping `add-service` / `onboard-tenant` |
| AI incident triage | Alert → agent reads the runbook and read-only cluster state → summary → human decides | Candidate: a read-only `triage` agent for the SLO alerts in Phase 14 |
| AI PR review in CI | A review agent comments on PRs alongside normal checks | The reviewer agent, run automatically |
| Guardrails for agents | Least-privilege identity, audit, human approval for writes | Same tenancy/RBAC thinking the platform already applies to teams |

**One-line CV wording:** *"Multi-agent plan/execute/review workflow with model tiering (Opus
plans and reviews, Sonnet executes), plan-scoped changes, mutation-tested reviews, and every
defect closed with a test or rule."*

## 7. Reading

- Anthropic — Building effective agents: https://www.anthropic.com/research/building-effective-agents
- Claude Code subagents: https://code.claude.com/docs/en/sub-agents
- Model Context Protocol: https://modelcontextprotocol.io
