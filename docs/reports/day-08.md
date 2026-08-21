# 课题三日常报告（第 8 天）：交付收口与自测对照

## 基本信息

- 课题：浏览器调用本地打印机（课题三）。
- 成员：独立完成。
- 验证环境：2026 年 8 月 19 日，Windows amd64，Go `1.25.4`；module 指定 `go 1.23.0`。
- 今日范围：只完善文档、启动脚本、干净副本验证和提交材料，不新增 API 或打印功能。

## 今日目标

完成交付收口：跑全量回归；写出他人可跟做的启动说明；对照本组必做逐项填写自测表；完成安全审查自检，使第 9 天结课报告所依据的材料基本齐备。

## 回归结果

| 命令 | 结果 |
| --- | --- |
| `go test ./... -count=1 -v` | 141 个顶层测试：135 通过、0 失败、6 跳过 |
| `go test -race ./... -count=1` | 11 个含测试包通过，0 race |
| `go vet ./...` | exit 0，无诊断 |
| `go mod verify` | all modules verified |
| `git diff --check` | exit 0 |
| PowerShell AST / Bash `-n` | 两平台启动脚本语法通过 |

第 7 天通检条目在本日全部再测通过；当日无失败项。6 个 skip 为显式 opt-in 的真实 Chrome/服务 E2E、外部 `pdfinfo` 和普通账户不能创建 symlink 的场景，skip 不计为通过，已单列说明。

## 审查自检

| 检查项 | 验证方式与结论 |
| --- | --- |
| 报告中有无密钥、账号口令 | 逐篇复查 Day 1-8 日报与 `docs/` 全部文档：无密钥、口令；启动能力值（per-launch token）从未写入任何报告或截图。 |
| 工程侧如何确认密钥未写入仓库 | 对全部历史提交执行 `git grep` 检索 secret/token/password/api[_-]?key 等模式，无命中；配置一律从环境变量读取，仓库无 `.env` 或凭据文件。 |
| 外部输入或关键参数是否有校验 | 有：创建请求限 1 MiB、拒绝未知字段与非法 UTF-8、业务字段与 RFC3339 时间校验、源码 6-65536 字节边界；打印机名必须在本轮枚举 allowlist；preview 仅接受固定任务目录下的 `preview.pdf`，拒绝越界与链接逃逸。均有失败注入测试。 |
| 需鉴权的能力是否在服务端生效 | 本课题无账号体系（不适用）；等价控制已验证：服务只监听 `127.0.0.1`，可选 `file://` 页面必须携带每次启动随机生成的能力值，普通网页 Origin 不获授权（CORS 测试通过）。 |
| 依赖、数据路径是否真实可安装可运行 | 干净副本（排除 Git/缓存/旧数据，82 个文件）按 README 主命令一次启动成功并预览 PDF（见下节）；Chrome 自动发现实测可用；Go 依赖 `go mod verify` 通过。 |

## 他人如何启动

依赖：Go 1.23+；Chrome/Chromium 131+（自动发现，一般无需手动配置）。默认 demo 模式不访问系统打印队列。

1. 安装 Go 与 Chrome 后，在项目根目录执行（Windows）：

   ```powershell
   .\scripts\run-windows.ps1 -Mode demo -GoCachePath '.cache\go-build'
   ```

   Linux 为 `chmod +x ./scripts/run-linux.sh && ./scripts/run-linux.sh --mode demo --go-cache .cache/go-build`。
2. 终端输出监听地址后，浏览器打开该 URL；页面显示“Mock Printer（不执行实体打印）"。
3. 提交气球或源码任务，等待 `succeeded`，打开 PDF 预览。
4. 停止：启动终端 Ctrl+C；端口可重新绑定。

platform 模式（可选，需自行提供 SumatraPDF 或 CUPS 队列）：`-Mode platform -SumatraPath '.\tools\sumatra\SumatraPDF.exe'`；依赖缺失时启动即失败，不静默回退 demo。

## 自测对照表

| 本组必做 | 结论 | 说明 |
| --- | --- | --- |
| 气球小票与源码两类业务 PDF | 通过 | 真实 Chrome 渲染：气球 80 x 120 mm 单页、140 行源码 6 页，截图与几何断言在 Day 5 |
| FIFO 队列 | 通过 | 容量 100、单 Worker 顺序执行；并发创建 20 个无 race，第 101 个拒绝 |
| 任务状态与失败重试 | 通过 | 五状态、稳定错误码、retry 仅限 failed 并保留 attempts；重启恢复测试通过 |
| 本地 API 与页面 | 通过 | 7 路由、统一 envelope、200/206 preview、Node VM 执行真实 `app.js` 行为测试 |
| Windows Adapter | 部分完成 | 代码与受控 runner 已验证；系统队列证据当日未取得（后于 2026-08-21 在隔离队列补验完成，见文末补验记录） |
| Linux Adapter | 部分完成 | 代码、fixture 与交叉编译完成；无 Linux runtime（后于 2026-08-21 在 WSL2 补验完成，见文末补验记录） |
| 可复现文档 | 通过 | README、API 文档、测试说明、演示脚本与双平台脚本交付；干净副本 demo 复现成功 |

## 干净副本验收

