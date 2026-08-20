# 课题三日常报告（第 3 天）：任务主路径与失败契约

## 基本信息与今日目标

- 课题：浏览器调用本地打印机。
- 完成方式：独立完成。
- 今日范围：完成任务存储、队列、Worker 和可注入假实现；不接 HTTP 页面和真实平台命令。
- 今日目标：打通“创建任务 -> FIFO 处理 -> 成功/失败持久化”的确定性主路径。

## 核心对象与接口

| Job 字段 | 来源 | 用途 |
| --- | --- | --- |
| `id` | Service 使用 `crypto/rand` 生成 | 32 位小写十六进制稳定任务标识 |
| `type`、`printer_name`、`payload` | 规范化后的创建请求 | 选择模板、打印目标和业务内容 |
| `status`、`error` | Service/Worker | 状态展示、失败诊断与重试判断 |
| `created_at`、`updated_at` | Service/状态迁移 | 排序与审计 |
| `started_at`、`finished_at`、`attempts` | Worker 状态迁移 | 记录每次尝试的生命周期 |
| `pdf_path` | Renderer | 将固定预览文件交给 Printer Adapter |

```go
type JobStore interface {
    Create(context.Context, *Job) error
    Update(context.Context, *Job) error
    Get(context.Context, string) (*Job, error)
    List(context.Context) ([]*Job, error)
}

type Renderer interface {
    Render(context.Context, *jobs.Job) (string, error)
}

type Adapter interface {
    List(context.Context) ([]Info, error)
    Print(context.Context, printerName, pdfPath string) error
}
```

Store、Renderer 和 Adapter 均以接口隔离，Worker 不需要知道 JSON 文件、Chrome、SumatraPDF 或 CUPS 的内部实现。Fake Renderer 只生成可识别的临时 PDF；Fake Printer 只线程安全记录调用并允许注入错误，两者都不声称已经进入系统队列。

## 主路径与失败约定

单 Worker 从容量 100 的 FIFO 中逐个取任务 ID：

```text
queued
  -> 持久化 rendering，attempts + 1
  -> Renderer 生成 PDF
  -> 持久化 pdf_path 和 printing
  -> Adapter 接受调用
  -> 持久化 succeeded
```

| 场景 | 稳定错误码 | 任务是否保留 | 后续动作 |
| --- | --- | --- | --- |
| 请求不合法 | `INVALID_REQUEST` | 否 | 修正输入后新建 |
| 新任务遇到队列满 | `QUEUE_FULL` | 否 | 队列释放后重新提交 |
| 已持久化任务无法安全投递 ID | `QUEUE_DELIVERY_FAILED` | 是，保持 `queued` | 修复队列后恢复处理 |
| 渲染失败 | `RENDER_FAILED` | 是，进入 `failed` | 可重试 |
| 打印命令失败 | `PRINT_COMMAND_FAILED` | 是，进入 `failed` | 可重试 |
| 非失败任务请求重试 | `RETRY_NOT_ALLOWED` | 是，状态不变 | 不允许 |
| 存储或取消 | `STORE_ERROR` / `CONTEXT_CANCELED` | 取决于持久化阶段 | 保留可诊断边界 |

成功只表示当前 Adapter 接受 PDF 调用，不等同于操作系统队列一定完成，更不等同于纸张已经输出。

## 重启恢复与重复提交边界

JSON Store 能在重新打开后恢复任务。服务重启时，仍处于 `rendering` 或 `printing` 的任务被标记为 `failed`，错误为 `SERVICE_RESTARTED`，之后可以人工重试；尚未开始的 `queued` 任务保持排队状态。

运行期间，如果 Adapter 已返回成功而最终 `succeeded` 持久化暂时失败，Worker 只重试写入状态，不再次调用 `Print`。但进程可能恰好在操作系统接受命令之后、状态持久化之前崩溃；重启后人工重试可能再次提交同一 PDF。因此平台提交是 **at-least-once**，当前范围不承诺 exactly-once。

## 今日验收

| 前提 | 操作 | 预期结果 | 实际结果与结论 |
| --- | --- | --- | --- |
| 顺序创建两个合法任务 | 启动单 Worker 消费队列 | 两任务按 FIFO 依次走完四个运行状态，无并发重叠 | 通过 history 断言 |
| Renderer 注入错误 | 消费 `queued` 任务 | `queued -> rendering -> failed`，保留 `RENDER_FAILED` | 通过 |
| Printer 注入错误 | 让渲染成功、打印失败 | `queued -> rendering -> printing -> failed`，保留 `PRINT_COMMAND_FAILED` | 通过 |
| 队列已满 | 创建第 101 个任务 | 返回 `QUEUE_FULL`，新任务不落库 | 通过 Service 测试 |
| Store 中存在中断状态 | 重新打开并执行恢复 | rendering/printing 改为 failed，可再次重试 | 通过持久化恢复测试 |
| Adapter 已调用、最终状态写入暂时失败 | 重试最终持久化 | 不重复调用 Adapter | 通过 Worker 重试契约 |

成功序列的测试摘要：

```text
first:  queued -> rendering -> printing -> succeeded
second: queued -> rendering -> printing -> succeeded
PASS
```

## 问题处理与 AI 沟通

| 问题 | 处理结果 |
| --- | --- |
| 并发创建可能竞争 | Service 串行化 Create/Retry；Store 和 Fake Printer 各自加锁。 |
| 队列满时可能出现“已落库但未排队” | Create 在持久化前检查容量；Retry 队列满时保持 `failed`。 |
| 最终状态写入失败可能导致重复打印 | 进程内只重试状态写入；崩溃窗口单独记录为 at-least-once 风险。 |
| 重启时任务状态含糊 | 中断运行态统一转为 `failed/SERVICE_RESTARTED`，允许显式重试。 |

本日与 AI 沟通中最重要的修正，是拒绝“加一个幂等判断即可保证绝不重复打印”的宽泛建议。沿着 Adapter 调用与状态落盘的时间顺序检查后，确认进程崩溃窗口无法仅靠内存状态消除，因此报告改为准确的 at-least-once 说明，并把是否重试交给可观察状态和人工判断。

## 自检与明日计划

今日已完成 Store、FIFO、Service、Worker 和两类 Fake 边界；成功、渲染失败、打印失败、队列满和重启恢复均有确定性测试。HTTP API、浏览器页面、真实 HTML/PDF 和平台命令仍未接入。

明日交付 7 个 HTTP 路由的初版、统一响应 envelope、嵌入式 Mock Web，以及创建、列表、详情、重试和任务状态展示；同时提交一张真实运行页面截图，并明确 preview 与 Fake Printer 的未实现边界。
