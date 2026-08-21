# 课题三日常报告（第 7 天）：全链路贯通与快速通检

## 基本信息

- 课题：浏览器调用本地打印机（课题三）。
- 成员：独立完成。
- 日期：2026 年 8 月 18 日。

## 今日目标

把第 4-6 天的部件贯通运行：HTTP -> Service -> FIFO -> Worker -> 真实 Chrome 渲染 -> Adapter 边界 -> 状态回写，对主路径和异常路径做不少于 5 条的快速通检，并把当天发现的代码问题闭环。

## 贯通说明

今日串联全部模块：嵌入式 Web 与 7 路由（Day 4）、任务流水线（Day 3）、真实 PDF 渲染与 preview（Day 5）、双平台 Adapter 与模式装配（Day 6）。替换的临时假实现：Fake Renderer 已在第 5 天换为 Chrome/Chromedp，本日贯通验证其与上下游协同；Fake Printer 保留为 demo 模式默认（安全边界），platform 模式装配真实 Adapter 但仅在受控 runner 下测试。当日新增 Node VM 执行真实 `app.js` 的行为测试，替换了此前“只扫描源码字符串”的弱验证。

## 快速通检

| 编号 | 步骤 | 期望 | 实际 | 是否通过 |
| --- | --- | --- | --- | --- |
| 1 | 占用 17653 后启动并请求 health | 回退 17654，服务字段固定 | 与期望一致 | 通过 |
| 2 | 提交中文、HTML 标签、长行与多页源码 | 转义安全，中文/高亮/行号/换行存在 | 与期望一致 | 通过 |
| 3 | 创建、列表、详情、完整/Range preview | 统一 envelope；200/206；不越界 | 与期望一致 | 通过 |
| 4 | 向容量 100 的队列提交 101 个任务 | 第 101 个 503/`QUEUE_FULL`，不落库 | 与期望一致 | 通过 |
| 5 | Store 存在 queued/rendering/printing 后重启 | queued 保留；运行态转 `failed/SERVICE_RESTARTED` | 与期望一致 | 通过 |
| 6 | 注入 Chrome 路径不存在 | `RENDERER_NOT_FOUND`，不泄露路径 | 与期望一致 | 通过 |
| 7 | 打印机名不在枚举 allowlist | `PRINTER_NOT_FOUND`，不执行打印命令 | 与期望一致 | 通过 |
| 8 | 构造 failed 后调用 retry | 清错误回 queued，attempts 保留 | 与期望一致 | 通过 |
| 9 | 20 个并发 POST 写 JSON Store | ID 唯一、全部落库、无 race | 与期望一致，race 检查 0 报告 | 通过 |

异常路径为第 4-8 条；全程未向实体或系统打印队列发送任务。

## 问题与处理

- 源码缩进和末尾换行丢失：现象是 Tab 开头或换行结尾的 `source_code` 在持久化前被删字符；期望是字节保持。已尝试确认根因为规范化函数对整段内容误用 `TrimSpace`；处理为仅用 TrimSpace 判空白、持久化原文，复测字节级一致，65536/65537 边界仍正确。
- Store 故障时 Worker 高频重试：现象是 Get/Update 持续失败时每 1 ms 重试形成忙循环；期望是有上限的退避。处理为 5 ms 起步指数退避、250 ms 封顶并响应取消，实测序列 `5,10,20,40,80,160,250,250 ms`。
- Worker 结束语义缺失：`Run` 返回后 Errors 不关闭、队列所有权未释放。处理为退出时幂等 Close Service 再关闭 Errors/Done，复测通道可正常 range 结束、锁可重新获取。
- Web 测试只扫描关键词：静态字符串命中不能证明行为执行。处理为在 Node VM 中运行真实 `app.js` 并提供最小 DOM/fetch 边界，验证探测、渲染、轮询与清理；Node 不可用时明确 skip。
- 真实 Chrome 当日复跑环境阻断：在受限桌面环境启动 Chrome E2E 返回 `context canceled`。已尝试隔离 profile 与超时参数；结论是环境阻断而非代码缺陷，该失败按日期如实保留，不用 Day 5 的历史成功覆盖。

## 遗留问题

留到第 8 天：固定章节 README/API 文档/演示脚本/干净副本启动记录；全量 test/race/vet/module/diff 检查。不影响主路径。人工证据缺口（Windows/Linux 系统队列、双平台录屏、真人 README 冷启动）不属于代码问题，已列入补验清单。

## 今日完成与自检

通检 9 条全部通过（含 5 条异常路径）；五个代码问题当天闭环并各有复测；`go test ./...` 133 个顶层测试通过、0 失败、6 跳过，race 0 报告，vet/module verify/diff 检查通过。skip 为显式 opt-in 的真实 Chrome/pdfinfo 与无权创建 symlink 场景，不计为通过。遗留事项已写明且不影响主路径。

## 问题、计划与 AI 沟通

- 不成功的沟通一例：排错初期只对助手说“Worker 有问题”，得到的是泛泛的重试建议。补充现象（每 1 ms 固定重试）、期望（有上限退避）与已尝试（调整间隔无效）后，才定位到重试逻辑缺少 attempt 状态。
- 比较有效的沟通一例：按“现象 / 期望 / 已尝试”三段提问 Web 弱验证问题，助手给出可验证的改造路径（Node VM 执行真实脚本），并促成“静态关键词命中不算页面功能通过”的结论。
- 明日安排：先做交付文档（README 与启动说明），再跑干净副本验证和提交材料整理。

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