干净副本排除 `.git`、工作树、临时目录、`data` 和缓存，共 82 个交付文件，严格按 README 主命令执行：

| 前提 | 操作 | 预期 | 实际 |
| --- | --- | --- | --- |
| 新副本无 Git/缓存/data | 执行 README 主命令 | 自动创建运行目录并发现浏览器 | 通过 |
| 17653 可用 | 请求 health | 监听回环；字段正确 | 通过 |
| 默认 demo | 查询打印机 | 只显示 Mock Printer | 通过 |
| 提交合法任务 | 轮询并请求 preview | `succeeded`、attempts=1、HTTP 200 PDF（35,180 bytes） | 通过 |
| 停止服务 | 等待退出再启动 | 端口可复用 | 通过 |

该结果证明实现者按当前 README 可复现 demo；原计划的真人冷启动验收已于 2026-08-22 取消，以本节复现记录为准（见下节说明）。

## 真人启动验收说明（2026-08-22 更新）

原计划的真人同学只看 README 冷启动验收已决定取消，不再作为交付要求；README 可用性以上节实现者干净副本复现为最终证据。系统队列录屏缺口已于 2026-08-21 双平台补齐（见文末补验记录）。

## 今日完成与自检

交付材料齐备：README、`docs/api.md`、`docs/testing.md`、`docs/demo-script.md`、双平台启动脚本、九天日报与证据截图。对照表与当日现状一致；两个“部分完成”项的边界（系统队列证据）已写明并列入补验计划，未被写成通过。当日还闭环了四个启动脚本问题：mode 参数缺失、默认 Go cache Access denied（增加 `-GoCachePath`）、demo 误校验无关 Sumatra 环境值、Linux 脚本 `.gitattributes` 固定 LF 防 `bash\r`。

## 问题、计划与 AI 沟通

- 不成功的沟通一例：让助手起草启动说明时，初稿中的命令与脚本真实参数不符（缺少 `-GoCachePath`）。对照终端真实输出逐条修改后才可跟做。
- 比较有效的沟通一例：按“只保留依赖、启动、停止三段，命令必须与本仓库脚本参数一致”的约束请助手改写，一次得到可用说明；又如要求“对照自测表逐项写结论，部分完成必须写边界”，避免了空勾选。
- 明日安排：结课大报告先写完成情况对照表与方案主路径两节，再整理验证与证据索引。

## 补验记录（2026-08-21）：Windows 系统队列录屏完成

- 安全目标：本地文件端口队列 `ISO-PDF-Queue`（Microsoft Print To PDF 驱动），固定输出到本机 print-iso 隔离目录下的 `iso-output.pdf`（仓库外）；提交前经 SumatraPDF 冒烟确认不出纸、不弹保存对话框。
- 录屏（约 4 分 13 秒）包含：系统版本（Windows 11 Pro build 22621）、SumatraPDF 3.6.1 版本、platform 模式启动、health 探活、`/api/v1/printers` 枚举、气球任务提交与 `succeeded` 状态、PDF 预览、print-iso 隔离目录下的输出文件、Ctrl+C 优雅停止后 health 连接被拒。
- 录屏任务 ID：`97654d58a864b473735d097571ba6b15`（attempts=1，系统队列落盘 289,600 字节 PDF）。
- 证据文件：`docs/reports/assets/windows-platform-evidence-2026-08-21.md`、`windows-iso-output-2026-08-21.pdf`、`windows-platform-queue-2026-08-21.mp4`（录屏不入版本控制）。
- 边界说明：本录屏证明系统队列接受与隔离文件输出，不等于物理出纸。

## 补验记录（2026-08-21）：Linux/CUPS runtime 与 request id 完成

- 环境：WSL2 Ubuntu 24.04.4 LTS（真实 Linux 内核）；Go 1.25.4；CUPS 2.4.7；Chrome 151；安全队列 `iso-queue`（cups-pdf:/ 后端，输出固定到 WSL 文件系统内 var 下的 print-iso 隔离目录（仓库外），不出纸、无弹窗，提交前冒烟验证）；服务以非特权用户运行。
- 真实 Linux 内核回归：`go test ./...`、`go test -race ./...`、`go vet`、`go mod verify` 全部通过。
- platform 模式端到端：枚举 CUPS 队列；中文气球任务 `07381f3ef5c6b9e2a73b86b34ad402d3` `queued -> succeeded`（attempts=1）；CUPS request id `iso-queue-8`（`lpstat -W all` 记录）；隔离输出 32362 字节 PDF；preview HTTP 200；SIGINT 优雅退出。
- 连续录屏（约 3 分 30 秒，`linux-platform-queue-2026-08-21.mp4`，不入版本控制）：覆盖环境信息、platform 启动、health、枚举、提交、状态、预览、隔离输出与 request id、优雅停止；录屏任务 `01d82152c37ba29a278d65c0c6544886`，request id `iso-queue-9`。
- 证据文件：`docs/reports/assets/linux-platform-evidence-2026-08-21.md`、`linux-iso-output-2026-08-21.pdf`。
- 边界说明：证明 Linux runtime 与系统队列接受（含 request id），不等于物理出纸；环境为 WSL2，如实标注。
