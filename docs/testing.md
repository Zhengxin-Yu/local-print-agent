# 测试与回归说明

## 安全边界

自动测试默认只使用 Fake Printer 或受控命令 runner，不得向实体打印机、交互式虚拟打印机或未确认的系统队列提交任务。`succeeded` 只表示选定 adapter 接受了 PDF，不表示物理出纸。

真实 Chrome 测试只生成 PDF；它不会调用 SumatraPDF、CUPS 或系统打印队列。平台打印测试通过受控 runner 断言参数、超时、允许列表和错误映射。

## 日常自动测试

从仓库根目录执行：

```powershell
$env:GOTOOLCHAIN = "local"
go test ./... -count=1 -v
go test -race ./... -count=1
go vet ./...
go mod verify
git diff --check
```

受限环境如果不允许写入默认 Go build cache，可先创建已忽略的项目相对目录 `.cache/go-test`，再按 README 的测试片段将其解析为绝对路径后赋给 `GOCACHE`；测试后可删除该缓存。`web` 包在安装 Node.js 时会真实执行 `app.js` 的发现、列表渲染、文本安全和定时器清理行为；没有 Node.js 时该增强用例跳过，便携的 Go 静态安全契约仍会执行。

## 失败注入与边界内容

| 类别 | 受控入口 | 必要断言 |
|---|---|---|
| 打印机不存在 | adapter 枚举中不包含目标名 | `PRINTER_NOT_FOUND`，不执行打印命令 |
| Chrome 不存在 | 显式传入不存在的 browser path | `RENDERER_NOT_FOUND`，不泄漏路径 |
| SumatraPDF 不存在 | Windows adapter 传入不存在的 exe | `PRINT_COMMAND_FAILED`，不启动外部命令 |
| CUPS 客户端不存在 | Linux build-tag 测试注入 `LookPath` 失败 | 稳定安装提示，不泄漏工具路径 |
| 模板非法 | 将不完整 Go template 传入真实解析器 | 解析失败，Worker 持久化 `RENDER_FAILED` |
| 请求超限 | 大于 1 MiB 的 HTTP JSON | HTTP 413 / `REQUEST_BODY_TOO_LARGE` |
| 队列满 | 使用真实 Service 提交 101 个任务 | 前 100 个持久化，第 101 个返回 503 / `QUEUE_FULL` |
| 服务重启 | 重新打开 JSON Store 后执行恢复 | `rendering/printing` 持久化为 `SERVICE_RESTARTED`，`queued` 保持待处理 |

内容边界必须同时覆盖：中文队名和注释、首行 Tab 与末尾换行原样保留、HTML 标签转义、65536 UTF-8 字节上界接受与 65537 字节拒绝、长行换行 CSS、多页源码、非法 RFC3339 时间。

## 真实 Chrome PDF 回归

```powershell
$env:LOCAL_PRINT_AGENT_CHROME_E2E = '.\tools\chrome\chrome.exe'
$env:LOCAL_PRINT_AGENT_PDFTOPPM_E2E = '.\tools\poppler\pdftoppm.exe'
go test ./internal/render -run '^TestPDFRendererChromeIntegration$' -count=1 -v
go test ./cmd/local-print-agent -run '^TestRealServiceRendersBothJobsServesPreviewAndCleansUp$' -count=1 -v
```

第一条生成一页气球 PDF 和多页中文源码 PDF；如配置 `pdftoppm`，还会检查前两页的页眉、正文、页码间隔。第二条启动真实 loopback HTTP 服务，但打印端仍是 Fake Printer。

若无头浏览器被容器、桌面权限或组策略禁止，必须记录完整失败，不得用 PDF 单元测试替代并宣称真实 Chrome 通过。

## Linux/CUPS 验证

Windows 可交叉编译 Linux build-tag 测试，但交叉编译不等于在 Linux 内核上运行：

```powershell
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go test -c ./internal/printer -o linux-printer.test
```

真实 Linux 环境中应执行 `go test -race ./... -count=1`，确认 `lp`/`lpstat` 可用后，只向明确的可控虚拟队列提交 PDF，保存 request id 和 `lpstat -o` 证据。本地没有 Linux runtime 时不得声称 CUPS 测试已运行。
