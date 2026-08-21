# Windows 平台 runtime 与系统队列证据（2026-08-21 补验）

本文件记录 PROJECT_HANDOFF.md P0-A 的补验结果。证据层级：代码存在 -> 受控命令测试 -> **Windows 平台 runtime（本次达成）** -> **系统队列接受（本次达成）** -> **隔离文件输出（本次达成）**。本次验证不涉及物理出纸。

## 环境

| 项目 | 值 |
| --- | --- |
| 操作系统 | Microsoft Windows 11 Pro（10.0.22621，build 22621） |
| Go 工具链 | 本地工具链（GOTOOLCHAIN=local），module 要求 go 1.23.0 |
| 打印驱动 | Microsoft Print To PDF |
| 安全队列 | `ISO-PDF-Queue`（本地文件端口，固定输出到本机 print-iso 隔离目录下的 `iso-output.pdf`，仓库外） |
| 队列安全确认 | 该队列为本地文件端口隔离目标：不出纸、不弹保存对话框、静默写文件；提交前已用 SumatraPDF 冒烟验证 |
| 打印适配器 | SumatraPDF 3.6.1，位于项目 `tools/sumatra/SumatraPDF.exe`（该目录被 .gitignore 忽略，不入库） |
| 服务模式 | platform（`.\scripts\run-windows.ps1 -Mode platform -SumatraPath .\tools\sumatra\SumatraPDF.exe -GoCachePath .cache\go-build`） |
| 监听地址 | http://127.0.0.1:17653 |

## 冒烟与自动化端到端（15:16-15:18）

1. 提交前用 SumatraPDF `-print-to ISO-PDF-Queue -silent` 向隔离队列试投一份已有 PNG，确认无弹窗且生成 PDF，验证队列安全性。
2. platform 模式启动服务后 `GET /api/v1/printers` 实际枚举到 4 个系统队列，含 `ISO-PDF-Queue`。
3. 提交气球任务：`POST /api/v1/print-jobs`。
4. 任务 ID：`9b85f85ae0ecd050f2c3ddbc6542b3ce`（payload：Team Alpha / 题号 C / 2026-08-21T15:16:00+08:00）。
5. 状态流转：`queued -> succeeded`，attempts=1，created 2026-08-21T07:17:27Z，finished 2026-08-21T07:17:28Z（约 1.8 秒）。
6. 系统队列产出：print-iso 隔离目录下的 `iso-output.pdf` 303,817 字节，文件头为 `%PDF-`。
7. `GET /api/v1/print-jobs/{id}/preview` 返回 HTTP 200，36,014 字节。
8. 验证后 taskkill 停止服务进程树，端口 17653 释放。

## 连续录屏主路径（15:41，对应 `windows-platform-queue-2026-08-21.mp4`）

录屏环境信息展示、platform 启动、health 探活、打印机枚举、气球任务提交、状态查询、PDF 预览、隔离队列输出文件、优雅停止（Ctrl+C 后 health 连接被拒）。录制时长约 4 分 13 秒，原始文件 553,956,196 字节。

录屏中的实际任务：

- 任务 ID：`97654d58a864b473735d097571ba6b15`（balloon_ticket，printer_name=`ISO-PDF-Queue`，队伍"星辰队"，题号 C）。
- 状态：`succeeded`，attempts=1，created 2026-08-21T07:41:42Z，finished 2026-08-21T07:41:44Z。
- 持久化记录：`data/jobs.json` 与 `data/jobs/97654d58a864b473735d097571ba6b15/preview.pdf`。
- 系统队列产出：print-iso 隔离目录下的 `iso-output.pdf` 289,600 字节（15:41:44），副本存于 `docs/reports/assets/windows-iso-output-2026-08-21.pdf`。
- 录屏文件 `docs/reports/assets/windows-platform-queue-2026-08-21.mp4` 因仓库体积限制不纳入版本控制（`.gitignore` 排除 `*.mp4`）；验收时在本机该路径直接播放，文件永久保存于本地。

## 措辞边界

- `platform succeeded` 与隔离输出文件共同证明 Windows 系统打印队列接受了任务并完成渲染落盘；这不等于物理出纸。
- 未连接任何实体打印机；未使用默认打印机；所有提交都指向已确认安全的 `ISO-PDF-Queue`。
- 本文件不含启动能力值（token）与个人用户目录路径。
