# 课题三日常报告（第 3 天）：任务主路径与失败契约

## 基本信息

- 课题：浏览器调用本地打印机。
- 完成方式：独立完成。
- 日期：2026 年 8 月 14 日。
- 今日范围：完成任务存储、队列、Worker 和可注入假实现；不接 HTTP 页面和真实平台命令。

## 今日目标

确定主路径上的三类约定：一是任务这一核心数据对象长什么样、由谁写哪些字段；二是主路径每一步成功与失败时各自的可观察表现（状态、错误码、任务去留）；三是哪些外部依赖今天先用假实现顶住，边界在哪、哪一天换成真实实现。

## 主路径上的数据或对象

主路径只有一个核心实体 Job，持久化在 `data/jobs.json`：

| Job 字段 | 来源 | 用途 |
| --- | --- | --- |
| `id` | Service 用 `crypto/rand` 生成 | 32 位小写十六进制稳定任务标识 |
| `type`、`printer_name`、`payload` | 规范化后的创建请求 | 选择模板、打印目标和业务内容 |
| `status`、`error` | Service/Worker | 状态展示、失败诊断与重试判断 |
| `started_at`、`finished_at`、`attempts` | Worker 状态迁移 | 记录每次尝试的生命周期 |
| `pdf_path` | Renderer | 把固定预览文件交给 Printer Adapter |

支撑主路径的三条边界接口（均为进程内函数调用，不涉及网络）：

```go
type JobStore interface { Create/Update/Get/List(context.Context, ...) }
type Renderer interface { Render(context.Context, *jobs.Job) (string, error) }
type Adapter  interface { List(context.Context) ([]Info, error)
                          Print(context.Context, printerName, pdfPath string) error }
```

Worker 只依赖这三个接口，不知道 JSON 文件、Chrome、SumatraPDF 或 CUPS 的存在。

## 调用约定

主路径最关键的一步是 Worker 对 `Renderer.Render` 的调用（HTTP 层明天才接，今天的入口是 Service/Worker 的函数调用）。

**成功一例**——创建气球任务：

- 输入要点：`type=balloon_ticket`、`printer_name` 非空、`payload` 含 `team_name`、`problem_id` 和 RFC3339 的 `solved_at`。
- 输出要点：`Service.Create` 返回落库的 Job（`status=queued`、`attempts=0`、32 位 hex `id`）；随后单 Worker 按 `queued -> rendering -> printing -> succeeded` 推进，`pdf_path` 写入预览路径，`finished_at` 非空。

**失败一例**——渲染失败：

- 输入要点：Renderer 被注入错误（模拟模板损坏或浏览器缺失）。
- 输出要点：任务进入 `failed`，保留 `RENDER_FAILED` 与稳定错误消息；任务仍在库中，可调用 retry 从 `failed` 回到 `queued` 再次尝试。

其余失败约定一句话索引：请求不合法 `INVALID_REQUEST`（不落库）；队列满 `QUEUE_FULL`（第 101 个不落库）；投递失败 `QUEUE_DELIVERY_FAILED`（保持 `queued`）；打印失败 `PRINT_COMMAND_FAILED`（进 `failed` 可重试）；非失败任务重试 `RETRY_NOT_ALLOWED`（状态不变）。

另有两条跨边界的硬约定：重启时 `rendering/printing` 一律转 `failed/SERVICE_RESTARTED`；平台命令提交是 at-least-once（进程在系统接受命令后、成功状态落盘前崩溃时，人工重试可能重复提交），不承诺 exactly-once。

## 临时假实现

| 假实现 | 当前行为 | 边界声明 | 换真计划 |
| --- | --- | --- | --- |
| Fake Renderer | 返回固定可识别的临时 PDF 路径 | 不渲染 HTML、不启动浏览器，`pdf_path` 仅用于链路演示 | 第 5 天换 Chrome/Chromedp 真实渲染 |
| Fake Printer | 线程安全记录调用、可注入错误 | 不执行任何系统打印命令，`succeeded` 不代表系统队列接受 | 第 6 天换 Windows SumatraPDF / Linux CUPS Adapter（代码与受控测试先行，真实队列证据后续单独补验） |
| Fake 枚举（Adapter.List） | 返回固定的 Mock 打印机列表 | 不查询 Win32_Printer 或 `lpstat` | 与 Fake Printer 同日替换 |

必须强调：今天测试里的 `succeeded` 只表示 Fake Adapter 接受了 PDF 调用，不能当成任何一层打印已完成。

## 项目备忘要点

- 技术栈：Go（module 要求 1.23+），标准库为主；JSON 单文件持久化，无数据库、无外部服务。
- 约定索引：错误码与状态迁移表见本报告；接口定义在 `internal/jobs`、`internal/worker`、`internal/store`、`internal/render`、`internal/printer`。
- 待决问题：preview 的读取路由尚未定义（明日先以 501 占位）；at-least-once 崩溃窗口保持记录，不做进程内“修复”。

## 今日完成与自检

| 前提 | 操作 | 预期结果 | 实际结果与结论 |
| --- | --- | --- | --- |
| 顺序创建两个合法任务 | 启动单 Worker 消费 | FIFO 依次走完四个运行状态，无并发重叠 | 通过 history 断言 |
| Renderer 注入错误 | 消费 `queued` 任务 | `failed`，保留 `RENDER_FAILED` | 通过 |
| Printer 注入错误 | 渲染成功、打印失败 | `failed`，保留 `PRINT_COMMAND_FAILED` | 通过 |
| 队列已满 | 创建第 101 个任务 | `QUEUE_FULL`，不落库 | 通过 |
| Store 存在中断状态 | 重新打开执行恢复 | 运行态转 `failed/SERVICE_RESTARTED` | 通过 |
| Adapter 已调用、最终写入失败 | 重试最终持久化 | 不重复调用 Adapter | 通过 |

自检：成功、渲染失败、打印失败、队列满和重启恢复均有确定性测试，约定齐备；Fake Renderer 与 Fake Printer 的边界和换真日期均已在上表标明，未被当作已完成。

## 问题、计划与 AI 沟通

- 不成功的沟通一例：起初让助手"列出任务系统需要的全部接口和字段"，得到一张面面俱到的大表（含审计、分页、多租户等），与当前主路径无关。调整方式是限定提示为"只保留创建到持久化主路径必需的实体和字段"，其余自行删减。
- 比较有效的沟通一例：要求只写"主路径成功一例与失败一例"并给出错误码与任务去留，一次就得到可用的调用约定；以及拒绝助手"加一个幂等判断即可保证绝不重复打印"的宽泛建议——沿 Adapter 调用与状态落盘的时间顺序复核后，确认崩溃窗口无法靠内存状态消除，改为如实声明 at-least-once。
- 其余问题处理：并发创建竞争由 Service 串行化 Create/Retry 解决；"已落库但未排队"由持久化前容量检查解决。

明日安排：先做统一响应 envelope 与 `/health` 探活，再交付 7 个 HTTP 路由初版和嵌入式 Mock Web；preview 路由先返回 501 并注明未实现边界。
