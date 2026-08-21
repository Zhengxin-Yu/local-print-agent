# 课题三日常报告（第 6 天）：双平台打印适配（两块独立工作）

## 基本信息

- 课题：浏览器调用本地打印机（课题三）。
- 成员：独立完成。
- 日期：2026 年 8 月 17 日。

## 今日目标

从第 5 天差距清单中选取两块可独立说明、可自测的工作：**块一，Windows SumatraPDF Adapter 与打印机枚举**（对应清单第 1 条）；**块二，Linux CUPS Adapter 与 demo/platform 模式装配**（对应清单第 2、3 条）。系统队列证据（清单第 4 条）不在今日范围，环境具备时另行补验。

## 今日完成的两块

### 块一：Windows SumatraPDF Adapter 与打印机枚举

- 工作内容与对应差距：实现 Windows 平台的打印机枚举与 PDF 提交，替换第 5 天遗留的“无真实平台接口”缺口。
- 改动范围：`internal/printer/windows.go`（新增）及 printer 装配；`/api/v1/printers` 与 Worker 共用同一 Adapter，页面看到的打印机即实际提交目标。
- 约定：枚举用 PowerShell/CIM 结构化 JSON（兼容单对象与数组）；提交参数严格为 `SumatraPDF.exe -print-to <本轮枚举的精确名称> -silent <dataDir>/jobs/<jobID>/preview.pdf`；参数走数组不经 shell；打印机名必须在本轮枚举 allowlist 中；命令 30 秒超时；PDF 必须是固定任务目录下的普通文件，拒绝越界、symlink、junction 和 reparse point；SumatraPDF 用完整路径并校验文件身份，防止枚举后被替换。
- 自测方法与结果：用受控 runner 记录命令参数逐项断言——合法名称时参数顺序与 deadline 固定（通过）；未知或注入式名称在调用 Sumatra 前返回 `PRINTER_NOT_FOUND`（通过）；超时、路径与身份校验、错误脱敏（通过）。真实 Sumatra 与系统队列未运行：本机未配置 SumatraPDF，且普通账户读取 `Win32_Printer` 曾返回 Access denied，如实标注。

### 块二：Linux CUPS Adapter 与 demo/platform 模式装配

- 工作内容与对应差距：实现 Linux 队列枚举与提交；交付显式模式选择，保证默认演示安全（对应清单第 2、3 条）。
- 改动范围：`internal/printer/linux.go`（build tag）、主程序装配、双平台启动脚本的 mode 参数。
- 约定：枚举先 `lpstat -p` 再 `lpstat -d`；提交参数严格为 `lp -d <本轮枚举的精确队列名> <preview.pdf>`；固定 `LC_ALL=C`；allowlist、数组参数、30 秒超时、逐级 symlink 检查与诊断脱敏和 Windows 一致。模式选择：未设置或 `demo` 用“Mock Printer（不执行实体打印）”；`platform` 显式构造平台 Adapter，依赖缺失时启动失败，绝不自动降级为 Fake；未知 mode 启动时拒绝。
- 自测方法与结果：Linux build-tag 测试（固定 fixture 与受控 runner，覆盖解析、参数、超时、allowlist、缺命令、路径边界）完成 linux/amd64 交叉编译（`go test -c` 成功）；模式装配测试验证 demo 默认 Mock、platform 缺依赖返回 `PRINT_COMMAND_FAILED` 不回退（通过）。当前 Windows 主机无可用 WSL 发行版、Docker 未运行，因此**不能声称测试在 Linux 内核执行**，更没有 CUPS request id。

## 交叉阅读或互查

独立完成，自我复查按第 6 天建立的五层证据口径逐层核对：第 1 层（代码与接口）两平台完成；第 2 层（受控命令测试）Windows 已运行、Linux 仅编译；第 3 层（平台 runtime）Linux 未运行；第 4、5 层（系统队列接受、物理或隔离输出）两平台均未验证。复查重点是不把低层证据写成高层结论，例如“交叉编译成功”不得写成“Linux 测试通过”。

## 今日完成与自检

| 前提 | 操作 | 预期结果 | 实际结果 |
| --- | --- | --- | --- |
| 未设置 mode | 启动并查询打印机 | 只出现 Mock Printer | 通过 |
| 显式 platform + 受控 Adapter | 查询并提交任务 | API/Worker 使用同一列表 | 通过装配测试 |
| Windows 合法枚举名和 PDF | 调用 Print | 参数、deadline、路径固定 | 受控 runner 通过 |
| Windows 注入式名称 | 调用 Print | `PRINTER_NOT_FOUND`，不执行命令 | 通过 |
| Linux 固定 fixture | 枚举并构造 `lp` 参数 | 契约符合 | 测试代码交叉编译，未运行 |
| 平台依赖缺失 | platform 启动 | 稳定失败，不回退 Fake | 通过 |

自检：两块均按上述方法自测并有结果；系统队列、物理输出、Linux runtime 仍在差距清单，未混入本日完成项。

## 问题、计划与 AI 沟通

- 不成功的沟通一例：讨论验收口径时，助手一度把“Linux 测试二进制交叉编译成功”概括为“Linux 测试通过”。现象是结论直接越层；调整方式是要求逐项回答“在哪个内核运行、是否调用真实工具、是否得到队列作业号”，据此拆出五层口径。
- 比较有效的沟通一例：限定“每个平台只写枚举命令、提交命令和三条硬约束（allowlist、数组参数、超时），不展开无关实现”后，一次得到可对照实现的契约描述。
- 明日安排：跨模块联调——把 Day 4-6 的部件串成全链路回归，覆盖端口冲突、队列满、并发、恢复、失败注入和 Web 行为。
