# Final Review Fix Report

## Scope and Base

- FIX_BASE: `13c8f587820136b844ea78fcbbeab0dd4a3ee2e9`.
- This is the single allowed final-review fix wave.
- The work used only demo/Fake Printer behavior and controlled tests. It did not enable platform mode or submit to any real or system print queue.
- The source plan is named `2026-08-19-local-print-agent-nine-day.md`.

## Root Causes

### DataDir ownership

`startWithBuilder` previously called the application builder before taking any process-level ownership of `DataDir`. The builder opens `data/jobs.json`, runs interrupted-job recovery, and resumes queued jobs. A second process could therefore fall back to another port while sharing the same snapshot and queue semantics.

The fix adds `internal/instance/` with a common lifetime handle and build-tagged implementations:

- Windows uses non-blocking exclusive `windows.LockFileEx`.
- Unix uses non-blocking exclusive `unix.Flock`.
- `instance.Acquire` creates only the DataDir and lock file, returns the stable `ErrAlreadyRunning` contention error, and keeps the file descriptor open until `Close`.

`startWithBuilder` now acquires this lock before token generation, builder, Store open, recovery, queue restoration, and listener creation. A startup guard releases it on every early return. Successful startup transfers ownership to the completion goroutine, which releases only after HTTP shutdown completion and `Worker.Done`.

### Null-origin authorization

The old Router treated `Origin: null` as sufficient proof that a request came from the optional local file page. Opaque origins are not unique to the intended file, so another opaque origin could receive localhost CORS.

Each launch now generates 32 random bytes with `crypto/rand` and encodes them with raw URL-safe Base64. The value is passed through internal config to Router dependencies, returned on the internal `runningServer`, and exposed to the operator only through the local console's optional `web/index.html?local_print_agent_token=<per-launch-token>` instruction.

For health and API paths, null-origin CORS is returned only when the query capability matches. Router comparison hashes both values to fixed-size SHA-256 digests and uses `subtle.ConstantTimeCompare`. Missing, wrong, or empty configured capabilities receive no CORS. Embedded Web remains same-origin and needs no capability.

The file-mode JavaScript reads the capability only on the `file:` protocol and routes health, every API fetch, and preview links through one `servicePath` helper. The capability is not added to Job objects or error payloads.

### Shutdown completion

`http.Server.Serve` returns when shutdown begins, not when active handlers finish. The old `running.Done` waited for that early Serve result and the Worker, but not for `httpServer.Shutdown` itself.

Startup now has independent buffered `httpServeDone` and `httpShutdownDone` result channels. Completion waits for Serve, cancels the shared service context, waits for the bounded Shutdown call, waits for `Worker.Done`, releases the instance lock, and only then sends `running.Done`. Shutdown failures remain generic in logs and are returned without application filesystem details.

### UI printer claim

The embedded page unconditionally claimed that no real printer was connected, even when explicit platform mode supplied operating-system queues.

The shipped asset now states both behaviors: demo uses Mock Printer without physical submission; platform submits to the selected OS queue and requires a confirmed safe target. The page does not need mode plumbing because the warning is unambiguous in both modes.

### Documentation path coverage

The previous submission test scanned README plus `docs/` only and rejected absolute paths, but did not inspect hidden `.superpowers` Markdown or parent-directory escapes. Historical reports retained one machine tool path, an external clean-copy directory name, and Day 5 links that climbed out of their local report directory.

The scan now walks all project Markdown from the repository root, including `.superpowers`, while excluding only VCS, dependency, cache, temporary, and runtime-data directories. Fixtures cover drive paths, UNC paths, home shorthand, common POSIX/macOS roots, leading and prefixed parent-directory escapes in both slash directions, and allowed HTTP/API/reference/prose cases.

## TDD Evidence

### Instance lock primitive

RED:

```text
go test ./internal/instance -run TestAcquirePreventsConcurrentUseAndReleaseAllowsReacquisition -count=1 -v
undefined: Acquire
undefined: ErrAlreadyRunning
FAIL local-print-agent/internal/instance [build failed]
```

GREEN:

```text
=== RUN   TestAcquirePreventsConcurrentUseAndReleaseAllowsReacquisition
--- PASS: TestAcquirePreventsConcurrentUseAndReleaseAllowsReacquisition
PASS
ok local-print-agent/internal/instance
```

### Startup lock lifecycle

RED command:

```text
go test ./cmd/local-print-agent -run 'TestStartWithBuilder(RejectsSecondSameDataDirBeforeBuilder|ReleasesLockWhenBuilderFails|ReleasesLockWhenListenerFails)$' -count=1 -v
```

RED observations:

