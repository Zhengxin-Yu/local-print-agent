# 第8天：回归、审查、自启动与交付准备

## 1. 最终回归摘要和自动测试输出

验证日期：2026-08-19；主机：Windows amd64；Go：`go1.25.4 windows/amd64`；module 指定 `go 1.23.0`。本轮所有 Go 命令从已忽略的项目相对目录 `.cache/` 解析出运行时所需绝对 `GOCACHE`，默认只使用 Fake Printer 或受控 command runner，没有向系统或实体打印队列提交任务。

| 命令 | 结果 | 摘要 |
| --- | --- | --- |
| `go test ./... -count=1 -v` | 完成 | 141 个顶层测试完成：135 通过、0 失败、6 跳过；11 个含测试包通过，`templates` 无测试文件。 |
| `go test -race ./... -count=1` | 完成 | 11 个含测试包通过，0 race；`templates` 无测试文件。 |
| `go vet ./...` | 完成 | exit 0，无输出。 |
| `go mod verify` | 完成 | `all modules verified`。 |
| `git diff --check` | 完成 | exit 0；仅有 Git 的 LF→CRLF 工作区提示，不是 whitespace error。 |
| PowerShell AST parse | 完成 | `scripts/run-windows.ps1` 无语法错误。 |
| Git Bash `bash -n` | 完成 | `scripts/run-linux.sh` 无语法错误。 |

6 个跳过项的边界与 Day 7 一致：真实 Chrome/服务 E2E 需要显式 opt-in，Fake PDF 的外部 `pdfinfo` 需要显式 opt-in，另有 3 个 Windows 普通账户无法创建 symlink 的安全场景。Node.js 22 已安装，`TestAppJavaScriptRunsDiscoveryRenderingAndTimerLifecycle` 本轮真实执行通过。

## 2. 审查自检

| 项目 | 结论 | 核对依据 |
| --- | --- | --- |
| 范围 | 完成 | 本日只改交付文档、启动脚本和缓存忽略规则，没有新增 API、状态或打印核心功能。 |
| 依赖 | 完成 | `go.mod`/`go.sum` 未改，`go mod verify` 通过；README 区分必需和可选依赖。 |
| 硬编码 | 完成 | 固定 loopback、17653–17660 和 `data/` 是代码现状并已明确写入 README；浏览器、Sumatra 和 Go cache 可显式传入。 |
| 接口一致性 | 完成 | `docs/api.md` 只列 `router.go` 的 7 个接口，字段使用真实 `source_code`，状态码来自 handler 与 API 测试。 |
| 安全 | 完成 | README/演示默认 `demo`，平台模式带明确警告；文档不接受客户端文件路径，不把 preview/Fake/交叉编译写成 OS 打印。 |
| 日志 | 完成 | 启动脚本输出 mode 与是否可能进入系统队列；应用仍不记录完整源码或敏感内部诊断。 |
| 异常处理 | 完成 | 脚本拒绝未知 mode、无效显式浏览器路径；Windows platform 要求 Sumatra，Linux platform 要求 `lp`/`lpstat`；受限 Go cache 可显式配置。 |

启动脚本的 TDD/故障修正证据：旧脚本没有 mode 参数，静态契约先失败；实现后参数和语法通过。第一次干净启动发现默认 Go build cache `Access is denied`，随后增加 `-GoCachePath`/`--go-cache` 并用显式目录执行 `go list` 成功。另一个红灯证明 demo 曾错误校验无关的旧 Sumatra 环境值；修正后 demo 忽略它，platform 仍严格要求 Sumatra。

## 3. 他人启动记录

**真人同学仅看 README 启动：未完成。** 当前会话无法邀请或观察一位真实同学，不能把实现者自测或代理审查伪造成他人实测。

| 记录项 | 当前结果 |
| --- | --- |
| 使用环境 | 待记录：OS/版本、Go、Chrome/Chromium、是否已有 module cache。 |
| 启动耗时 | 待记录：从拿到 README 到页面显示“已连接”，再到首个任务创建成功。 |
| 卡点 | 待由真实同学原话记录。 |
| 文档修改 | 本轮实现者干净启动发现并补充 Go cache 参数；这不算同学反馈。 |
| 复测结果 | 待同一人只按修订后的 README 再测，不口头补充。 |

