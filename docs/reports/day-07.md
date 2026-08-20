# 第7天：回归表、问题处理与遗留项

## 回归表

| 编号 | 平台 | 场景 | 输入 | 期望 | 实际 | 结论 | 证据 |
|---:|---|---|---|---|---|---|---|
| 1 | Windows | 健康检查 | `GET /health` | service/api/status 固定 | HTTP 200，字段正确 | 通过 | `TestHealthReturnsServiceStatusJSON` |
| 2 | Windows | 气球 HTML | 中文元数据与 HTML 标签 | 窄纸、中文可见、转义 | 真实 template 输出符合 | 通过 | `TestRenderBalloonHTMLShowsRequiredTicketDetailsAndEscapesMetadata` |
| 3 | Windows | 源码 HTML | 中文注释、`<iostream>` | 行号、高亮、HTML 转义 | Chroma 结构与转义正确 | 通过 | `TestRenderSourceHTMLHighlightsSupportedLanguagesAndPreservesSourceAsText` |
| 4 | Windows | 任务列表/详情 | API GET | 统一 envelope | 列表和详情均 200 | 通过 | `TestAPIJobListAndDetail` |
| 5 | Windows | PDF 预览 | 安全 job PDF 与 Range | 200/206，不越界 | 完整与分段内容正确 | 通过 | `TestAPIPreviewServesOnlyStoredJobPDFWithSafeHeadersAndRange` |
| 6 | Windows | 失败重试 | `failed -> retry` | 清空错误/时间并重新排队 | 状态重置，attempts 保留 | 通过 | `TestServiceRetryOnlyFailedJobAndResetsLifecycle` |
| 7 | Windows | 端口回退 | 17653 被占用 | 监听 17654 | 返回第二候选端口 | 通过 | `TestListenFirstAvailableFallsBackWhenFirstPortIsOccupied` |
| 8 | Windows | 打印机不存在 | 枚举外的打印机名 | `PRINTER_NOT_FOUND`，不打印 | Worker 保留稳定错误，无成功命令 | 通过 | `TestWorkerPreservesStablePrinterAdapterError` |
| 9 | Windows | Chrome 不存在 | 不存在的 exe | `RENDERER_NOT_FOUND` | 错误稳定且不泄漏路径 | 通过 | `TestNewPDFRendererRejectsExplicitMissingBrowserWithoutLeakingPath` |
| 10 | Windows | SumatraPDF 不存在 | 不存在的 exe | `PRINT_COMMAND_FAILED` | 构造阶段拒绝，路径脱敏 | 通过 | `TestNewPlatformAdapterRejectsMissingSumatraWithoutLeakingPath` |
| 11 | Linux build-tag | CUPS 缺失 | 注入 `lp`/`lpstat` LookPath 失败 | 稳定安装提示 | linux/amd64 测试二进制交叉编译成功 | 仅编译 | `.tmp-linux-printer.test` 4,576,023 bytes；无 Linux runtime |
| 12 | Windows | 模板非法 | 未闭合 `{{if}}` | 真实 parser 失败 | 返回 parse error | 通过 | `TestExecuteTemplateContentsRejectsInvalidTemplate` |
| 13 | Windows | 请求超限 | 大于 1 MiB JSON | 413 | 413 / `REQUEST_BODY_TOO_LARGE` | 通过 | `TestAPICreateJobRejectsMalformedBodies/body_too_large` |
| 14 | Windows | 队列满 | 真实 Service 提交 101 个 | 第 101 个 503，不落库 | 持久化数仍为 100 | 通过 | `TestAPICreateReportsQueueFullWithoutPersistingOverflow` |
| 15 | Windows | 服务重启 | JSON 中 rendering/printing/queued | 中断任务失败，queued 保留 | 重新打开后仍持久 | 通过 | `TestJSONStoreRestartRecoveryIsDurableAndDoesNotRequeueInterruptedWork` |
| 16 | Windows/race | 20 并发创建 | 20 个 HTTP POST + 真实 JSON Store | ID 唯一、全部落库、无 race | 20 个 32 位 ID 唯一，Store 为 20 | 通过 | `TestAPIConcurrentCreatePersistsTwentyUniqueJobs`; `go test -race ./...` |
| 17 | Windows/race | FIFO 单线程 | 20 个顺序 job ID | 顺序不变，最大并行数 1 | 00–19 顺序，maximum=1 | 通过 | `TestWorkerProcessesTwentyJobsFIFOWithoutOverlap` |
| 18 | Windows | Tab/末尾换行 | 首行 Tab、内嵌 Tab、中文和末尾 `\n` | 规范化不修改源码 | 字节级一致 | 通过 | `TestNormalizeCreateRequestPreservesSourceWhitespaceAndTabs` |
| 19 | Windows | 65536 字节边界 | 65536/65537 个 ASCII 字节 | 前者接受，后者拒绝 | 符合边界 | 通过 | `TestValidateCreateRequest` 上下界子用例 |
| 20 | Windows | 长行/多页 | 长行 CSS 与 140 行中文 fixture | 换行、行号、多页 | HTML/fixture 契约通过 | 部分通过 | `TestRenderSourceHTMLUsesLineStructureForWrappedCode`; 本轮 Chrome 实跑被环境阻断 |
| 21 | Windows | 非法 RFC3339 | `19-08-2026 09:30` 和逗号小数 | INVALID_REQUEST | 校验拒绝 | 通过 | `TestValidateCreateRequest` 时间子用例 |
| 22 | Windows/Node 22 | Web 真实行为 | 健康、打印机、任务 fixture | 发现、倒序、安全文本、2s 定时器清理 | JavaScript 实际执行通过 | 通过 | `TestAppJavaScriptRunsDiscoveryRenderingAndTimerLifecycle` |
| 23 | Windows | 真实 Chrome PDF | Chrome 151 + `pdftoppm` | 气球 1 页、源码多页 | Chrome 启动回退返回 `context canceled` | 环境阻断 | 本轮 E2E 命令原始输出；无系统打印 |

