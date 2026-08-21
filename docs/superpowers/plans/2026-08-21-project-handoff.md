# Project Handoff Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a truthful, repository-relative `PROJECT_HANDOFF.md` that lets Code Buddy continue the remaining course delivery work without prior conversation context.

**Architecture:** The handoff is a single root-level Markdown entry point backed by existing README, API, testing, demo, final-report, and source files. It separates implemented code from recorded evidence and unfinished platform validation, then gives each next task explicit files, acceptance criteria, risks, and prohibited claims.

**Tech Stack:** Markdown, Git, Go document contract tests, PowerShell validation commands.

## Global Constraints

- Work directly on the current `main` branch because the user declined worktree isolation for this task.
- Use repository-relative paths only; do not include absolute user paths, secrets, accounts, remote repository URLs, or fabricated evidence.
- Treat Day 9 test counts as recorded historical evidence unless the same commands are rerun in this session.
- Never equate Fake Adapter acceptance, platform command success, system-queue acceptance, and physical output.
- Platform verification must be opt-in and use an explicitly confirmed safe virtual or isolated queue.
- Keep the current loopback, port, state-machine, FIFO, path, command-execution, redaction, and at-least-once invariants explicit.

---

### Task 1: Write the root handoff document

**Files:**
- Create: `PROJECT_HANDOFF.md`
- Delete: `CODE_BUDDY_HANDOFF.md`
- Reference: `README.md`
- Reference: `docs/api.md`
- Reference: `docs/testing.md`
- Reference: `docs/demo-script.md`
- Reference: `docs/reports/day-09-final.md`

**Interfaces:**
- Consumes: Existing repository behavior and recorded course evidence.
- Produces: One repository-relative handoff entry point for Code Buddy.

- [x] **Step 1: Replace the temporary draft with the approved document structure**

  Create `PROJECT_HANDOFF.md` with project status, completed modules, evidence levels, P0/P1/P2 work, startup and verification commands, invariants, known dependencies, takeover order, definition of done, and a copy-ready Code Buddy prompt. Remove `CODE_BUDDY_HANDOFF.md` because it contains absolute local paths and unsupported fresh-verification wording.

- [x] **Step 2: Self-review every completion claim**

  Check that Windows controlled-runner evidence is not described as SumatraPDF/system-queue runtime evidence; Linux cross-compilation is not described as CUPS runtime evidence; Day 7 `context canceled` remains stated; and no Windows/Linux system-queue recording or README-only human test is claimed.

- [x] **Step 3: Check every referenced repository path**

  Run a PowerShell path-existence check over every backticked path intentionally cited as a file or directory, and fix missing or ambiguous paths.

### Task 2: Validate and commit the handoff

**Files:**
- Test: `PROJECT_HANDOFF.md`
- Test: `docs/doc_contract_test.go`

**Interfaces:**
- Consumes: The completed `PROJECT_HANDOFF.md` from Task 1.
- Produces: A committed, verified handoff document with explicit evidence boundaries.

- [x] **Step 1: Scan for prohibited content and unfinished markers**

  Search the document for drive-letter paths, remote web URLs, secret-like field names, and unfinished-marker strings. Review all matches and remove anything not required by the project description.

- [x] **Step 2: Run focused document tests**

  Run: `go test ./docs -count=1`

  Expected: the `docs` package passes.

- [x] **Step 3: Run the full ordinary test suite**

  Run: `go test ./... -count=1`

  Expected: all runnable packages pass; any environment-dependent skips remain explicitly distinguished from passes.

- [x] **Step 4: Check patch formatting**

  Run: `git diff --check`

  Expected: exit code 0 with no whitespace errors.

- [x] **Step 5: Commit the final handoff**

  Run:

  ```powershell
  git add -- PROJECT_HANDOFF.md CODE_BUDDY_HANDOFF.md docs/superpowers/plans/2026-08-21-project-handoff.md
  git commit -m "docs: add Code Buddy project handoff"
  ```