执行清单：

1. 给同学一个不含 `.git/`、`.worktrees/`、`.superpowers/`、`.tmp*`、`data/` 的干净副本，只提供 README。
2. 不口头提示；记录每次停顿、查找和报错原文。
3. 成功标准是打开页面、看到 health/printer，并创建一个 demo 任务；不得把平台队列设为默认验收目标。
4. 按卡点修 README 后，让同一人从新副本重新开始，保存总耗时和结果。

独立 Codex 子代理完成了“陌生读者”只读审查；它不是同学、不是人工可用性测试，其结论只可列为自动文档审查。两轮修正后的最终复审结果为 Critical、Important、Minor 均无。

## 4. 必做对照表

| 必做项 | 状态 | 证据路径/事实 |
| --- | --- | --- |
| README 固定 12 章节 | 完成 | `README.md`；静态检查逐项匹配项目简介至参考项目。 |
| 7 个 API 独立记录请求、响应、状态码和双端示例 | 完成 | `docs/api.md`；章节数静态检查为 7，字段/状态与 `internal/httpapi` 测试核对。 |
| 固定顺序 8 分钟演示脚本 | 完成 | `docs/demo-script.md`；8 段时间点从 00:00 连续至 08:00。 |
| Windows/Linux 脚本显式 demo/platform | 完成 | `scripts/run-windows.ps1`、`scripts/run-linux.sh`；AST/`bash -n`、非法参数和依赖前置检查。 |
| Linux 脚本 LF 交付 | 完成 | `.gitattributes` 固定 `*.sh text eol=lf`，避免 Windows 打包后 shebang 变为 `bash\r`。 |
| 干净副本不带版本库/缓存/数据 | 完成 | 一个未提交的外部干净副本有 82 个文件；复制器显式拒绝 `.git/.worktrees/.superpowers/.tmp/data/.cache`，README SHA-256 与当前工作树一致。demo 运行时按预期新建 `.cache/` 与 `data/`；该副本是本机临时验证物，不提交。 |
| 只按当前 README 启动至 health/打印机/创建/预览 | 完成 | 严格执行不含 `-BrowserPath` 的 demo 主命令；服务自动发现浏览器并监听 `http://127.0.0.1:17653`；`/health` 返回 `service=local-print-agent`、`status=ok`、API `v1`；发现 Mock Printer；合法新建任务 `809b3c736979ffe2ac9466441f8c4fa2` 最终为 `succeeded`、`attempts=1`；preview 返回 HTTP 200、`application/pdf`、35,180 bytes；停止后进程退出且端口可重新绑定。全程未访问系统打印队列。 |
| 真人同学只看 README 启动 | 未做 | 无真人参与；见第 3 节待执行清单。 |
| Windows 系统队列录屏 | 未做 | 无 SumatraPDF/已确认自动保存虚拟队列；未向系统队列提交。 |
| Linux/CUPS 录屏和 runtime 测试 | 未做 | 本机无 Linux runtime/CUPS；已有交叉编译不能替代。 |
| 自动回归/race/vet/mod/diff | 完成 | 本报告第 1 节的本轮命令输出。 |

该干净副本验证只证明 README demo 主路径可复现，不替代真人同学启动、Windows 系统队列、Linux/CUPS runtime 或双平台录屏证据；这些外部人工事项仍保持未完成。

## 5. Windows、Linux 录屏文件名和关键时间点

当前没有录屏文件。以下是建议文件名和必须采集的时间点，不代表文件已经存在：