- The second start returned no error and invoked its builder.
- Builder failure did not observe a pre-builder lock.
- Listener failure did not observe a lock during listener creation.

GREEN: all three tests passed. The tests also proved reacquisition after builder failure, listener failure, and graceful `running.Done`.

### File-origin CORS

RED:

```text
go test ./internal/httpapi -run 'Test(FileOriginCORSRequiresLaunchCapability|EmptyFileOriginCapabilityNeverEnablesNullOriginCORS)$' -count=1 -v
unknown field FileOriginToken in struct literal of type Dependencies
FAIL local-print-agent/internal/httpapi [build failed]
```

GREEN: eight correct/wrong/missing GET/preflight/web-origin/static-path cases passed, and empty Router configuration denied null-origin CORS.

### Per-launch token generation

RED:

```text
go test ./cmd/local-print-agent -run TestStartWithBuilderGeneratesDistinctFileOriginCapabilities -count=1 -v
cfg.FileOriginToken undefined
running.FileOriginToken undefined
FAIL local-print-agent/cmd/local-print-agent [build failed]
```

GREEN: two simultaneous launches with different DataDirs received nonempty, builder-visible, running-server-visible, distinct capabilities.

### Browser propagation

RED:

```text
go test ./web -run TestAppJavaScriptPropagatesFileOriginCapabilityToRequestsAndPreview -count=1 -v
Error: file console did not connect with its capability
FAIL local-print-agent/web
```

GREEN: the Node VM harness connected in `file:` mode, exercised health, printers, list, detail, retry, create, and preview, and verified the capability on every request or link. The existing same-origin behavior test remained GREEN.

### HTTP shutdown ordering

RED:

```text
go test ./cmd/local-print-agent -run TestRunningDoneWaitsForActiveHTTPHandlerAndShutdownCompletion -count=1 -v
running.Done closed before HTTP shutdown completed: <nil>
FAIL local-print-agent/cmd/local-print-agent
```

GREEN: cancellation left Done open while a real handler blocked; after release, the full response completed, Done closed, the port rebound, and the DataDir lock was reacquired.

### Platform warning

RED:

```text
go test ./web -run TestEmbeddedConsoleWarningDistinguishesDemoAndPlatformPrinting -count=1 -v
embedded console warning lacks "演示模式"
FAIL local-print-agent/web
```

GREEN: the embedded asset contains the demo/Mock Printer and platform/OS queue safety distinction and no unconditional no-print claim.

### Markdown path contract

RED stages:

1. `TestRepositoryEscapingPathPatterns` first failed to compile because no escape detector existed.
2. The expanded project scan then reported six project files: the SDD ledger, Task 8 report, Task 14 report, Day 5 report, and Day 8 report, with Task 14 containing two categories of prohibited reference.
3. Added prefixed repository escape fixtures then failed for both slash directions, proving the first detector handled only paths beginning with a parent segment.

GREEN:

```text
go test ./docs -run 'Test(AbsoluteFilePathPatterns|RepositoryEscapingPathPatterns|SubmissionDocumentsUseRelativeFilePaths|FinalReportHasNoLocalMarkdownDestinations)$' -count=1 -v
PASS
ok local-print-agent/docs
```

The final scan includes this report and all other project Markdown.

## Changed Files

Production and runtime:

- `internal/instance/lock.go`
- `internal/instance/lock_windows.go`
- `internal/instance/lock_unix.go`
- `cmd/local-print-agent/main.go`
- `internal/config/config.go`
- `internal/httpapi/router.go`
- `web/app.js`
- `web/index.html`

Regression tests:

- `internal/instance/lock_test.go`
- `cmd/local-print-agent/main_test.go`
- `internal/httpapi/web_test.go`
- `web/app_behavior_test.go`
- `web/embed_test.go`
- `docs/path_contract_test.go`

Submission documentation:

- `README.md`
- `docs/api.md`
- `docs/testing.md`
- `docs/demo-script.md`
- `docs/reports/day-04.md`
- `docs/reports/day-05.md`
- `docs/reports/day-08.md`
- `docs/reports/day-09-final.md`
- `.superpowers/sdd/2026-08-19-local-print-agent-nine-day/ledger.md`
- `.superpowers/sdd/2026-08-19-local-print-agent-nine-day/task-8-report.md`
- `.superpowers/sdd/2026-08-19-local-print-agent-nine-day/task-14-report.md`
- `.superpowers/sdd/2026-08-19-local-print-agent-nine-day/final-review-fix-report.md`

No task brief was rewritten. Historical Day 4 and Task 8 reports record the later capability hardening instead of changing the original requirement.

## Verification

Focused packages:

```text
go test ./internal/instance ./cmd/local-print-agent ./internal/httpapi ./web ./docs -count=1 -v
PASS for all five packages
```

