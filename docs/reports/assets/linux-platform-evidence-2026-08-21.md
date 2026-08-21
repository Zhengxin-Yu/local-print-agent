# Linux/CUPS 平台 runtime 与系统队列证据（2026-08-21 补验）

本文件记录 PROJECT_HANDOFF.md P0-B 的补验结果。证据层级：代码存在 -> 受控命令测试 -> **Linux 平台 runtime（本次达成）** -> **系统队列接受（本次达成，含 CUPS request id）** -> **隔离文件输出（本次达成）**。本次验证不涉及物理出纸。

## 环境

| 项目 | 值 |
| --- | --- |
| 运行环境 | WSL2 Ubuntu 24.04.4 LTS，内核 6.18.33.2-microsoft-standard-WSL2（真实 Linux 内核，非交叉编译） |
| Go | go1.25.4 linux/amd64（GOTOOLCHAIN=local） |
| CUPS | cups/cups-client 2.4.7-1.2ubuntu7.14（`lp`、`lpstat` 实际调用） |
| 浏览器 | Google Chrome 151.0.7922.173（真实 PDF 渲染） |
| 安全队列 | `iso-queue`（cups-pdf:/ 后端，输出固定到 WSL 文件系统内 var 下的 print-iso 隔离目录（仓库外），不出纸、无交互弹窗；提交前已用 `lp` 冒烟验证落盘） |
| 运行账户 | 非特权用户 `printuser`（Chrome 沙箱在 root 下不可用，故按真实用户场景运行） |
| 服务模式 | platform（`./scripts/run-linux.sh --mode platform --go-cache .cache/go-build`） |
| 监听地址 | http://127.0.0.1:17653 |

## 自动回归（真实 Linux 内核）

| 命令 | 结果 |
| --- | --- |
| `go test ./... -count=1` | 12 个含测试包全部 `ok`，0 失败 |
| `go test -race ./... -count=1` | 12 个含测试包全部 `ok`，0 race（cgo + gcc 真实运行） |
| `go vet ./...` | exit 0 |
| `go mod verify` | all modules verified |

`internal/printer` 的 Linux build-tag 测试本次在真实 Linux 内核执行，不再是 Windows 上的交叉编译验证。

## 端到端主路径（20:06）

1. platform 模式启动后 `GET /api/v1/printers` 实际枚举到 `iso-queue` 与 `PDF` 两个 CUPS 队列（`is_default=true` 的 `PDF` 同为 cups-pdf 虚拟队列，无实体设备）。
2. 提交气球任务（中文队伍"星辰队"、题号 C、`printer_name=iso-queue`）。
3. 任务 ID：`07381f3ef5c6b9e2a73b86b34ad402d3`。
4. 状态流转：`queued -> succeeded`，attempts=1，created 2026-08-21T12:06:41.56Z，finished 2026-08-21T12:06:42.29Z（约 0.73 秒，含 Chrome 渲染）。
5. **CUPS 系统队列接受证据（request id）**：`lpstat -W all` 显示 `iso-queue-8  printuser  51200  Fri Aug 21 20:06:42 2026`。
6. **隔离输出**：print-iso 隔离目录下的 `preview.pdf` 32362 字节，`%PDF-` 文件头，属主 `printuser`（cups-pdf 后端真实产出，经 Ghostscript 重蒸馏），副本存于 `docs/reports/assets/linux-iso-output-2026-08-21.pdf`。
7. `GET /api/v1/print-jobs/{id}/preview` 返回 HTTP 200，50869 字节。
8. 向进程组发送 SIGINT 后服务优雅退出，端口 17653 关闭（health 连接被拒）。

## 措辞边界

- 该证据链（真实 Linux 内核 + 真实 `lp`/`lpstat` 调用 + CUPS request id + 隔离输出文件）证明 Linux 平台 runtime 与系统队列接受，不等于物理出纸。
- 运行环境为 WSL2（真实 Linux 内核但非物理机/独立虚拟机），报告如实标注。
- 未使用系统默认队列试投；所有提交都显式指定已确认安全的 `iso-queue`。
- 本文件不含启动能力值（token）与个人用户目录路径。