## 问题处理记录

### 1. 源码缩进被规范化破坏

- 现象：以 Tab 开头或以换行结尾的 `source_code` 在 Service 持久化前被删除这些字符。
- 预期：元数据可去除首尾空白，源码必须字节级保留。
- 报错：新增回归得到无 Tab/无末尾换行的值。
- 定位：沿 API 输入→`NormalizeCreateRequest`→JSON Store 追踪，确认 `normalizeSourceCodePayload` 对整段源码调用 `strings.TrimSpace`。
- 根因：把源码误当作普通表单元数据处理。
- 修改：只用 TrimSpace 判断纯空白，长度与持久化均使用原始源码。
- 复测：Tab、中文、HTML 字符和末尾换行完整保留，65536/65537 边界仍正确。

### 2. Worker 暂态存储失败会高频重试

- 现象：Get/Update 持续失败时固定每 1ms 重试。
- 预期：保持 FIFO 与取消响应，同时避免磁盘故障时忙循环。
- 定位/根因：`retryWait` 只有一个固定延迟，没有 attempt 状态。
- 修改：5ms 起步指数退避，250ms 封顶；Get 和每个持久化阶段独立计数。
- 复测：真实重试循环观测到 `5,10,20,40,80,160,250,250ms`，取消仍立即退出。

### 3. Worker 观察通道和队列所有权没有结束语义

- 现象：`Run` 返回后 `Errors()` 仍永久打开；全局 `queueRegistry` 永久持有 low-level queue。
- 根因：只设计了启动与独占检查，没有把 Worker/Service 终止纳入生命周期。
- 修改：Worker 退出时关闭 Errors/Done；`Service.Close` 幂等释放当前 owner；`NewPipeline` 自动绑定释放。
- 复测：通道可被 range 正常结束，Close 后同一 low-level queue 可由新 Service 合法接管。

