# local-print-agent

## 项目简介

`local-print-agent` 是一个用 Go 编写的跨平台本地打印代理。浏览器在回环地址上提交气球小票或源代码任务，服务按 FIFO 顺序生成 PDF、提交给所选打印适配器，并持久化任务状态和失败原因。

默认的 `demo` 模式会使用真实 Chrome/Chromium 生成 PDF，但打印阶段只调用 **Mock Printer（不执行实体打印）**。只有显式选择 `platform` 模式，任务才可能进入 Windows 或 Linux 的系统打印队列。状态 `succeeded` 仅表示当前适配器接受了命令，不保证物理出纸。

## 架构

```text
嵌入式 Web 控制台
        │ HTTP/JSON（127.0.0.1:17653–17660）
        ▼
HTTP API ── Job Service ── JSON Store
                   │
                   ▼
             单 Worker FIFO
                   │
          HTML/Chroma → Chrome PDF
                   │
          demo Fake / platform Adapter
                   │
        Windows SumatraPDF / Linux CUPS
```

Web、API、Worker 和适配器运行在同一进程。任务状态为 `queued → rendering → printing → succeeded`；渲染或打印失败进入 `failed`，失败任务可手动重试。

## 环境要求

- Go 1.23 或更新版本；`go` 必须在 `PATH` 中。
- Chrome 或 Chromium 131+；推荐把可执行文件绝对路径显式传给启动脚本。
- 可用端口：回环地址 `127.0.0.1:17653` 至 `17660` 中至少一个未被占用。
- Windows `platform` 模式：SumatraPDF 可执行文件，以及当前账户可枚举的 Windows 打印队列。
- Linux `platform` 模式：CUPS client，必须提供 `lp` 和 `lpstat`；还需要预先配置好的打印队列。
- 可选：Node.js。安装后 `web` 包测试会真实执行页面 JavaScript；未安装时该增强测试跳过。
- 可选：curl 7.76+，只用于 Linux 命令行 API/演示中的 `--fail-with-body`；Web 主路径不依赖 curl。
- 可选：Git，只用于 Git checkout 中的 `git diff --check`；无 `.git` 的解压交付包跳过该项。

首次执行 `go run`/`go test` 需要能够从 Go module proxy 或已有 module cache 取得 `go.mod` 中的依赖。

## Windows 安装与运行

1. 安装 Go 1.23+、Chrome/Chromium 131+，克隆或解压本仓库。
2. 在仓库根目录打开 PowerShell。
3. 先用不访问系统打印队列的 demo 模式验收：

   ```powershell
   $goCache = Join-Path $env:TEMP 'local-print-agent-gocache'
   .\scripts\run-windows.ps1 -Mode demo -BrowserPath 'C:\Program Files\Google\Chrome\Application\chrome.exe' -GoCachePath $goCache
   ```

4. 终端会输出实际 URL，例如 `http://127.0.0.1:17653`。在浏览器打开它；如果 17653 被占用，以终端实际输出为准。
5. 按 `Ctrl+C` 停止服务。

仅在确认目标队列安全后使用平台模式：

```powershell
.\scripts\run-windows.ps1 `
  -Mode platform `
  -BrowserPath 'C:\Program Files\Google\Chrome\Application\chrome.exe' `
  -SumatraPath 'C:\Tools\SumatraPDF\SumatraPDF.exe' `
  -GoCachePath (Join-Path $env:TEMP 'local-print-agent-gocache')
```

平台模式不会自动退回 Mock Printer。SumatraPDF 路径缺失或不可用时，脚本会在启动前失败；打印机枚举或命令失败会以稳定错误写入任务。

## Linux 安装与运行

1. 安装 Go 1.23+ 和 Chrome/Chromium 131+，克隆或解压本仓库。
2. 在仓库根目录执行：

   ```bash
   chmod +x ./scripts/run-linux.sh
   ./scripts/run-linux.sh --mode demo --browser-path /usr/bin/google-chrome --go-cache /tmp/local-print-agent-gocache
   ```

   Chromium 的常见路径也可以是 `/usr/bin/chromium` 或 `/usr/bin/chromium-browser`，请使用本机真实路径。
3. 打开终端输出的 `http://127.0.0.1:<实际端口>`，按 `Ctrl+C` 停止。

仅在已经检查 CUPS 队列且确认目标不会误出纸时使用平台模式：

```bash
command -v lp lpstat
lpstat -p
./scripts/run-linux.sh --mode platform --browser-path /usr/bin/google-chrome --go-cache /tmp/local-print-agent-gocache
```

脚本会检查 `lp`、`lpstat`，但不会安装 CUPS、创建队列或修改默认打印机。

## 配置

| 参数/环境变量 | 值 | 默认值 | 说明 |
| --- | --- | --- | --- |
| Windows `-Mode` / Linux `--mode` / `LOCAL_PRINT_AGENT_PRINTER_MODE` | `demo`、`platform` | `demo` | `demo` 永不进入系统队列；`platform` 显式使用当前系统适配器。命令行参数优先于环境变量。 |
| Windows `-BrowserPath` / Linux `--browser-path` / `LOCAL_PRINT_AGENT_BROWSER_PATH` | 浏览器绝对路径 | 自动发现 | 显式路径必须是已有普通文件；浏览器版本须为 131+。 |
| Windows `-SumatraPath` / `LOCAL_PRINT_AGENT_SUMATRA_PATH` | SumatraPDF 绝对路径 | 无 | 仅 Windows `platform` 模式必需。 |
| Windows `-GoCachePath` / Linux `--go-cache` / `GOCACHE` | 可写目录 | Go 默认缓存 | 受限账户无法写默认缓存时显式指向临时目录；不要放入交付包。 |

