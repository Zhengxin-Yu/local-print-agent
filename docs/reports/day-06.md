# 第6天：两类打印模块完成说明与自测

> 本日完成 Windows 与 Linux 平台适配代码及主程序选择接线。“成功”仍只表示被所选 adapter 接受；Fake、受控命令 runner、PDF 预览和交叉编译都不等于操作系统队列接受，更不等于物理出纸。本机没有安全可用的自动保存打印目标，因此没有向实体或交互式虚拟打印机提交任务。

## 气球小票模块

输入字段为比赛名、队伍编号、队伍名称、房间、题号、气球颜色和通过时间。`balloon_ticket.html.tmpl` 使用 80 mm × 120 mm 窄纸、4 mm 边距，动态文本由 Go HTML 模板转义。完整步骤为：API 校验并排队 → Worker 进入 `rendering` → 生成 `data/jobs/<32位小写十六进制任务ID>/preview.pdf` → 进入 `printing` → 所选平台 adapter 重新枚举并精确匹配打印机 → 提交 PDF → adapter 接受后进入 `succeeded`。

Task 10 的 Chrome 验收已经证明气球 PDF 为一页，截图证据为 `docs/reports/assets/day-05-balloon.png`。本日 Windows 受控 adapter 测试已经执行；Linux fixture 断言仅完成交叉编译、尚未运行。两者都不写成 Windows 或 Linux 系统打印成功。

## 源代码模块

支持 `cpp`、`go`、`python`、`java`；Chroma 提供语法高亮，模板保留缩进、显示行号，Chromedp 输出 A4 PDF，并使用页眉、页码和可换行长行。打印步骤与气球模块相同，平台 adapter 只接收已经生成的固定 `preview.pdf`。

Task 10 的 Chrome 验收已经证明 140 行 C++ 样例生成 6 页 A4 PDF，中文注释、行号、页眉和页码可见，截图证据为 `docs/reports/assets/day-05-source-page-1.png` 与 `docs/reports/assets/day-05-source-page-2.png`。本日未产生 Linux CUPS 任务号或 Windows 队列记录。

## 主程序的安全选择机制

- `LOCAL_PRINT_AGENT_PRINTER_MODE` 未设置或设置为 `demo`：使用名称为“Mock Printer（不执行实体打印）”的 Fake Adapter。这是默认值，保证缺少 SumatraPDF/CUPS 的环境仍可演示网页、队列和 PDF，同时页面明确显示不会实体打印。
- `LOCAL_PRINT_AGENT_PRINTER_MODE=platform`：显式构造当前操作系统的 `NewPlatformAdapter`，不自动降级为 Fake；Windows 还要求 `LOCAL_PRINT_AGENT_SUMATRA_PATH`。平台依赖缺失时启动失败并返回稳定错误，避免把 Fake 成功误认成系统打印成功。
- `/api/v1/printers`、Worker 和任务提交使用同一个选中 adapter。`TestExplicitPlatformModeExposesPlatformAdapterPrintersAPI` 证明 platform 模式下 API 返回 adapter 枚举队列而不是固定 Mock Printer。
- 其他 mode 值在启动时拒绝，不作含糊的自动探测或回退。

## Windows 适配器命令与安全校验

- 枚举：固定 PowerShell `Get-CimInstance Win32_Printer | Select-Object Name,Default | ConvertTo-Json`，兼容单对象与数组。
- 提交参数严格为 `-print-to <枚举精确名称> -silent <DataDir>/jobs/<jobID>/preview.pdf`，不经 shell 拼接。
- 每次命令有 30 秒子 context，stdout/stderr 分离捕获；公开错误只返回稳定 code/message，不泄露可执行文件路径、PDF 路径和命令诊断。
- 仅接受本程序 `jobs/<32位小写hex>/preview.pdf`，打印前和枚举后各校验一次；逐路径组件拒绝 symlink、junction 和 reparse point。
- 构造时固定 SumatraPDF 的完整绝对路径、逐组件拒绝 reparse，并记录文件身份；枚举后再次校验路径和身份，防止父目录重定向或可执行文件替换。
- 打印机名必须精确属于本次枚举 allowlist，否则在 SumatraPDF 前返回 `PRINTER_NOT_FOUND`。

本机非破坏性验证使用受控 runner，只记录 PowerShell/Sumatra 参数，不启动真实 Sumatra 或访问打印队列。当前未配置 `LOCAL_PRINT_AGENT_SUMATRA_PATH`，且受限账户读取 `Win32_Printer` 曾返回 Access denied，因此 Windows 系统队列接受结果仍为未验证。

## Linux 适配器命令与安全校验

- 构造时分别使用 `exec.LookPath("lp")`、`exec.LookPath("lpstat")`；任一缺失均返回 `PRINT_COMMAND_FAILED` 和安装 CUPS client 的稳定提示，底层路径不公开。
- 枚举先执行 `lpstat -p`，再执行 `lpstat -d` 查询默认项；这样“有队列但未设置默认项”不会误判为枚举失败。解析 `printer <name> ...` 和 `system default destination: <name>[/<instance>]`，默认 instance 映射到其基础队列；明确的 `No destinations added` 或空队列返回 `PRINTER_NOT_FOUND`。
- 提交参数严格为 `lp -d <本次枚举精确名称> <preview.pdf>`，不使用 shell；未知或注入式名称在执行 `lp` 前拒绝。
- 每次外部命令使用 30 秒子 context，stdout/stderr 分离捕获并脱敏；生产 runner 固定 `LC_ALL=C`，使解析格式稳定。
- PDF 路径只允许 `DataDir/jobs/<32位小写hex>/preview.pdf`；从文件系统根到 PDF 逐组件 `Lstat`，拒绝 symlink，并要求 data/jobs/job 为目录、PDF 为普通文件；枚举后再次校验，缩小校验后替换窗口。

