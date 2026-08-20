# 课题三日常报告（第 5 天）：真实 PDF 渲染与版式验收

## 基本信息与今日目标

- 完成方式：独立完成。
- 今日范围：将 Fake Renderer 替换为真实 HTML -> PDF 流程并开放安全预览；Printer 仍保持 Fake。
- 今日目标：用真实 Chrome 产物证明气球窄纸版式、源码高亮、多页行号和页眉页脚正确。

## 今日完成

- 固定使用 `github.com/chromedp/chromedp v0.13.2` 驱动 Chrome/Chromium。
- 气球模板输出 80 x 120 mm 单页 PDF，包含竞赛、队伍、房间、题号、颜色和通过时间。
- 源码模板支持 `cpp`、`go`、`python`、`java`，使用 Chroma 高亮并保留缩进、行号和长行换行。
- 源码使用 A4、多页页眉和“第 n / N 页”页码。
- preview 路由返回 `application/pdf`，支持 HTTP Range，并限制为任务固定目录下的 `preview.pdf`。
- 测试机自动发现并验证 Google Chrome major 151，高于要求的最低 major 131。

## 前提、操作与结果

| 前提 | 操作 | 预期结果 | 实际结果与结论 |
| --- | --- | --- | --- |
| Chrome 151 可用 | 启动真实 Renderer | 版本检查通过；缺失或过低版本返回稳定错误 | 通过 |
| 提交含中文队名的气球 JSON | 轮询至完成并打开 preview | HTTP 202；状态合法流转；窄纸 PDF 为 1 页，中文和题号可见 | 通过 |
| 提交 140 行、含中文注释的 C++ | 打开 preview 并解析页数 | 高亮、连续行号、长行换行、页眉页码；稳定产生多页 | 通过，实际 6 页 |
| 请求完整或分段 preview | 执行普通 GET 与 Range GET | 分别返回 200 和 206，内容均来自同一安全 PDF | 通过 |
| 浏览器缺失或版本不支持 | 构造 Renderer | 返回 `RENDERER_NOT_FOUND` 或 `RENDERER_VERSION_UNSUPPORTED`，不泄露路径 | 通过失败测试 |
| PDF 尚未生成或路径被篡改 | 请求 preview | 返回 409 或脱敏 500，不能读取任务目录外文件 | 通过安全测试 |

真实服务集成生成的任务 ID 为：气球 `09a23714981d6043aa7d682574188db3`，源码 `7056cddfeef0d09741416758f5628b01`。两任务最终均为 `succeeded`，但此处仍由 Fake Printer 接受调用，不是系统打印证据。

## 真实 PDF 截图

以下 PDF 由 Chrome 151 的 `PrintToPDF` 生成，再用本地 Poppler 按 150 DPI 转为 PNG。三张图直接来自真实产物：

气球小票：

![气球小票](assets/day-05-balloon.png)

源码首页，包含中文注释、行号、页眉和页码：

![源码首页](assets/day-05-source-page-1.png)

源码第二页，用于检查分页后的连续行号和正文间距：

![源码第二页](assets/day-05-source-page-2.png)

几何检查定位页眉横线、正文首行和底部页码。源码第 1 页测得横线 y=62、正文 y=139、页码 y=1710；第 2 页正文 y=127，页面高度均为 1754。页眉、正文和页脚之间有独立空隙，旧版页底覆盖正文的问题已消除。

## 验收数据与自动证据

- `testdata/balloon.json`：包含完整竞赛和队伍字段，强制验证中文与窄纸。
- `testdata/source_cpp.json`：包含 140 行可见源码、中文注释和完整元数据，稳定触发多页。
- API 只接收 `type`、`printer_name` 和 `payload`；任务 ID 必须由服务自行生成。

```text
TestPDFRendererChromeIntegration
Chromium major: 151
source PDF pages: 6
source page 1 geometry: header rule y=62, body y=139, footer y=1710, height=1754
source page 2 geometry: header rule y=62, body y=127, footer y=1710, height=1754
PASS
```

真实服务 E2E 还验证了两类任务、200/206 preview、服务优雅关闭、端口重新绑定和临时 Chrome profile 清理。

## 安全和失败边界

- 浏览器路径必须存在且为普通文件；自动发现按环境变量、PATH 和平台常见位置进行。
- job ID 只接受服务生成的 32 位小写十六进制值。
- HTML/PDF 先写同目录临时文件，落盘后原子替换；重试期间只能读到完整旧版或新版 PDF。
- preview 必须精确对应 `<dataDir>/jobs/<jobID>/preview.pdf`，拒绝越界、错误文件名、symlink 和 Windows reparse point。
- 对外只返回稳定错误码和消息，不泄露 Chrome、profile、数据目录或暂存文件的绝对路径。

本机普通账户不能创建 symlink，相关 HTTP 用例如实标为 skip；非 symlink 的越界、Range、错误文件名和 Store 篡改测试均实际运行。

## 问题、AI 沟通与自检

早期源码页脚使用页面内固定元素，在多页时覆盖底部正文。只让 AI 检查 CSS 字符串不能证明最终 PDF 正确，因此沟通约束改为“必须基于真实 Chrome 产物，测量页眉、正文和页脚的像素位置”。最终改用 Chrome CDP 页眉/页脚和显式页边距，并用几何断言复测，三张截图作为可读证据。

自检结论：两类真实 PDF、中文、高亮、行号、长行、多页、preview 和失败提示已验证；打印机枚举、Windows/Linux Adapter 和系统队列仍未完成。

## 明日计划

交付 Windows SumatraPDF 与 Linux CUPS Adapter、打印机枚举、显式 `demo/platform` 模式和稳定平台错误；分别记录 Windows 受控命令测试、Linux build-tag 测试状态，以及系统队列仍需补验的精确缺口。
