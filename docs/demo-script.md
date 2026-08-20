# 8 分钟演示脚本

本脚本用于安全演示 `demo` 模式：Chrome/Chromium 生成真实 PDF，Mock Printer 不访问 Windows/Linux 系统队列。录制前删除或移走旧 `data/`，准备 Chrome 131+、两个终端窗口，并把终端字号调到可读。不要展示密码、用户名目录中的无关文件或实体打印机名称。

Linux 命令行示例需 curl 7.76+ 才支持 `--fail-with-body`；不具备时可直接使用 Web 页面，不影响代理主路径。

预先启动（不计入 8 分钟）：

```powershell
.\scripts\run-windows.ps1 -Mode demo -GoCachePath '.cache\go-build'
```

或：

```bash
./scripts/run-linux.sh --mode demo --go-cache .cache/go-build
```

记录终端输出的实际 URL，并令 `$base`/`$BASE` 指向它。演示默认直接打开该同源 URL，不从磁盘打开 HTML。终端同时给出的 `web/index.html?local_print_agent_token=<per-launch-token>` 只用于可选 `file://` 模式，能力值每次启动变化，不应在录屏或报告中复用。准备一段至少 6 字节、含中文注释和缩进的 C++ 源码。

如果默认 Go cache 不可写，在用于 API/测试的第二终端也独立设置（启动脚本不会修改父终端环境）：

```powershell
$env:GOTOOLCHAIN = 'local'
$cacheDir = New-Item -ItemType Directory -Force '.cache\go-demo-test'
$env:GOCACHE = $cacheDir.FullName
```

```bash
export GOTOOLCHAIN=local
mkdir -p .cache/go-demo-test
export GOCACHE="$PWD/.cache/go-demo-test"
```

## 00:00–00:30 背景（30 秒）

**画面**：README 标题和 Web 控制台首页。

**讲解**：浏览器不能安全、统一地直接控制本机打印机，因此本项目在回环地址提供 Go 本地服务。它支持气球小票和源码两类任务，给出排队、渲染、打印提交、成功或失败状态。项目不实现 OJ、驱动或云打印。

**必须说清**：本次是 demo 模式，不访问系统打印队列；`succeeded` 表示 Mock Adapter 接受命令，不等于物理出纸。

## 00:30–01:30 架构（60 秒）

**画面**：README“架构”图与目录结构。

**讲解顺序**：

1. 嵌入式同源 Web 调用 7 个 HTTP 接口，只监听 `127.0.0.1`；可选 `file://` 模式必须携带终端给出的本次启动能力值。
2. Job Service 先写 JSON Store，再把 ID 放入容量 100 的 FIFO。
3. 单 Worker 依次执行 `queued → rendering → printing → succeeded/failed`。
4. 气球使用 HTML 模板；源码由 Chroma 高亮；Chrome 131+ 固化为 PDF。
5. 默认 Fake Adapter 不打印；显式 `platform` 才选择 Windows SumatraPDF 或 Linux CUPS。

## 01:30–02:00 服务探活（30 秒）

**画面**：启动终端先显示“demo mode”，再显示实际 loopback URL；第二终端执行：

```powershell
$base = 'http://127.0.0.1:17653' # 按终端实际端口修改
Invoke-RestMethod "$base/health"
(Invoke-RestMethod "$base/api/v1/printers").data
```

```bash
BASE=http://127.0.0.1:17653 # 按终端实际端口修改
curl --fail-with-body "$BASE/health"
curl --fail-with-body "$BASE/api/v1/printers"
```

**指出**：`service=local-print-agent`、`api_version=v1`、`status=ok`；打印机名称明确写着“不执行实体打印”。如果 17653 被占用，服务会使用 17654–17660 中的首个可用端口。

## 02:00–03:30 气球打印（90 秒）

**画面**：Web 气球表单、任务列表、详情和 PDF 预览。

**操作**：

1. 输入队伍名称“星辰队”、题号“C”和当前通过时间。
2. 创建任务，指出页面每 2 秒刷新。
3. 展示最终 `succeeded`、32 位任务 ID、`attempts=1` 和 `pdf_path`。
4. 点“打开 PDF 预览”，展示 80 mm × 120 mm 窄纸、中文文本和任务编号。

**讲解**：HTTP 创建返回 202；后台异步推进，所以页面不保证能捕捉每个瞬时状态，但 Worker 测试逐项断言完整状态顺序。