Full suite:

```text
go test ./... -count=1 -v
exit 0
```

JSON-derived top-level counts from a separate fresh run:

```text
go_exit=0 top_total=152 pass=146 skip=6 fail=0 package_pass=12 package_fail=0
```

The six skips are existing environment/opt-in boundaries:

- `TestRealServiceRendersBothJobsServesPreviewAndCleansUp`
- `TestAPIPreviewRejectsSymlinkEscapeWhenSupported`
- `TestFakeRendererPassesConfiguredPDFInfo`
- `TestPDFRendererChromeIntegration`
- `TestPDFRendererRejectsPreexistingSymlinkJobDirectory`
- `TestNewPDFRendererRejectsSymlinkOutputWithoutCreatingJobsAtTarget`

Race and static/module checks:

```text
go test -race ./... -count=1
12 tested packages passed; 0 race reports; templates has no test files

go vet ./...
exit 0; no output

go mod verify
all modules verified

git diff --check
exit 0; only Windows line-ending conversion warnings, no whitespace errors

git diff --cached --check
exit 0; no output
```

The repository-wide Markdown path test was rerun after this report was created and passed. Its scan covers this file, every submitted `.superpowers` report, README, and all other project Markdown.

Linux cross-compilation:

```text
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/instance
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c ./cmd/local-print-agent
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/local-print-agent
all exit 0
```

The outputs were created under `.tmp-final-review-cross/`, inspected as exactly `instance.test`, `main.test`, and `local-print-agent`, then all three files and the directory were removed. No script changed, so PowerShell AST and Git Bash syntax checks were not required for this wave.

## Documentation Reconciliation

- README and API docs make embedded same-origin Web the default and describe the per-launch capability only for optional file mode.
- Testing and demo docs cover correct/wrong/missing capability, JS propagation, single-instance ownership, and shutdown completion.
- The UI warning is truthful in demo and platform modes.
- The final report architecture includes the pre-Store instance lock, explicit Shutdown completion, capability security boundary, new regression tests, and measured 152/146/6/0 counts.
- External clean-copy evidence is described without an external filesystem name.
- Day 5 fixtures are written as repository-root-relative inline-code paths.
- SDD ledger and reports contain no machine-absolute or repository-escaping paths.

## Self-Review

Lock release paths:

- Nil context and nil builder return before lock acquisition.
- Lock acquisition failure returns without builder invocation.
- Random generation, builder, and listener failures run the startup guard.
- Successful startup disables the guard only after all goroutines and the running handle are ready.
- Normal completion waits Serve, Shutdown, Worker, then closes the lock.
- Lock `Close` is idempotent and joins unlock/close errors.

Shutdown and deadlock ordering:

- Serve and Shutdown each have a buffered one-result channel, so neither completion goroutine depends on the receiver being scheduled first.
- Serve cancellation starts Shutdown on unexpected server exit; outer context cancellation starts Worker and HTTP shutdown together.
- Completion waits both HTTP signals before Worker and lock release.
- The real blocking-handler test covers the previous early-Done race and verifies response/port/lock completion.

Capability behavior:

- Empty direct-builder capability is deny-by-default.
- Missing and wrong values receive no `Access-Control-Allow-Origin` on GET or preflight.
- Correct token is limited to health and versioned API paths, GET/POST methods, and `Content-Type` preflight header.
- Same-origin pages ignore any token-like query and use unchanged same-origin URLs.
- File-mode health, list, detail, printers, create, retry, and preview all use `servicePath`.
- The capability is absent from persisted jobs and API error construction; the only intentional operator exposure is the local console instruction.

UI and documentation:

- Static wording does not claim platform mode is non-printing.
- Submission path scan includes hidden `.superpowers` Markdown and this report.
- HTTP and reference URLs, API routes, project-root-relative paths, and ordinary punctuation remain allowed.
- Test counts in the current final report match the fresh JSON run.

## Remaining Concerns and External Gaps

- The advisory lock prevents cooperating application instances from sharing one DataDir; it does not protect against a hostile process that ignores the lock and edits runtime files directly.
- The per-launch capability narrows optional null-origin CORS. It is not user authentication and does not prevent a local same-account process from calling the loopback API directly.
- The existing five-second HTTP shutdown timeout remains. A timeout is surfaced as a generic shutdown error rather than an internal path-bearing diagnostic.
- Linux lock/main compilation succeeded, but no Linux kernel or CUPS runtime was available in this wave.
- Classmate-only README startup, Windows safe platform queue evidence, Linux/CUPS request evidence, and both platform recordings remain incomplete. No automated result is used as a substitute.
- Six opt-in or privilege-dependent tests remain skipped and are not counted as passes.