服务固定监听 `127.0.0.1`，依次尝试端口 17653–17660；运行数据固定写入仓库根目录的 `data/`。当前没有修改主机、端口或数据目录的 CLI 参数。`data/` 已被 Git 忽略，不应加入交付包。

## 使用步骤

1. 先以 `demo` 模式启动并打开终端给出的 URL。
2. 确认页面显示“已连接”、API `v1`，打印机为“Mock Printer（不执行实体打印）”。
3. 填写队伍名称、题号、通过时间，创建气球小票任务。
4. 等任务进入 `succeeded`，点“详情”再打开 PDF 预览。
5. 选择 `cpp`、`go`、`python` 或 `java`，输入至少 6 个、最多 65536 个 UTF-8 字节的源码，创建源码任务并检查行号、高亮和分页预览。
6. 失败任务才会显示“重试”。重试会清除上次错误和本次运行时间，再重新进入 FIFO 队列；`attempts` 在下一次开始渲染时递增。
7. 只有完成 demo 验收、明确核对系统队列后，才另行停止服务并用 `platform` 模式重启。

完整接口及 curl/PowerShell 示例见 [docs/api.md](docs/api.md)，逐分钟演示顺序见 [docs/demo-script.md](docs/demo-script.md)。

## 测试

从仓库根目录执行：

```powershell
$env:GOTOOLCHAIN = 'local'
$env:GOCACHE = Join-Path $env:TEMP 'local-print-agent-test-gocache'
go test ./... -count=1 -v
go test -race ./... -count=1
go vet ./...
go mod verify
if (Test-Path -LiteralPath '.git') { git diff --check }
```

Linux Bash 中对应写法：

```bash
export GOTOOLCHAIN=local
export GOCACHE=/tmp/local-print-agent-test-gocache
go test ./... -count=1 -v
go test -race ./... -count=1
go vet ./...
go mod verify
if [[ -e .git ]]; then git diff --check; fi
```

`go test -race` 另外需要 Go 支持的 OS/架构、已启用 cgo 和可用 C 编译器；环境不具备时应保留错误，不影响单独运行非 race 回归。默认测试使用 Fake Printer 或受控命令 runner，不向实体或系统打印队列提交任务。真实 Chrome E2E、Linux/CUPS 运行边界见 [docs/testing.md](docs/testing.md)。

## 常见错误

| 现象/错误码 | 原因与处理 |
| --- | --- |
| `RENDERER_NOT_FOUND` | Chrome/Chromium 路径不存在。给脚本传入正确绝对路径。 |
| `RENDERER_VERSION_UNSUPPORTED` | 浏览器低于 131 或版本无法识别。升级浏览器后重启。 |
| `unsupported printer mode` / 脚本 mode 错误 | 只允许 `demo` 或 `platform`。 |
| Windows 提示需要 SumatraPDF | `platform` 模式必须传 `-SumatraPath` 或设置对应环境变量。 |
| `CUPS client commands are unavailable` | Linux 缺少 `lp`/`lpstat`；安装发行版的 CUPS client 软件包。 |
| `PRINTER_NOT_FOUND` | 平台枚举不到目标队列，或该名称不再位于本次枚举允许列表。 |
| `PREVIEW_NOT_READY` | PDF 尚在生成或生成失败；刷新任务详情，先检查任务状态和错误。 |
| `print queue is full` | 内存队列容量为 100；等待已有任务完成后再提交或重试。 |
| 17653 无法访问 | 服务可能回退到 17654–17660；使用终端输出的实际 URL。 |
| Chrome 返回 `context canceled` | 可能受容器、桌面权限或组策略限制；在普通本机终端复验并保留原始失败，不能以单元测试替代真实 Chrome 证据。 |

## 限制

- 不实现完整 OJ、登录权限、浏览器插件、桌面 GUI、云打印或自研驱动。
- 只提供单 Worker FIFO，队列容量 100；不并发打印。
- 运行数据保存在单机 JSON 文件，不是多进程数据库。
- OS 命令提交存在 at-least-once 崩溃窗口：系统已接受但成功状态尚未持久化时，人工重试可能重复提交。
- `succeeded` 不等于物理出纸；虚拟/实体队列结果必须另行取证。
- 当前仓库的 Windows 系统队列、Linux/CUPS 实跑、真人仅看 README 启动和双平台录屏仍需在具备相应环境时人工完成，不能用受控测试或交叉编译替代。

## 目录结构

```text
cmd/local-print-agent/  主程序装配、监听与关闭
internal/config/        固定端口和环境变量配置
internal/httpapi/       7 个 HTTP 接口与嵌入式网页路由
internal/jobs/          请求校验、任务模型、状态机和 FIFO Service
internal/render/        HTML、Chroma 高亮、Chrome PDF 与安全发布
internal/printer/       demo、Windows SumatraPDF、Linux CUPS 适配器
internal/store/         JSON 持久化和重启恢复
internal/worker/        单线程任务处理与存储重试
templates/              气球和源码 HTML 模板
web/                    嵌入式控制台
testdata/               两类任务验收样例
scripts/                Windows/Linux 显式模式启动入口
docs/                   API、测试、演示和每日报告
```

## 参考项目

- [CCPCOJ](https://github.com/CSGrandeur/CCPCOJ)：借鉴任务编号、排队、现场分发和可审计状态；本项目不实现 OJ。
- [Lodop/C-Lodop 官方演示](https://www.lodop.net/LodopDemo_iframe.html)：借鉴“浏览器调用本机服务”的产品边界；本项目不依赖 Lodop。
- 课程《实践指导书》课题三“浏览器调用本地打印机”：本项目的验收范围来源。
