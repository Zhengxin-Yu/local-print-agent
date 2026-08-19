# 第3天：主路径对象、成功失败约定与假实现

## Job 字段

| 字段 | 来源 | 用途 | 示例 |
|---|---|---|---|
| `id` | Service 的 crypto/rand 128-bit 生成器 | 稳定任务标识及队列消息 | 32 位十六进制字符串 |
| `type` | 已规范化的创建请求 | 选择渲染模板 | `source_code` |
| `printer_name` | 已规范化的创建请求 | 操作系统打印目标 | `front-desk` |
| `payload` | 已规范化的 JSON 请求体 | 渲染内容 | `{"language":"go",...}` |
| `status` | Service/Worker 状态机 | 生命周期展示与重试判断 | `queued` |
| `error` | Worker 或 Service | 稳定错误码和可读原因 | `PRINT_COMMAND_FAILED` |
| `created_at` / `updated_at` | Service/状态迁移 | 排序与审计 | RFC3339 时间 |
| `started_at` / `finished_at` | Worker 状态迁移 | 本次尝试的运行区间 | RFC3339 时间 |
| `attempts` | 进入 `rendering` 时递增 | 重试计数 | `1` |
| `pdf_path` | Renderer 返回 | 传给打印适配器的暂存 PDF | `C:\\Temp\\job.pdf` |

## 接口

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

## 成功与失败约定

成功只表示操作系统适配器已接受 PDF 打印命令，状态为 `succeeded`；不等同于纸张已经实际输出。物理出纸须由后续硬件/人工记录处理。

| 场景 | 错误码 | 用户可读原因 | 任务保留 | 可重试 |
|---|---|---|---|---|
| 请求不合法 | `INVALID_REQUEST` | 校验失败详情 | 否 | 修正请求后新建 |
| 等待队列满 | `QUEUE_FULL` | `print queue is full` | Create 否；Retry 保持 failed | 是 |
| 队列交付契约被破坏 | `QUEUE_DELIVERY_FAILED` | 已持久化但无法安全投递 ID | Create/Retry 已是 queued | 需人工恢复队列后处理 |
| 渲染失败 | `RENDER_FAILED` | Renderer 原因 | 是 | 是 |
| 打印命令失败 | `PRINT_COMMAND_FAILED` | Adapter 原因 | 是 | 是 |
| 非失败状态重试 | `RETRY_NOT_ALLOWED` | `only failed jobs can be retried` | 是 | 否 |
| 存储/取消 | `STORE_ERROR` / `CONTEXT_CANCELED` | 保留底层可诊断原因 | 视持久化结果 | 视状态 |

`Fake Renderer` 只在测试中生成临时 PDF 文件；`Fake Printer` 只线程安全地记录 `Print` 调用并可注入错误。二者绝不声称已真实打印。

## 状态时序（测试内存 history 断言摘要）

```text
TestWorkerProcessesQueuedJobThroughSuccessfulFIFOStates
first:  queued -> rendering -> printing -> succeeded
second: queued -> rendering -> printing -> succeeded
PASS
```

Worker 单 goroutine 逐个取 FIFO ID，先持久化 `rendering`（并增加 attempts），再渲染；随后持久化 PDF 路径与 `printing`，最后提交打印命令并持久化 `succeeded`。

## JSON 恢复与服务重启

`JSONStore` 的恢复测试覆盖持久化后重新打开，以及 `RecoverInterrupted`：重启时仍处于 `rendering` 或 `printing` 的任务改为 `failed`，使用 `SERVICE_RESTARTED` 说明原因，之后可通过 Service 的 Retry 再入队。将尚未开始的 `queued` 任务重新入队属于后续启动接线，本日尚未实现；Worker 本身不关闭队列，也不会将“操作系统已接受”误报为物理出纸完成。

## 命令提交的 at-least-once 边界

运行期中，`succeeded` 持久化失败会重试同一快照，绝不会再次调用 `Print`。但进程可能在操作系统已经接受 `Print` 命令、而 `succeeded` 尚未持久化的窗口崩溃；重启恢复后 Retry 可能再次提交同一 PDF。因此 OS 命令提交是 **at-least-once**，而非 exactly-once，不能由当前进程内逻辑消除该崩溃窗口。

## 今日问题与处理

| 问题 | 处理 |
|---|---|
| 并发访问 | Service 互斥串行化 Create/Retry；Store/Fake Printer 各自加锁。 |
| 重复打印 | 运行期的最终持久化重试不重复 Print；崩溃窗口仍是 at-least-once，Retry 可能重复提交。 |
| 队列满 | 固定容量 100，Service 发送前检查容量；Create 不落库，Retry 不改变 failed 状态。 |

## 明日计划

实现 HTTP API、Mock Web，以及创建、列表、详情、重试和任务状态页面。
