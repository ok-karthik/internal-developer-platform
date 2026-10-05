# ADR 0016: Backstage Dispatches Scaffolding to GitHub Actions

**Date:** 2026-10-04
**Status:** Accepted

## Context

Backstage needs a way to run the Go scaffolder (ADR 0015) on behalf of a developer. Four options were weighed:

- **(a) Reimplement scaffolding in TypeScript.** High cost, throws away the Go engine, and gives a second place to change every template field. Rejected.
- **(b) A custom Backstage action (`idp:run-cli`) that runs the CLI.** Needs TypeScript, the Go binary inside the Backstage image, and credentials to push to this repo stored in Backstage.
- **(c) A REST API in front of the CLI.** 2-3 days of work and a service to host. Not built (ADR 0015 says when to add it).
- **(d) Backstage's built-in `github:actions:dispatch` starts a GitHub Actions workflow.** No TypeScript, and the workflow already has a checkout, Go, and a token scoped to this repo.

## Decision

Use (d). The Backstage template has one step: dispatch `.github/workflows/scaffold-service.yaml` with the form values. The workflow validates the inputs, runs `add-service` against a checkout, and opens a pull request from branch `scaffold/<tenant>-<app>`. It never pushes to `main`. Environment is fixed to `dev` (ADR 0003, ADR 0007).

## Consequences

- No TypeScript and no push credentials in Backstage. Backstage only needs a token that can start workflows (Actions: read and write).
- Backstage reports "started", not "succeeded" (backstage/backstage issues #29727, #33104). The template shows links to the workflow runs and to the PR search so the user can follow the result.
- A PR opened with `GITHUB_TOKEN` does not trigger `pull_request` workflows, so `ci.yaml` does not run on it by itself. Close and reopen the PR, or later use a GitHub App token.
- The repo setting "Allow GitHub Actions to create and approve pull requests" must be on, or the PR step fails.
- Names are checked twice: by the workflow before it uses them in paths and branch names, and by the CLI (Phase 24, `nameRule`). Both use one identical rule: `^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$`.
- The golden-path list in the template and the workflow is copied by hand from `catalog.yaml`.

## Revisit when

A platform API exists (then a thin custom action calls it), or many templates need live progress inside Backstage.
