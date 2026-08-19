# HTTP API v1

服务只监听回环地址。以下示例假设终端输出的实际地址为 `http://127.0.0.1:17653`；若端口回退，请替换 `$base`/`$BASE`。除 `/health` 外，JSON 响应统一为 `{"data":...,"error":null}` 或 `{"data":null,"error":{"code":"...","message":"..."}}`。所有 JSON 都使用 UTF-8。

```powershell
$base = 'http://127.0.0.1:17653'
```

```bash
BASE=http://127.0.0.1:17653
```

## 1. 服务探活

**请求**：`GET /health`，无请求体。

**成功响应（200）**：此接口是发现兼容响应，不使用 API envelope。

```json
{"api_version":"v1","service":"local-print-agent","status":"ok"}
```

**状态码**：`200` 成功；`405 METHOD_NOT_ALLOWED` 表示使用了非 GET 方法，并返回 `Allow: GET`。来自 `file://` 页面的 `OPTIONS` 预检会返回 `204`。

```powershell
Invoke-RestMethod -Method Get -Uri "$base/health"
```

```bash
curl --fail-with-body "$BASE/health"
```

## 2. 列出打印机

**请求**：`GET /api/v1/printers`，无请求体。`demo` 模式返回明确的非打印 Mock Printer；`platform` 模式返回当前系统适配器实际枚举到的队列。

**成功响应（200）**：

```json
{
  "data": [
    {"name": "Mock Printer（不执行实体打印）", "is_default": true}
  ],
  "error": null
}
```

**状态码**：`200` 成功；`500 PRINTER_LIST_FAILED` 表示适配器枚举失败；`503 DEPENDENCY_UNAVAILABLE` 表示打印适配器没有装配；`405 METHOD_NOT_ALLOWED` 表示方法错误。

```powershell
(Invoke-RestMethod -Method Get -Uri "$base/api/v1/printers").data
```

```bash
curl --fail-with-body "$BASE/api/v1/printers"
```

## 3. 创建打印任务

**请求**：`POST /api/v1/print-jobs`，`Content-Type: application/json`。顶层只允许 `type`、`printer_name`、`payload`，三者都是必需字段；请求体上限为 1 MiB。

气球小票要求 `team_name`、`problem_id` 和 RFC3339 `solved_at`；其余字段可选：

```json
{
  "type": "balloon_ticket",
  "printer_name": "Mock Printer（不执行实体打印）",
  "payload": {
    "team_name": "星辰队",
    "problem_id": "C",
    "solved_at": "2026-08-19T14:32:10+08:00",
    "contest_name": "2026 校赛",
    "team_id": "team001",
    "room": "A101",
    "balloon_color": "red"
  }
}
```

源代码要求语言为 `cpp`、`go`、`python` 或 `java`，`source_code` 为 6–65536 个 UTF-8 字节；源码首尾空白会原样保留：

```json
{
  "type": "source_code",
  "printer_name": "Mock Printer（不执行实体打印）",
  "payload": {
    "language": "cpp",
    "source_code": "#include <iostream>\nint main() { return 0; }",
    "contest_name": "2026 校赛",
    "team_id": "team001",
    "team_name": "星辰队",
    "room": "A101",
    "problem_id": "C"
  }
}
```

**成功响应（202）**：`data` 是完整 Job；ID 为 32 位小写十六进制。异步 Worker 可能在客户端读取响应前已经推进状态，因此初始观测不应依赖一定为 `queued`。

```json
{
  "data": {
    "id": "0123456789abcdef0123456789abcdef",
    "type": "balloon_ticket",
    "printer_name": "Mock Printer（不执行实体打印）",
    "payload": {
      "team_name": "星辰队",
      "problem_id": "C",
      "solved_at": "2026-08-19T14:32:10+08:00"
    },
    "status": "queued",
    "created_at": "2026-08-19T06:32:11Z",
    "updated_at": "2026-08-19T06:32:11Z",
    "attempts": 0
  },
  "error": null
}
```

**状态码**：`202` 已创建；`400 INVALID_REQUEST` 表示 JSON、未知字段或业务字段不合法；`413 REQUEST_BODY_TOO_LARGE` 表示超过 1 MiB；`415 UNSUPPORTED_MEDIA_TYPE` 表示不是 `application/json`；`503 QUEUE_FULL` 或 `QUEUE_DELIVERY_FAILED` 表示队列不可用；`503 DEPENDENCY_UNAVAILABLE` 表示 Job Service 未装配；`500 INTERNAL_ERROR` 表示未分类内部错误；`405 METHOD_NOT_ALLOWED` 表示方法错误。

