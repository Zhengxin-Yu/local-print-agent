# 第6天：两类打印模块完成说明与自测（Windows 部分草稿）

> 当前仅完成 Task 11 的 Windows 适配器代码与非破坏性验证。Linux/CUPS、主程序平台接线及双平台真实打印证据将在 Task 12 后补齐；本草稿不把 Fake Printer 或 PDF 预览写成系统打印成功。

## 气球小票模块

输入包括比赛名、队伍编号/名称、房间、题号、气球颜色和通过时间；模板为 80 mm × 120 mm 窄纸，动态文本经 HTML 转义。Windows 提交步骤应为：生成 `data/jobs/<jobID>/preview.pdf` → 从 `Win32_Printer` 选择枚举名称 → SumatraPDF 提交。当前仅完成到受控假命令验证，未进入真实系统队列。

## 源代码模块

支持 `cpp`、`go`、`python`、`java`，包含 Chroma 高亮、行号、A4 分页、页眉元数据和页码。Windows 提交步骤与气球模块共用同一 PDF/打印适配边界。现有 Task 10 证据证明源码预览可生成多页 PDF；这不等同于 Task 11 的系统打印成功。

## Windows 适配器命令与安全校验

- 枚举：固定 PowerShell `Get-CimInstance Win32_Printer | Select-Object Name,Default | ConvertTo-Json`，兼容单对象与数组。
- 提交：`SumatraPDF.exe -print-to <枚举名称> -silent <任务目录/preview.pdf>`，参数数组顺序由测试精确断言。
- 配置：`LOCAL_PRINT_AGENT_SUMATRA_PATH` 指定 SumatraPDF 普通文件。
- 超时：每次外部命令 30 秒。
- 打印机：请求名称必须精确匹配枚举结果，否则 `PRINTER_NOT_FOUND`。
- PDF：仅接受 `DataDir/jobs/<32位小写十六进制ID>/preview.pdf`，逐级拒绝链接和 Windows reparse point。
- 使用前复核：枚举完成后、SumatraPDF 启动前再次验证 PDF，避免枚举期间文件被删除或替换。
- 脱敏：稳定公开错误不包含可执行文件路径、PDF 路径、stdout 或 stderr。

## 状态时间线

- 气球小票：完整真实 Windows 时间线尚未产生；主程序仍使用 Fake Printer。既有自动主路径为 `queued → rendering → printing → succeeded`，只能证明 Worker/Fake 契约。
- 源代码：同上。Windows Adapter 的受控 runner 单测独立证明打印边界会先枚举、再以严格四参数提交，但不声称 OS 已接受任务。

## Windows 自测表

| 功能 | 输入 | 期望 | 实际 | 结论 | 证据 |
|---|---|---|---|---|---|
| 命令构造 | 枚举名称 + 合法 preview.pdf | 严格四参数 | 参数和顺序一致 | 通过 | `TestWindowsAdapterPrintUsesEnumeratedNameAndStrictSumatraArguments` |
| 枚举解析 | 单对象/数组 JSON | 保留 Name/Default | 与期望一致 | 通过 | `TestWindowsAdapterListsPowerShellJSONShapes` |
| 空列表 | `[]`、`null`、空输出 | `PRINTER_NOT_FOUND` | 稳定错误码 | 通过 | `TestWindowsAdapterReportsNoEnumeratedPrinters` |
| 未枚举名称 | 注入式字符串 | 命令执行前拒绝 | 仅发生枚举 | 通过 | `TestWindowsAdapterRejectsUnknownPrinterBeforePrintCommand` |
| PDF 边界 | 越界路径/错误文件名 | 外部命令前拒绝 | 无命令调用且不泄露路径 | 通过 | `TestWindowsAdapterRejectsAnythingExceptGeneratedJobPreview` |
| 命令失败 | stderr 含敏感路径 | `PRINT_COMMAND_FAILED` 且脱敏 | 公开错误无路径 | 通过 | `TestWindowsAdapterDoesNotLeakCommandDiagnostics` |
| 系统枚举 | 当前 Windows 账户 | 返回安装打印机 | CIM/Get-Printer 均 Access denied | 环境受限 | Task 11 报告的只读验证记录 |
| 虚拟/实体打印 | SumatraPDF + 自动保存虚拟队列 | OS 接受并留存证据 | SumatraPDF 缺失，未执行打印 | 未验证 | Task 11 报告的环境检查 |

## 未完成与明日计划

1. 在具有 CIM 读取权限的 Windows 环境安装/配置 SumatraPDF，并只对已确认自动保存的虚拟打印机补做气球小票队列与输出证据；不得默认向实体打印机提交。
2. Task 12 实现 Linux/CUPS 适配器并完成主程序平台接线。
3. 接线后分别记录气球和源码任务的真实完整状态时间线；若任一平台仍受环境限制，继续明确标为未验证。