### 4. Web 契约测试只检索源码字符串

- 现象：JavaScript 即使存在语法错误或没有真正执行某分支，仍可能满足关键字断言。
- 根因：初期受 DevTools 策略限制，只增加了静态守护。
- 修改：在 Node.js VM 中运行嵌入的 `app.js`，提供最小 DOM/fetch 边界，断言发现顺序、打印机渲染、任务倒序、恶意错误文本、2s 轮询和 unload 清理。
- 复测：Windows Node 22 实际执行通过；无 Node 平台跳过该增强用例，仍运行原静态安全契约。

## Windows/Linux 差异

| 项目 | Windows | Linux |
|---|---|---|
| 打印机枚举 | PowerShell/CIM 结构化 JSON | `lpstat -p` + `lpstat -d` |
| PDF 提交 | SumatraPDF 参数数组 | `lp -d <queue> <pdf>` 参数数组 |
| 本轮运行 | Windows 单元、集成、race、vet 已运行 | linux/amd64 build-tag 测试仅交叉编译 |
| 真实队列 | 未配置 Sumatra/可控自动保存队列，未提交 | 无 Linux runtime/CUPS，未提交 |
| 路径安全 | 另外检查 reparse point 和 exe 文件身份 | 逐级 `Lstat` 拒绝 symlink |
| 错误语言 | PowerShell/Sumatra 诊断不对外公开 | runner 固定 `LC_ALL=C`，再转换稳定错误 |

## 测试统计与环境边界

- `go test ./... -count=1`：133 个顶层测试通过，0 失败，6 跳过；跳过项包含显式 opt-in 的真实 Chrome/pdfinfo 和 Windows 非特权用户无法创建的 symlink 场景。
- `go test -race ./... -count=1`：10 个含测试的包全部通过，0 race；`templates` 无测试文件。
- `go vet ./...`、`go mod verify`、`git diff --check`：通过；本机 Git 另行提示 LF 将按配置转为 CRLF，不是 diff whitespace error。
- 真实 Chrome E2E：本轮在受限桌面环境中失败，版本探测回退启动返回 `context canceled`；申请在沙箱外启动无头 Chrome 时工具审批额度耗尽而被拒绝。因此本报告不声称本轮 Chrome E2E 通过，也没有用单元测试替代。
- Linux/CUPS：没有 Linux runtime；只证明 build-tag 代码和测试可编译，不声称 CUPS 已运行或 OS 队列已接受。
- 全过程未向实体或系统打印队列发送任务。

## 遗留项

| 项目 | 影响 | 临时规避 | 是否进入最终报告 |
|---|---|---|---|
| 本轮受限环境不能启动无头 Chrome | 缺少当日真实 PDF 复验；不影响自动渲染边界与 Day 5 已保存证据 | 在本机普通终端或 CI 重跑两条 opt-in E2E | 是，必须说明本轮失败 |
| 无 Linux runtime/CUPS 队列 | 不影响 Windows 代码和 Linux 交叉编译；Linux 实跑验收仍未完成 | 在 Linux/WSL2 安装 CUPS client，只用可控虚拟队列 | 是 |
| 无 SumatraPDF 与安全自动保存目标 | 不影响参数/安全/错误映射测试；Windows OS 接受仍未验证 | 配置隔离的虚拟 PDF 队列后人工确认目标再提交 | 是 |

以上都是环境/现场证据遗留，本日发现的源码损坏、Worker 高频重试、Errors 生命周期和 queue registry 泄漏均已修复，没有将已知代码阻断项推迟到第 8 天。

## 明日计划

1. 完成 README，把 Chrome、SumatraPDF、CUPS、环境变量和常见错误写成他人可复现步骤。
2. 在具备权限的终端补跑真实 Chrome E2E，并在 Linux/CUPS 环境保存原始命令与队列证据。
3. 录制不向实体打印的安全演示，再由他人按 README 启动。
4. 完成自测对照表、证据索引和交付归档。