```powershell
$body = @{
  type = 'balloon_ticket'
  printer_name = 'Mock Printer（不执行实体打印）'
  payload = @{
    team_name = '星辰队'
    problem_id = 'C'
    solved_at = '2026-08-19T14:32:10+08:00'
  }
} | ConvertTo-Json -Depth 4
$created = Invoke-RestMethod -Method Post -Uri "$base/api/v1/print-jobs" -ContentType 'application/json' -Body $body
$jobID = $created.data.id
$created
```

```bash
curl --fail-with-body -X POST "$BASE/api/v1/print-jobs" \
  -H 'Content-Type: application/json' \
  --data '{"type":"balloon_ticket","printer_name":"Mock Printer（不执行实体打印）","payload":{"team_name":"星辰队","problem_id":"C","solved_at":"2026-08-19T14:32:10+08:00"}}'
```

## 4. 列出打印任务

**请求**：`GET /api/v1/print-jobs`，无请求体、无分页参数。

**成功响应（200）**：`data` 是 Job 数组；空仓库返回 `[]`。每个 Job 可能包含 `error`、`started_at`、`finished_at` 和 `pdf_path` 等生命周期字段。

```json
{
  "data": [
    {
      "id": "0123456789abcdef0123456789abcdef",
      "type": "balloon_ticket",
      "printer_name": "Mock Printer（不执行实体打印）",
      "payload": {"team_name":"星辰队","problem_id":"C","solved_at":"2026-08-19T14:32:10+08:00"},
      "status": "succeeded",
      "created_at": "2026-08-19T06:32:11Z",
      "updated_at": "2026-08-19T06:32:13Z",
      "started_at": "2026-08-19T06:32:11Z",
      "finished_at": "2026-08-19T06:32:13Z",
      "attempts": 1,
      "pdf_path": "data/jobs/0123456789abcdef0123456789abcdef/preview.pdf"
    }
  ],
  "error": null
}
```

**状态码**：`200` 成功；`503 DEPENDENCY_UNAVAILABLE` 表示 Job Service 未装配；`500 INTERNAL_ERROR` 表示存储/内部错误；`405 METHOD_NOT_ALLOWED` 表示方法错误。

```powershell
(Invoke-RestMethod -Method Get -Uri "$base/api/v1/print-jobs").data
```

```bash
curl --fail-with-body "$BASE/api/v1/print-jobs"
```

## 5. 查询任务详情

**请求**：`GET /api/v1/print-jobs/{jobID}`，无请求体。`jobID` 使用创建接口返回的值，不要自行拼文件路径。

**成功响应（200）**：

```json
{
  "data": {
    "id": "0123456789abcdef0123456789abcdef",
    "type": "source_code",
    "printer_name": "Mock Printer（不执行实体打印）",
    "payload": {"language":"cpp","source_code":"int main() { return 0; }"},
    "status": "failed",
    "error": {"code":"RENDER_FAILED","message":"PDF rendering failed"},
    "created_at": "2026-08-19T06:40:00Z",
    "updated_at": "2026-08-19T06:40:01Z",
    "started_at": "2026-08-19T06:40:00Z",
    "finished_at": "2026-08-19T06:40:01Z",
    "attempts": 1
  },
  "error": null
}
```

**状态码**：`200` 成功；`404 NOT_FOUND` 表示任务不存在；`503 DEPENDENCY_UNAVAILABLE` 表示 Job Service 未装配；`500 INTERNAL_ERROR` 表示存储/内部错误；`405 METHOD_NOT_ALLOWED` 表示方法错误。

```powershell
$base = 'http://127.0.0.1:17653'
$jobID = '0123456789abcdef0123456789abcdef'
Invoke-RestMethod -Method Get -Uri "$base/api/v1/print-jobs/$jobID"
```

```bash
BASE=http://127.0.0.1:17653
JOB_ID=0123456789abcdef0123456789abcdef
curl --fail-with-body "$BASE/api/v1/print-jobs/$JOB_ID"
```

## 6. 获取 PDF 预览

**请求**：`GET /api/v1/print-jobs/{jobID}/preview`，无请求体。该示例需要一个已经生成可读取 PDF 的 Job；任务尚未准备好时会返回 `PREVIEW_NOT_READY`。可发送标准 `Range: bytes=...` 请求。

