# 课题三日常报告（第 8 天）：可复现交付与提交准备

## 基本信息与今日目标

- 完成方式：独立完成。
- 验证环境：2026 年 8 月 19 日，Windows amd64，Go `1.25.4`；module 指定 `go 1.23.0`。
- 今日范围：只完善文档、启动脚本、干净副本验证和提交材料，不新增 API 或打印功能。
- 今日目标：让不了解代码的人能按 README 复现安全 demo，并明确列出仍需真人或平台环境完成的验收。

## 今日交付

| 交付物 | 内容 | 状态 |
| --- | --- | --- |
| `README.md` | 项目简介、依赖、demo/platform、启动、故障和限制 | 完成 |
| `docs/api.md` | 7 个接口的请求、响应、状态码与双端示例 | 完成 |
| `docs/demo-script.md` | 固定顺序的 8 分钟安全演示 | 完成 |
| `docs/testing.md` | 自动测试、失败注入和 opt-in E2E 边界 | 完成 |
| Windows/Linux 脚本 | 显式 mode、依赖检查、Go cache 参数 | 完成 |
| 干净副本 demo | 不依赖 Git/缓存/旧数据启动并预览 PDF | 完成 |
| 真人同学只看 README | 记录耗时、卡点、修订和复测 | 未完成 |
| Windows/Linux 系统队列录屏 | 安全目标、队列证据和连续录像 | Windows 已于 2026-08-21 补验完成；Linux 未完成 |
| Linux/CUPS runtime | `lp/lpstat`、request id 和队列记录 | 未完成 |

## 干净副本验收

干净副本排除 `.git`、工作树、临时目录、`data` 和缓存，共 82 个交付文件。严格按 README 的 Windows demo 主命令执行，不额外指定浏览器路径：

```powershell
.\scripts\run-windows.ps1 -Mode demo -GoCachePath '.cache\go-build'
```

| 前提 | 操作 | 预期结果 | 实际结果与结论 |
| --- | --- | --- | --- |
| 新副本无 Git、缓存和 data | 执行 README 主命令 | 自动创建运行目录并发现浏览器 | 通过 |
| `17653` 可用 | 等待启动输出并请求 health | 监听回环地址；service/status/API v1 正确 | 通过 |
| 默认 demo | 查询打印机 | 只显示不执行实体打印的 Mock Printer | 通过 |
| 提交合法任务 | 轮询并请求 preview | 最终 `succeeded`、attempts=1；HTTP 200 PDF | 通过，preview 为 35,180 bytes |
| 停止服务 | 等待退出并重新绑定端口 | HTTP/Worker 清理完成，端口可复用 | 通过 |

该结果只证明实现者按当前 README 可复现 demo，不替代真人同学测试、系统队列或物理出纸。

## 自动验证

所有 Go 命令使用项目相对 `.cache/` 解析出的实际 `GOCACHE`，默认只使用 Fake Printer 或受控 runner。

| 命令 | 结果 |
| --- | --- |
| `go test ./... -count=1 -v` | 141 个顶层测试：135 通过、0 失败、6 跳过 |
| `go test -race ./... -count=1` | 11 个含测试包通过，0 race |
| `go vet ./...` | exit 0，无诊断 |
| `go mod verify` | `all modules verified` |
| `git diff --check` | exit 0，无 whitespace error |
| PowerShell AST / Bash `-n` | 两个平台脚本语法通过 |

6 个 skip 仍是显式 opt-in 的真实 Chrome/服务 E2E、外部 `pdfinfo` 和普通账户不能创建 symlink 的安全场景。Node.js 22 可用，真实 `app.js` 行为测试已运行通过。

## 启动脚本问题闭环

1. 初版脚本没有 mode 参数，无法清楚区分安全演示和系统打印。新增静态契约先失败，再实现 `demo/platform`。
2. 干净副本首次启动时默认 Go build cache 返回 Access denied。增加 `-GoCachePath` / `--go-cache` 后，显式项目内缓存可正常运行。
3. demo 曾错误校验无关的旧 Sumatra 环境值。修正后 demo 完全忽略平台依赖，platform 仍严格要求 Sumatra 或 CUPS。
4. Linux 脚本通过 `.gitattributes` 固定 LF，防止 Windows 打包后 shebang 出现 `bash\r`。

## 真人启动与录屏缺口

当前没有真人同学只看 README 的原始记录。执行时必须给对方全新副本且不做口头提示，记录系统版本、启动耗时、报错原文和卡点；修订 README 后由同一人重新开始。实现者自测和代理审查均不能替代这一项。

当前也没有录屏文件。安全补录必须先确认目标不会进入实体设备且不会弹出阻塞保存对话框：

| 平台 | 录屏必须出现的证据 |
| --- | --- |
| Windows | 系统版本、platform 启动、真实队列枚举、任务状态、preview、已确认隔离输出，以及“队列接受不等于出纸”说明 |
| Linux | `uname`、`lpstat -p/-d`、platform 启动、源码多页 preview、CUPS request id 和 `lpstat -o` |

如果没有安全目标，只录 demo 并如实说明，不为取得材料向实体打印机试投。

## 补验记录（2026-08-21）：Windows 系统队列录屏完成

上表 Windows 行要求的全部证据已于 2026-08-21 补齐：

- 安全目标：本地文件端口队列 `ISO-PDF-Queue`（Microsoft Print To PDF 驱动），固定输出到本机 print-iso 隔离目录下的 `iso-output.pdf`（仓库外）；提交前经 SumatraPDF 冒烟确认不出纸、不弹保存对话框。
- 录屏（约 4 分 13 秒）包含：系统版本（Windows 11 Pro build 22621）、SumatraPDF 3.6.1 版本、platform 模式启动、health 探活、`/api/v1/printers` 枚举、气球任务提交与 `succeeded` 状态、PDF 预览、print-iso 隔离目录下的输出文件、Ctrl+C 优雅停止后 health 连接被拒。
- 录屏任务 ID：`97654d58a864b473735d097571ba6b15`（attempts=1，系统队列落盘 289,600 字节 PDF）。
- 证据文件：`docs/reports/assets/windows-platform-queue-2026-08-21.mp4`、`windows-iso-output-2026-08-21.pdf`、`windows-platform-evidence-2026-08-21.md`。
- 边界说明：本录屏证明系统队列接受与隔离文件输出，不等于物理出纸；Linux 录屏与 CUPS request id 缺口保持不变。

## AI 沟通与自检

Codex 只读审查曾作为“陌生读者”检查 README。最初容易把“代理未发现问题”写成“他人启动验收完成”；加入“必须是真人、只看 README、记录原话卡点和同一人复测”的约束后，报告把代理审查降回自动文档检查，并将真人验收保留为未完成。这一修正提高了证据可信度。

自检结果：范围、依赖、接口、安全、日志和异常处理与代码一致；README demo 已在干净副本复现；Windows 系统队列录屏已于 2026-08-21 补验完成；真人启动、Linux/CUPS runtime 和 Linux 录屏没有被替代或虚构。

## 第 9 天计划

第 9 天不新增功能，只整理结课报告：前置最终范围与完成矩阵，汇总架构、API、两类 PDF、平台边界、测试统计、真实问题和九天证据索引。人工材料若仍未补齐，最终报告继续标为未完成，并给出可执行补验步骤。