| 平台 | 建议文件名 | 建议关键时间点 |
| --- | --- | --- |
| Windows | `day-08-windows-platform-20260819.mp4` | `00:00` `winver`/Go/Chrome/Sumatra；`00:25` platform 启动命令；`00:50` `/health` 与系统队列枚举；`01:15` 气球任务提交；`01:45` 状态与 preview；`02:15` 已确认虚拟队列/自动保存输出；`02:40` 明确“队列接受≠物理出纸”。 |
| Linux | `day-08-linux-platform-20260819.mp4` | `00:00` `uname -a`/Go/Chrome/CUPS；`00:25` `lpstat -p/-d`；`00:50` platform 启动与 health；`01:15` 源码任务提交；`01:50` 多页 preview；`02:20` `lpstat -o` 与 request id；`02:45` 边界说明。 |

录制前必须确认虚拟队列可自动保存且不会弹出阻塞对话框；没有安全目标就只录 demo，不得为了证据向实体打印机试投。

## 6. 项目启动、测试、演示命令汇总

Windows demo：

```powershell
.\scripts\run-windows.ps1 -Mode demo -GoCachePath '.cache\go-build'
```

Linux demo：

```bash
./scripts/run-linux.sh --mode demo --go-cache .cache/go-build
```

平台模式仅在安全队列确认后执行，完整参数见 README。回归：

```powershell
$env:GOTOOLCHAIN = 'local'
$cacheDir = New-Item -ItemType Directory -Force '.cache\go-test'
$env:GOCACHE = $cacheDir.FullName
go test ./... -count=1 -v
go test -race ./... -count=1
go vet ./...
go mod verify
if (Test-Path -LiteralPath '.git') { git diff --check }
```

无 `.git` 的解压包跳过最后一条；README 和演示脚本已给出 PowerShell/Bash 条件执行写法。

演示操作和逐分钟讲解见 `docs/demo-script.md`；API 命令见 `docs/api.md`。

## 7. 最终目录树和关键提交摘要

交付目录（运行时 `data/`、Go cache、`.git/` 和 `.superpowers/` 不在交付树中）：

```text
local-print-agent/
├── README.md
├── cmd/local-print-agent/
├── internal/{config,httpapi,jobs,printer,render,server,store,worker}/
├── templates/
├── web/
├── testdata/
├── scripts/{run-windows.ps1,run-linux.sh}
├── docs/
│   ├── api.md
│   ├── demo-script.md
│   ├── testing.md
│   └── reports/{day-01.md,...,day-08.md,assets/}
├── go.mod
└── go.sum
```

功能阶段关键提交：

| 阶段 | commit | 说明 |
| --- | --- | --- |
| Bootstrap | `527ed12`、`cd01ef5` | 默认配置、health 和候选端口。 |
| 任务模型 | `9775aca`、`098ceb2` | 请求校验与状态机。 |
| 存储/Worker | `1b1139f`、`7798e27` | JSON Store 与 FIFO 主路径。 |
| HTTP/Web | `5eae3d0`、`82ddf4` | API 与 Mock Web。 |
| 渲染 | `9ce7293`、`5c479b4` | 两类 HTML、Chroma 和 Chrome PDF。 |
| 平台适配 | `cc63dd1`、`0ae9e15` | Windows SumatraPDF、Linux CUPS。 |
| 最终回归基线 | `eba577e` | Task 13 边界/并发/故障回归。 |

Day 8 文档与脚本属于本次 `docs: finalize reproducible setup and demonstration` 提交；为避免 commit 自引用，不在提交内容中硬编码自身 hash，使用 `git log -1 --format='%H %s'` 获取最终值。

## 8. 第 9 天只整理报告

第 9 天不再新增功能或修改接口，只整理以下材料：

- 设计说明与架构图；
- README、API 和 8 分钟演示脚本；
- Day 1–8 报告及 Day 5 真实 PDF 截图；
- 最终自动测试、race、vet、module 与 diff 摘要；
- 真人同学仅看 README 的原始记录（当前待补）；
- Windows/Linux 安全虚拟队列录屏与关键时间点（当前待补）；
- Linux runtime/CUPS request id 与队列截图（当前待补）；
- 关键提交摘要、最终目录树和未完成边界说明。

若人工材料未能在提交截止前补齐，最终报告必须继续标注未完成，不能以 Fake Printer、受控 runner、交叉编译、历史截图或代理审查替代。