**成功响应（200）**：响应体为 PDF 字节而非 JSON，关键响应头如下：

```text
HTTP/1.1 200 OK
Content-Type: application/pdf
Content-Disposition: inline; filename="preview.pdf"
X-Content-Type-Options: nosniff
```

合法范围请求返回 `206 Partial Content`；无效范围由 Go `http.ServeContent` 返回 `416 Requested Range Not Satisfiable`。

**状态码**：`200` 完整 PDF；`206` 部分 PDF；`404 NOT_FOUND` 表示任务不存在；`409 PREVIEW_NOT_READY` 表示任务尚无可读取 PDF；`416` 表示 Range 不可满足；`503 DEPENDENCY_UNAVAILABLE` 表示 Job Service 未装配；`500 INTERNAL_ERROR` 表示存储路径不符合安全边界或读取失败；`405 METHOD_NOT_ALLOWED` 表示方法错误。

```powershell
$base = 'http://127.0.0.1:17653'
$jobID = '0123456789abcdef0123456789abcdef' # 替换为已有 ready PDF 的 Job ID
Invoke-WebRequest -Method Get -Uri "$base/api/v1/print-jobs/$jobID/preview" -OutFile '.\preview.pdf'
```

```bash
BASE=http://127.0.0.1:17653
JOB_ID=0123456789abcdef0123456789abcdef # 替换为已有 ready PDF 的 Job ID
curl --fail-with-body "$BASE/api/v1/print-jobs/$JOB_ID/preview" --output preview.pdf
```

## 7. 重试失败任务

**请求**：`POST /api/v1/print-jobs/{jobID}/retry`，不需要请求体。该示例需要一个当前状态为 `failed` 的 Job；其他状态会返回 `RETRY_NOT_ALLOWED`。调用成功后清除旧错误、`started_at`、`finished_at`，保持已有 `attempts`，状态回到 `queued`。

**成功响应（200）**：异步 Worker 可能很快开始下一次尝试，因此客户端应继续查询详情，而不是假设读取到的状态一定仍为 `queued`。

```json
{
  "data": {
    "id": "0123456789abcdef0123456789abcdef",
    "type": "source_code",
    "printer_name": "Mock Printer（不执行实体打印）",
    "payload": {"language":"cpp","source_code":"int main() { return 0; }"},
    "status": "queued",
    "created_at": "2026-08-19T06:40:00Z",
    "updated_at": "2026-08-19T06:42:00Z",
    "attempts": 1
  },
  "error": null
}
```

**状态码**：`200` 已重新排队；`404 NOT_FOUND` 表示任务不存在；`409 RETRY_NOT_ALLOWED` 表示任务不是 `failed`；`503 QUEUE_FULL` 或 `QUEUE_DELIVERY_FAILED` 表示队列不可用；`503 DEPENDENCY_UNAVAILABLE` 表示 Job Service 未装配；`500 INTERNAL_ERROR` 表示存储/内部错误；`405 METHOD_NOT_ALLOWED` 表示方法错误。

```powershell
$base = 'http://127.0.0.1:17653'
$jobID = '0123456789abcdef0123456789abcdef' # 替换为 failed Job ID
$retried = Invoke-RestMethod -Method Post -Uri "$base/api/v1/print-jobs/$jobID/retry"
$retried.data
```

```bash
BASE=http://127.0.0.1:17653
JOB_ID=0123456789abcdef0123456789abcdef # 替换为 failed Job ID
curl --fail-with-body -X POST "$BASE/api/v1/print-jobs/$JOB_ID/retry"
```

## Job 状态与失败语义

| 状态 | 含义 |
| --- | --- |
| `queued` | 已持久化并等待单 Worker FIFO 处理。 |
| `rendering` | 正在生成 HTML/PDF；进入此状态时 `attempts + 1`。 |
| `printing` | PDF 已生成，正在调用所选打印适配器。 |
| `succeeded` | 适配器接受命令；不等于物理出纸。 |
| `failed` | 渲染、打印或重启恢复失败；查看 Job 的 `error.code/message`。 |

Worker 常见稳定错误包括 `RENDERER_NOT_FOUND`、`RENDERER_VERSION_UNSUPPORTED`、`RENDER_FAILED`、`PRINTER_NOT_FOUND`、`PRINT_COMMAND_FAILED` 和 `SERVICE_RESTARTED`。这些是持久化 Job 的失败原因，不代表创建请求本身的 HTTP 状态码。
