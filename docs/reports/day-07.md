# 课题三日常报告（第 7 天）：全链路回归与问题闭环

## 基本信息与今日目标

- 完成方式：独立完成。
- 今日范围：不增加业务功能，集中验证主路径、失败路径、并发、恢复和跨平台边界。
- 今日目标：把已发现的代码问题当天闭环，并将环境阻断与代码失败分开记录。

## 代表性回归结果

| 类别 | 前提与操作 | 预期结果 | 实际结论 |
| --- | --- | --- | --- |
| 探活与端口 | 占用 `17653` 后启动并请求 health | 回退 `17654`，服务字段固定 | 通过 |
| 两类 HTML | 输入中文、HTML 标签、长行和多页源码 | 转义安全；中文、高亮、行号与换行存在 | 通过 |
| API 与 preview | 创建、列表、详情、完整/Range 预览 | 统一 envelope；200/206；不越界 | 通过 |
| 失败重试 | 构造 `failed` 后调用 retry | 清理错误和运行时间并回到 `queued` | 通过 |
| 队列上限 | 向容量 100 的真实 Service 提交 101 个任务 | 第 101 个返回 503 且不落库 | 通过 |
| 并发创建 | 20 个 HTTP POST 写入 JSON Store | ID 唯一、全部落库、无 race | 通过 |
| FIFO | 顺序提交 20 个任务 | 00-19 顺序不变，最大并行数为 1 | 通过 |
| 服务重启 | Store 中存在 queued/rendering/printing | queued 保留；中断运行态转 failed | 通过 |
| 依赖缺失 | Chrome 或 Sumatra 路径不存在 | 返回稳定错误且不泄露绝对路径 | 通过 |
| 源码字节 | 输入 Tab、中文、HTML 字符和末尾换行 | 原始源码字节保持；65536 接受、65537 拒绝 | 通过 |
| Web 行为 | Node 22 执行真实 `app.js` | 探测、倒序、安全文本、2 秒刷新与清理正确 | 通过 |
| Linux/CUPS | 交叉编译 Linux build-tag 测试 | 代码和测试可编译 | 仅编译，无 Linux runtime |
| 真实 Chrome 当日复跑 | 在受限桌面环境启动 Chrome E2E | 生成两类真实 PDF | 环境阻断；返回 `context canceled` |

Day 5 已保存的真实 Chrome 截图仍是有效历史证据，但 Day 7 的重新执行失败必须单独记录，不能用历史成功覆盖当日环境结果。全程没有向实体或系统打印队列发送任务。

## 问题一：源码缩进和末尾换行丢失

- 现象：以 Tab 开头或以换行结尾的 `source_code` 在持久化前被删除字符。
- 根因：规范化函数把源码误当普通表单元数据，对整段内容调用 `strings.TrimSpace`。
- 处理：TrimSpace 只用于判断是否全为空白；长度检查和持久化继续使用原始字符串。
- 复测：Tab、中文、HTML 字符和末尾换行字节级一致，65536/65537 byte 边界仍正确。

## 问题二：Store 故障时 Worker 高频重试

- 现象：Get/Update 持续失败时每 1 ms 固定重试，可能形成忙循环。
- 根因：重试逻辑没有 attempt 状态和上限。
- 处理：改为 5 ms 起步的指数退避，250 ms 封顶，每个持久化阶段独立计数并响应取消。
- 复测：实际观察到 `5,10,20,40,80,160,250,250 ms`，取消后立即退出。

## 问题三：Worker 缺少完整结束语义

- 现象：`Run` 返回后 `Errors()` 不关闭，低层队列所有权未释放。
- 根因：初始设计只覆盖创建和运行，没有把观察通道、Service 所有权与 Worker 结束视为同一生命周期。
- 处理：Worker 退出时调用幂等 `Service.Close`，随后关闭 Errors 和 Done。
- 复测：通道可正常 range 结束；Close 后同一低层队列能由新 Service 接管。

## 问题四：Web 测试只扫描源码字符串

- 现象：即使 JavaScript 有语法错误或关键分支没有执行，关键词断言仍可能通过。
- 根因：初期浏览器自动化受策略限制，测试退化为静态源码检查。
- 处理：在 Node VM 中执行真实 `app.js`，提供最小 DOM/fetch 边界。
- 复测：验证 health 优先探测、打印机和任务渲染、恶意文本安全、两秒轮询和 unload 清理；Node 不可用时明确 skip，不伪装通过。

## 测试统计与证据边界

- `go test ./... -count=1`：133 个顶层测试通过、0 失败、6 跳过。
- `go test -race ./... -count=1`：10 个含测试包通过，0 race。
- `go vet ./...`、`go mod verify`、`git diff --check`：通过。
- 6 个 skip 包含显式 opt-in 的真实 Chrome/pdfinfo 和普通账户无权创建 symlink 的场景，skip 不计为 pass。
- Linux 只证明 build-tag 代码和测试可交叉编译，不证明 CUPS 运行。（该缺口已于 2026-08-21 补验，见文末「补验记录」。）
- Windows 当日未配置 SumatraPDF 和安全自动保存队列，不证明系统接受。（该缺口已于 2026-08-21 补验，见文末「补验记录」。）