## 03:30–05:00 源码打印（90 秒）

**画面**：源码表单和 PDF 预览；需要稳定多页时可直接使用 `testdata/source_cpp.json` 的源码内容填入页面。

**操作**：

1. 选择 C++，粘贴含 Tab 缩进、`<iostream>` 和中文注释的源码。
2. 创建任务，等待 `succeeded`，打开预览。
3. 指出源码的缩进、HTML 字符转义、行号、语法高亮、A4 分页、页眉和页码。

**讲解**：源码允许 6–65536 个 UTF-8 字节，支持 `cpp/go/python/java`；规范化不会删除首行 Tab 或末尾换行。

## 05:00–06:00 失败与重试（60 秒）

**画面**：第二终端。先用 API 制造一个不会入队的验证失败：

```powershell
$bad = @{type='source_code';printer_name='Mock Printer（不执行实体打印）';payload=@{language='rust';source_code='fn main() {}'}} | ConvertTo-Json -Depth 4
try { Invoke-RestMethod -Method Post -Uri "$base/api/v1/print-jobs" -ContentType 'application/json' -Body $bad } catch { $_.ErrorDetails.Message }
```

```bash
curl -i -X POST "$BASE/api/v1/print-jobs" \
  -H 'Content-Type: application/json' \
  --data '{"type":"source_code","printer_name":"Mock Printer（不执行实体打印）","payload":{"language":"rust","source_code":"fn main() {}"}}'
```

指出 HTTP 400 / `INVALID_REQUEST`。然后运行不访问设备的失败与重试测试：

```powershell
go test ./internal/worker -run 'TestWorker(RecordsRenderFailure|RecordsPrintCommandFailure|PreservesStablePrinterAdapterError)$' -count=1 -v
go test ./internal/jobs -run '^TestServiceRetryOnlyFailedJobAndResetsLifecycle$' -count=1 -v
```

上述两条 `go test` 在 Bash 中命令相同，保留单引号即可。

**必须说清**：400 请求不会创建 Job，所以不能重试。持久化任务只有进入 `failed` 后才可调用 retry；测试验证旧错误和运行时间被清除、已有 attempts 保留、下一次渲染再递增。此处使用受控失败注入，不冒险调用真实队列。

## 06:00–07:00 Windows/Linux 证据（60 秒）

**画面**：`docs/reports/day-06.md` 的双平台适配表、`docs/reports/day-07.md` 的环境边界，再展示 Day 5 三张真实 PDF 截图。

**讲解**：Windows 受控测试验证 PowerShell 枚举和 SumatraPDF 严格参数、allowlist、路径与可执行文件身份；Linux build-tag 测试验证 `lpstat`/`lp` 参数和 symlink 边界，并已交叉编译。当前材料没有 Windows 系统队列接受记录，也没有 Linux 内核运行/CUPS request id；双平台真人录屏仍待在安全虚拟队列环境补做，不能把受控测试说成实机打印。

如已补齐真人录屏，本段改为并排播放并指出：系统信息、启动命令、health、任务提交、状态、可控虚拟队列结果；仍要区分“队列接受”和“物理出纸”。

## 07:00–08:00 测试和总结（60 秒）

**画面**：终端中的最新完整验证摘要和 `docs/reports/day-08.md` 必做对照表。

```powershell
$env:GOTOOLCHAIN = 'local'
$cacheDir = New-Item -ItemType Directory -Force '.cache\go-demo-test'
$env:GOCACHE = $cacheDir.FullName
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go mod verify
if (Test-Path -LiteralPath '.git') { git diff --check }
```

```bash
export GOTOOLCHAIN=local
mkdir -p .cache/go-demo-test
export GOCACHE="$PWD/.cache/go-demo-test"
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go mod verify
if [[ -e .git ]]; then git diff --check; fi
```

**总结**：两类真实 PDF、FIFO、持久化、预览、失败原因和显式双平台边界已经实现；默认 demo 安全可复现。尚未由证据完成的真人 README 启动、Windows/Linux 安全队列录屏和 Linux runtime 验收继续标为未完成，不用自动测试替代。

## 录制后检查

- 总时长接近 08:00，八段顺序和时长均未调整。
- 画面没有密码、token、无关个人文件或未经确认的实体打印机名称。
- demo 画面明确出现“不执行实体打印”。
- 双平台证据按实际情况措辞；没有录像就保留“未完成”，不放占位假文件。
