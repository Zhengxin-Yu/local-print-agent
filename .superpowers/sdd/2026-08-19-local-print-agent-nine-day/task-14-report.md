# Task 14 Report: 文档化和可复现交付

## 基线与范围

- BASE: `eba577ea9dc6db7ab13ea1ba77808c8930c43847`
- 本任务停止新增核心功能，只更新 README/API/演示/日报、双平台启动入口和缓存忽略规则。
- 提交：`08d843efe10a17bd0a255a83dc1fe3b2ffc39214` (`docs: finalize reproducible setup and demonstration`)。

## 交付物

- `README.md`：固定 12 个 H2 章节，覆盖双平台 demo/platform、配置、测试、错误和局限。
- `docs/api.md`：恰好 7 个接口，每个独立给出请求、响应、状态码和 PowerShell/curl 示例。
- `docs/demo-script.md`：8 段从 00:00 至 08:00 连续时间线，含 Windows/Bash 命令与证据边界。
- `scripts/run-windows.ps1`：显式 mode/browser/Sumatra/Go cache 参数，环境默认、路径和依赖检查。
- `scripts/run-linux.sh`：显式 `--mode`/`--browser-path`/`--go-cache`，CUPS 前置检查，相对浏览器路径在 `cd` 前规范化。
- `docs/reports/day-08.md`：回归、自检、必做对照、人工待办、建议录屏名和时间点。
- `.gitattributes`：固定 `*.sh` 为 LF，避免 Windows 打包破坏 Linux shebang。
- `.gitignore`：忽略审查代理留下的 `.tmp-*-gocache/`，不删除既有缓存。

## 干净启动验证

- 最终副本：`../local-print-agent-task14-clean-20260819-03`；复制器显式拒绝 `.git/`、`.worktrees/`、`.superpowers/`、`.tmp*`、`data/`。
- 严格按 README 启动后，服务监听 `http://127.0.0.1:17653`，`/health` 返回 `status=ok`、API `v1`，并发现 Mock Printer。
- 创建任务 `cb952a0b7ab5df015b599b3cbf3c6484`，最终状态为 `succeeded`；preview 返回 HTTP 200、`application/pdf`、36,972 bytes。
- 停止服务后端口已释放。全程 `demo`，未访问系统打印队列。

## 自动验证

- `go test ./... -count=1 -v`：133 顶层测试通过，0 失败，6 跳过。
- `go test -race ./... -count=1`：10 个含测试包通过，0 race。
- `go vet ./...`：exit 0，无输出。
- `go mod verify`：`all modules verified`。
- `git diff --check`：exit 0，只有 Git LF→CRLF 提示。
- Windows PowerShell AST、Git Bash `bash -n`、无效 mode/路径/依赖前置检查通过。
- 演示脚本中的 Worker 失败注入和 Service retry 聚焦命令实际通过。

## 审查

- 独立 Codex 代理只读模拟陌生 README 用户，不是同学或真人实测。
- 初审无 Critical，提出 Windows mode 环境默认、Linux 演示命令、第二终端 Go cache、API 必需字段和 Linux 相对浏览器路径问题，均已修改。复审又发现 Linux LF、解压包 Git 命令、race/curl 环境说明，修正后第三次复审结果为 Critical/Important/Minor 均无。

## 未完成的人工事项

1. 真实同学只看 README 启动、记录耗时/卡点、修文档后从新副本复测。
2. Windows 安全虚拟队列录屏，建议名 `day-08-windows-platform-20260819.mp4`。
3. Linux/CUPS runtime、request id/`lpstat -o` 及录屏，建议名 `day-08-linux-platform-20260819.mp4`。
详细时间点、必做对照和第 9 天材料清单见 `docs/reports/day-08.md`。

## Fix Round 1（2026-08-19）

### RED / GREEN

- 保留并提交既有回归测试 `docs/path_contract_test.go`。它用于阻止 README 和 `docs/**/*.md` 再引入机器绝对文件路径，不匹配 HTTP/API/reference URL。
- RED：`$env:GOTOOLCHAIN='local'; $env:GOCACHE="$env:TEMP\local-print-agent-path-contract-cache"; go test ./docs -run '^TestSubmissionDocumentsUseRelativeFilePaths$' -count=1 -v` 返回 exit 1；准确报告 README、`docs/demo-script.md`、`docs/testing.md`、`docs/reports/day-03.md`、`docs/reports/day-08.md` 中的 8 处绝对路径命中。
- GREEN：同一命令返回 exit 0，输出 `--- PASS: TestSubmissionDocumentsUseRelativeFilePaths`、`PASS` 和 `ok local-print-agent/docs`。

### 修正内容与文件

- `.gitignore`：新增 `.cache/`。
- `README.md`、`docs/demo-script.md`、`docs/testing.md`：浏览器/Sumatra/可选工具改用 `tools/` 下的项目相对示例，Go cache 改用 `.cache/` 下的项目相对示例。
- `docs/reports/day-03.md`：`pdf_path` 示例改为 `data/jobs/<jobID>/preview.pdf`。
- `docs/api.md`：第 5–7 节的 PowerShell 与 Bash 示例均独立定义 base 和 Job ID；preview 明确要求已有 ready PDF，retry 明确要求 failed Job。
- `docs/reports/day-08.md`：第 7 节改为“关键提交摘要”，记录已完成的干净副本 demo 证据，并保留真人同学启动、Windows 录屏、Linux/CUPS runtime/录屏为未完成。
- `docs/path_contract_test.go`：提交已执行 RED/GREEN 的文档路径契约回归测试。

### 完整验证

使用 `GOTOOLCHAIN=local`，并把 `GOCACHE` 指向已忽略的 `.cache/task-14-fix-verify` 后依次执行：

- `go test ./... -count=1`：exit 0；11 个含测试包通过，`templates` 无测试文件。
- `go test -race ./... -count=1`：exit 0；11 个含测试包通过，0 race，`templates` 无测试文件。
- `go vet ./...`：exit 0，无输出。
- `go mod verify`：exit 0，输出 `all modules verified`。
- `git diff --check`：exit 0；只有 LF→CRLF 工作区提示，无 whitespace error。

### 自审与关注项

- 手工扫描提交范围后，没有剩余机器绝对文件路径；HTTP URL、API route、reference URL 未改，运行时代码的浏览器发现路径和安全测试 fixture 未改。
- API 第 5–7 节逐节检查确认 `$base`/`BASE` 与 `$jobID`/`JOB_ID` 四项均存在。
- 干净副本证据与未完成人工事项没有互相替代：真人同学只看 README 启动、Windows 安全队列录屏、Linux/CUPS runtime 与录屏仍需外部环境完成。
- 本轮未向任何真实打印机或系统打印队列提交任务。