## AI 沟通与自检

本日最有效的 AI 沟通不是让助手“再多列测试”，而是要求指出现有验证能否真正执行行为。这个问题暴露出 Web 测试只检查字符串的薄弱点，随后改用 Node VM 执行真实脚本。相反，把静态关键词命中写成“页面功能通过”是不成功的证据表达，已从报告中删除。

自检结果：本日发现的源码损坏、忙循环、生命周期缺口和 Web 弱验证均已修复并复测；真实 Chrome、Windows 系统队列和 Linux/CUPS 属于环境/人工证据缺口，不是被隐藏的代码通过项。

## 明日计划

交付固定章节 README、7 接口 API 文档、8 分钟演示脚本、Windows/Linux 显式 demo/platform 启动脚本和干净副本启动记录；运行全量 test/race/vet/module/diff 检查，并列出真人启动、双平台录屏和 Linux runtime 的待补材料。

## 补验记录（2026-08-21）：Windows 平台 runtime 与系统队列证据

Day 7 记录的「真实 Chrome 当日复跑环境阻断」与「Windows 未配置 SumatraPDF 和安全队列」两项缺口中的后者已按 PROJECT_HANDOFF.md P0-A 补验完成：

- 环境：Windows 11 Pro build 22621；SumatraPDF 3.6.1（`tools/sumatra/`，不入库）；安全队列 `ISO-PDF-Queue`（Microsoft Print To PDF 驱动 + 本地文件端口，固定输出到本机 print-iso 隔离目录下的 `iso-output.pdf`（仓库外），不出纸、不弹保存对话框）。
- 提交前先用 SumatraPDF 向该队列冒烟试投一份已有文件，确认静默产出 PDF 后才提交气球任务。
- platform 模式启动后实际枚举到系统队列；气球任务 `queued -> succeeded`，attempts=1，系统队列实际落盘 289,600 字节 PDF，`%PDF-` 文件头。
- 连续录屏约 4 分 13 秒，覆盖环境信息、启动、health、枚举、提交、状态、预览、隔离输出与优雅停止。
- 证据文件：`docs/reports/assets/windows-platform-queue-2026-08-21.mp4`（录屏）、`docs/reports/assets/windows-iso-output-2026-08-21.pdf`（队列产出副本）、`docs/reports/assets/windows-platform-evidence-2026-08-21.md`（结构化记录，含任务 ID `97654d58a864b473735d097571ba6b15`）。
- 措辞边界：以上证明 Windows 系统队列接受与隔离文件输出，不等于物理出纸；Linux/CUPS runtime 缺口保持不变。

## 补验记录（2026-08-21）：Linux/CUPS runtime 与 request id

Day 7 记录的「Linux/CUPS 仅交叉编译、无 Linux runtime」缺口已按 PROJECT_HANDOFF.md P0-B 补验完成：

- 环境：WSL2 Ubuntu 24.04.4 LTS（内核 6.18.33.2-microsoft-standard-WSL2，真实 Linux 内核）；Go 1.25.4 linux/amd64；CUPS 2.4.7（真实 `lp`/`lpstat` 调用）；Chrome 151.0.7922.173；安全队列 `iso-queue`（cups-pdf:/ 后端，输出固定到 WSL 文件系统内 var 下的 print-iso 隔离目录（仓库外），不出纸、无弹窗，提交前已冒烟验证）；服务以非特权用户 `printuser` 运行。
- 自动回归在真实 Linux 内核执行：`go test ./...`、`go test -race ./...`（cgo+gcc）、`go vet`、`go mod verify` 全部通过；`internal/printer` Linux build-tag 测试不再是交叉编译验证。
- platform 模式端到端：枚举到 `iso-queue` 与 `PDF` 两个 CUPS 队列；中文气球任务 `07381f3ef5c6b9e2a73b86b34ad402d3` 经 `queued -> succeeded`（attempts=1）；CUPS request id 证据 `iso-queue-8`（`lpstat -W all`）；隔离输出 print-iso 目录下的 `preview.pdf` 32362 字节（`%PDF-`）；preview HTTP 200；SIGINT 优雅退出后端口关闭。
- 证据文件：`docs/reports/assets/linux-platform-evidence-2026-08-21.md`（结构化记录）、`docs/reports/assets/linux-iso-output-2026-08-21.pdf`（队列产出副本）。
- 连续录屏约 3 分 30 秒（`linux-platform-queue-2026-08-21.mp4`，因仓库体积限制不入版本控制，本机 assets 目录播放），录屏任务 `01d82152c37ba29a278d65c0c6544886`，CUPS request id `iso-queue-9`，隔离输出 32108 字节。
- 措辞边界：证明 Linux 平台 runtime 与 CUPS 系统队列接受（含 request id）及隔离输出，不等于物理出纸；运行环境为 WSL2，非物理机，如实标注。