Linux build tag 测试以固定 `lpstat` fixture 和受控 runner 覆盖解析、严格参数、deadline、allowlist、缺命令、路径越界、symlink、枚举期间删除和诊断脱敏。当前 Windows 主机的 WSL 没有可用发行版，Docker daemon 也未运行，因此本日证据仅为 Linux amd64 测试二进制交叉编译；测试二进制没有在 Linux 内核上运行，也没有 CUPS 任务号或队列截图。

## 两类任务状态时间线

| 模块 | 已观测时间线 | adapter 边界 | 结论 |
|---|---|---|---|
| 气球小票 | `queued → rendering → printing → succeeded` | Task 10 真实 Chrome PDF + Fake Printer；本日 Windows 受控命令边界 | PDF 与状态主路径已验证；OS 队列未验证 |
| 源代码 | `queued → rendering → printing → succeeded` | Task 10 真实 Chrome 6 页 PDF + Fake Printer；本日 Linux fixture/交叉编译边界 | PDF 与状态主路径已验证；CUPS 运行和队列未验证 |

如果平台 adapter 返回错误，Worker 会从 `printing → failed`，保留 `PRINTER_NOT_FOUND` 或 `PRINT_COMMAND_FAILED`；受控测试验证稳定错误会穿过 Worker，不被改写成含敏感诊断的文本。

## 自测表

| 功能 | 输入 | 期望 | 实际 | 结论 | 证据 |
|---|---|---|---|---|---|
| 默认安全模式 | mode 未设置 | 不构造平台 adapter | 返回显式 Mock Printer | 通过 | `TestConfiguredPrinterDefaultsToNonPrintingDemoAdapter` |
| 显式平台接线 | mode=`platform` + 受控 adapter | API/Worker 使用该 adapter | API 仅返回 `Platform Queue` | 通过 | `TestExplicitPlatformModeExposesPlatformAdapterPrintersAPI` |
| Windows 严格命令 | 枚举名称 + 合法 preview | 固定四参数 | 参数、顺序、30 秒 deadline 一致 | 通过 | `TestWindowsAdapterPrintUsesEnumeratedNameAndStrictSumatraArguments` |
| Windows allowlist | 注入式未知名称 | Sumatra 前拒绝 | 仅执行枚举 | 通过 | `TestWindowsAdapterRejectsUnknownPrinterBeforePrintCommand` |
| Windows PDF/可执行安全 | 越界、错误名、替换、reparse | 外部打印前拒绝且脱敏 | 稳定错误，无第二条命令 | 通过 | `windows_test.go` 路径与身份测试 |
| Linux `lpstat` 解析 | PDF + Office fixture，PDF 默认 | 两队列，PDF 默认 | Linux 测试二进制编译包含断言 | 未运行 | `TestParseLinuxPrintersMarksSystemDefault`；无 Linux runtime |
| Linux 严格 `lp` | `PDF` + 合法 preview | `-d PDF <preview>` | fixture runner 测试已编译 | 未运行 | `TestLinuxAdapterPrintUsesEnumeratedNameAndStrictLPArguments`；无 Linux runtime |
| Linux allowlist | `-o raw; ...` | `lp` 前拒绝 | fixture runner 测试已编译 | 未运行 | `TestLinuxAdapterRejectsUnknownPrinterBeforeLP`；无 Linux runtime |
| Linux PDF/symlink | 越界、错误名、叶 symlink、枚举中删除 | `lp` 前拒绝且脱敏 | Linux 测试二进制编译成功 | 未运行 | `linux_test.go` 路径测试；无 Linux runtime |
| Windows 实际队列 | Sumatra + 自动保存虚拟队列 | OS 接受并留存输出 | Sumatra/权限条件不满足，未提交 | 未验证 | Task 11/本报告环境记录 |
| Linux 实际队列 | CUPS 虚拟或实体队列 | 返回 request id 和队列记录 | 无 Linux/CUPS 环境，未提交 | 未验证 | WSL/Docker/CUPS 环境检查 |

## 第 7 天问题与明日计划

1. 在具有 SumatraPDF、打印机枚举权限且明确配置自动保存虚拟队列的 Windows 环境补做一次气球任务，保存队列与输出证据；未确认目标前不得向实体打印机提交。
2. 在真实 Linux 或 WSL2 + CUPS 环境运行 `go test ./... -count=1 -v`，再选择可控虚拟队列提交源码 PDF，记录 `lp` request id、`lpstat -o` 和输出；当前交叉编译不能替代此项。
3. 注入 Windows/Linux 枚举失败、命令失败、context 取消和重试，复核 `printing → failed → queued` 恢复路径与稳定错误。
4. 做双平台全量回归、竞态检查、端口冲突与服务重启测试，问题清零后再更新真实打印完成口径。
