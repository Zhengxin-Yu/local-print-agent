# 课题三日常报告（第 6 天）：双平台打印适配与证据分层

## 基本信息与今日目标

- 完成方式：独立完成。
- 今日范围：实现并接通 Windows/Linux 平台 Adapter；不在未确认安全目标的环境中试投实体或交互式虚拟打印机。
- 今日目标：让两类 PDF 能进入统一平台接口，同时准确区分代码完成、受控测试、系统队列接受和物理出纸。

## 五层证据口径

| 层级 | 能证明什么 | 当前状态 |
| --- | --- | --- |
| 1. 代码与接口 | 两个平台 Adapter 已实现并接入同一 Worker | 完成 |
| 2. 受控命令测试 | 枚举、参数、allowlist、超时和错误映射符合契约 | Windows 已运行；Linux 测试代码已编译 |
| 3. 平台 runtime | 代码在对应操作系统内核与工具链上实际运行 | Windows 单元/受控 runner 已运行；Linux 未运行 |
| 4. 系统队列接受 | SumatraPDF/CUPS 将任务交给已确认的队列 | Windows、Linux 均未验证 |
| 5. 物理或隔离文件输出 | 设备出纸或自动保存目标产生文件 | 未验证 |

后续报告不得用较低层证据替代较高层结论。`succeeded` 在 demo 模式只表示 Fake 接受调用，在 platform 模式只表示平台命令成功返回，二者都不自动等于物理出纸。

## 两类打印模块

气球任务经过 API 校验和 FIFO 后，生成 `data/jobs/<jobID>/preview.pdf`，版式为 80 x 120 mm 单页；源码任务支持四种语言，生成带高亮、连续行号、换行、页眉和页码的 A4 多页 PDF。Worker 对两类任务使用完全相同的 Adapter：

```text
PDF 就绪 -> printing -> 重新枚举并精确匹配打印机
-> 参数数组提交 -> 命令成功则 succeeded，失败则 failed
```

Day 5 已证明气球为 1 页、140 行源码为 6 页。今日工作不改变渲染结果，只把固定 `preview.pdf` 接到平台边界。

## 模式选择

- 未设置 `LOCAL_PRINT_AGENT_PRINTER_MODE` 或值为 `demo`：使用“Mock Printer（不执行实体打印）”，保证默认演示安全。
- 值为 `platform`：显式构造当前系统 Adapter，依赖缺失时启动失败，绝不自动降级为 Fake。
- Windows platform 还要求 `LOCAL_PRINT_AGENT_SUMATRA_PATH`；Linux platform 要求 `lp` 和 `lpstat`。
- `/api/v1/printers` 与 Worker 共用同一个 Adapter，页面看到的打印机就是实际提交目标。
- 未知 mode 在启动时拒绝，防止含糊回退。

## Windows Adapter

打印机枚举固定使用 PowerShell/CIM 结构化 JSON，兼容单对象和数组。提交参数严格为：

```text
SumatraPDF.exe -print-to <本轮枚举的精确名称> -silent <dataDir>/jobs/<jobID>/preview.pdf
```

关键约束：

- 参数以数组传入，不经 shell 拼接；打印机名必须在本轮枚举 allowlist 中。
- 每个外部命令有 30 秒超时，公开错误只保留稳定 code/message。
- PDF 必须是固定任务目录下的普通 `preview.pdf`，拒绝越界、错误名、symlink、junction 或 reparse point。
- SumatraPDF 使用完整路径，并检查文件身份，避免枚举后可执行文件或父目录被替换。

本机使用受控 runner 记录 PowerShell/Sumatra 参数，没有启动真实 Sumatra，也没有访问系统队列。当前未配置 SumatraPDF，且普通账户读取 `Win32_Printer` 曾返回 Access denied，因此不能声称 Windows 队列已接受。

## Linux Adapter

Linux 构造阶段查找 `lp` 与 `lpstat`。枚举先执行 `lpstat -p`，再执行 `lpstat -d`；提交参数严格为：

```text
lp -d <本轮枚举的精确队列名> <dataDir>/jobs/<jobID>/preview.pdf
```

关键约束与 Windows 一致：不经 shell、名称必须来自本轮枚举、命令 30 秒超时、固定 `LC_ALL=C`、路径逐级拒绝 symlink、底层诊断脱敏。

Linux build-tag 测试包含固定 fixture 和受控 runner，覆盖解析、参数、超时、allowlist、缺命令和路径边界。但当前 Windows 主机没有可用 WSL 发行版，Docker daemon 也未运行；今日只完成 linux/amd64 测试二进制交叉编译，不能声称测试在 Linux 内核执行，更没有 CUPS request id。

## 今日验收

| 前提 | 操作 | 预期结果 | 实际结果与结论 |
| --- | --- | --- | --- |
| 未设置 mode | 启动服务并查询打印机 | 只出现明确的 Mock Printer，不构造平台 Adapter | 通过 |
| 显式 platform + 受控 Adapter | 查询 API 并提交任务 | API/Worker 使用同一平台列表和目标 | 通过装配测试 |
| Windows 合法枚举名和 PDF | 调用 Print | 参数顺序、deadline 和路径均固定 | 受控 runner 通过 |
| Windows 未知或注入式名称 | 调用 Print | 在 Sumatra 前返回 `PRINTER_NOT_FOUND` | 通过 |
| Linux 固定 `lpstat` fixture | 枚举并构造 `lp` 参数 | 默认队列解析和参数符合契约 | 测试代码交叉编译，未运行 |
| 平台依赖缺失 | 以 platform 启动 | 返回 `PRINT_COMMAND_FAILED`，不回退 Fake | 受控失败测试通过 |
| 可控 Windows/Linux 系统队列 | 实际提交并检查队列记录 | 获得系统接受证据和隔离输出 | 两平台均未验证 |

如果 Adapter 返回错误，Worker 执行 `printing -> failed`，保留 `PRINTER_NOT_FOUND` 或 `PRINT_COMMAND_FAILED`，可由失败任务重试接口重新排队。

## 问题、AI 沟通与自检

本日最容易出现的证据越界，是把“Linux 测试二进制交叉编译成功”概括为“Linux 测试通过”。与 AI 沟通时明确要求逐项回答“在哪个内核运行、是否调用真实工具、是否得到队列作业号”。据此将结论拆成五层，并把 Linux 标为“已编译、未运行”，Windows 标为“受控 runner 已运行、系统队列未验证”。

自检结果：两类业务 PDF 已共用统一 Adapter；默认 demo 安全，platform 不含糊降级；平台错误可进入任务失败状态。系统队列、物理输出和 Linux runtime 仍是明确缺口。

## 明日计划

交付跨模块回归表和真实问题闭环：覆盖端口冲突、队列满、并发创建、FIFO、请求超限、Chrome/Sumatra 缺失、打印机不存在、重启恢复、源码字节保持和 Web 行为；同时尝试补跑真实 Chrome 与 Linux/CUPS，若环境阻断则保留原始失败和精确缺口。
