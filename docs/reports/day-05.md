# 第5天：可演示版本、预期现象与差距清单

## 本日结果

本地服务已由占位 PDF 切换为 Chromedp 真实渲染，固定使用 `github.com/chromedp/chromedp v0.13.2`。测试机上发现并验证 Google Chrome major `151`，高于源码页码契约要求的 `131`。气球小票为 1 页，长源码验收样例为 6 页。打印仍使用 Fake Printer，不会执行实体打印。

## 演示步骤与现象对照

| 步骤 | 操作 | 预期现象 | 实际现象 |
|---|---|---|---|
| 1. 启动 | 设置 `LOCAL_PRINT_AGENT_BROWSER_PATH` 后运行服务 | 发现 Chrome 131+，绑定第一个可用端口 | Chrome 151 通过版本检查；缺少浏览器时明确返回 `RENDERER_NOT_FOUND` |
| 2. 探活 | `GET /health` | HTTP 200，`service=local-print-agent`、`api_version=v1`、`status=ok` | 真实服务集成测试返回 200 |
| 3. 选打印机 | `GET /api/v1/printers` 并选择默认项 | 出现演示用 Fake Printer | 返回“Mock Printer（不执行实体打印）” |
| 4. 提交气球 | 提交 `balloon_ticket` JSON | HTTP 202，生成 32 位小写十六进制 jobID | 真实服务生成 `09a23714981d6043aa7d682574188db3` |
| 5. 状态变化 | 轮询任务详情 | `queued → rendering → printing → succeeded` | 端到端最终为 `succeeded`；状态顺序由 Worker 回归测试逐项断言 |
| 6. 气球预览 | 打开 `/api/v1/print-jobs/{id}/preview` | HTTP 200、`application/pdf`，窄纸单页 | HTTP 200，PDF 可解析且为 1 页；Range 请求返回 206 |
| 7. 提交源码 | 提交包含中文注释的长 C++ JSON | HTTP 202，源码被高亮并显示行号 | 真实服务生成 `7056cddfeef0d09741416758f5628b01` 并成功完成 |
| 8. 源码预览 | 打开源码 preview | A4 多页，每页有页眉、行号和“第 n / N 页” | 验收 PDF 可解析为 6 页；Chrome CDP 页眉/页脚位于显式上下边距内，第 1/2 页正文与页眉、页码均有可测空隙 |

## 真实 PDF 截图

以 Chrome 151 `PrintToPDF` 生成 PDF，再用本地 Poppler `pdftoppm` 按 150 DPI 导出 PNG；未调用 GLM 视觉。本实现代理已直接查看三张 PNG：源码第 1/2 页的页眉分隔线均在正文上方，页码均在正文下方，旧版页底覆盖 56–59、116–120 行的问题不再出现。

- 气球小票：![气球小票](assets/day-05-balloon.png)
- 源码首页（中文注释、行号、页眉、页码）：![源码首页](assets/day-05-source-page-1.png)
- 源码第二页（验证分页与连续行号）：![源码第二页](assets/day-05-source-page-2.png)

## 任务 JSON

- [气球小票完整 JSON](../../testdata/balloon.json)：包含比赛、队伍、房间、题号、气球颜色和 RFC3339 通过时间。
- [长 C++ 源码完整 JSON](../../testdata/source_cpp.json)：包含 140 行可见源码、中文注释和完整竞赛元数据，用于稳定触发多页。

API 提交时只提取两个文件的 `type`、`printer_name` 和 `payload`；`id` 仅是渲染固定样例，不得把包含 `id` 的整个 fixture 直接 POST 到严格 API。真实 API 会自行生成安全 jobID。

## 自动测试输出

真实 Chrome 双类型 PDF：

```text
$ LOCAL_PRINT_AGENT_CHROME_E2E=... go test -count=1 ./internal/render -run TestPDFRendererChromeIntegration -v
Chromium major: 151
source PDF pages: 6
source page 1 geometry: header rule y=62, body y=139, footer y=1710, height=1754
source page 2 geometry: header rule y=62, body y=127, footer y=1710, height=1754
--- PASS: TestPDFRendererChromeIntegration
```

真实服务、两任务、preview 与清理：

```text
$ LOCAL_PRINT_AGENT_CHROME_E2E=... go test -count=1 ./cmd/local-print-agent -run TestRealServiceRendersBothJobsServesPreviewAndCleansUp -v
service jobs: balloon=09a23714981d6043aa7d682574188db3 source=7056cddfeef0d09741416758f5628b01
--- PASS: TestRealServiceRendersBothJobsServesPreviewAndCleansUp
```

几何断言把真实 PDF 的第 1/2 页光栅化，定位页眉横线、正文首行和底部页码，要求页眉/正文与正文/页脚之间均有独立空隙。真实服务集成还断言：两任务均进入 `succeeded`；preview 为 HTTP 200/PDF；Range 为 206；取消上下文后服务优雅关闭；端口可重新绑定；新建 Chrome profile 全部清理。另有确定性生命周期测试在渲染进行中取消，验证 `running.Done` 必须等待 renderer 清理 profile 与 worker 退出。提交前的完整 `GOTOOLCHAIN=local go test -count=1 ./... -v` 通过（真实 Chrome/Poppler 用例为显式 opt-in，已单独运行）；`go test -race` 关键包、`go vet ./...`、`go mod verify` 和 `git diff --check` 均通过。

## 安全与失败行为

- 浏览器显式路径必须存在且为普通文件；自动发现顺序为环境变量、PATH、Windows/Linux 常见安装路径。
- Chrome 低于 131 或版本不可识别时返回 `RENDERER_VERSION_UNSUPPORTED`；对外消息不包含浏览器路径。
- jobID 只接受 Service 生成的 32 位小写十六进制形式。公开任务目录保持稳定；HTML/PDF 先写同目录临时文件并落盘，再用文件级原子替换发布，因此重试期间 Preview 始终读到完整旧版或新版 PDF。启动时会恢复旧实现崩溃遗留的 `.previous`。
- Chromedp/文件暂存等运行期诊断仅保留在 Worker 内部错误流；持久化任务/API 只返回稳定的 `PDF rendering failed`，不包含浏览器、profile、dataDir 或 staging 路径。
- preview 只从 Store 取 job 的 `PDFPath`，且必须精确对应 `<PreviewRoot>/<jobID>/preview.pdf`；路径清理、相对路径和 symlink/reparse 检查会阻止越界。
- PDF 未就绪返回 409 `PREVIEW_NOT_READY`，任务不存在返回 404，篡改或越界路径返回不泄露细节的 500。
- 本机 Windows 普通用户无创建 symlink 权限，symlink HTTP 测试如实标记为 skip；非symlink 的路径越界、错误文件名、store 篡改和 Range 测试均实际运行。

## 差距清单

1. Windows 真打印：尚未实现 spooler/系统打印命令适配。
2. Linux 真打印：尚未实现 CUPS/lp 适配。
3. 打印机枚举：当前只有 Fake Printer，未读取操作系统列表。
4. 失败场景：已覆盖渲染失败、打印命令失败和 preview 路径攻击，但尚缺真实打印机缺纸、离线、驱动错误等现场演练。
5. 重启恢复：持久化恢复和队列恢复已有单测，但未完成“渲染途中强制退出”的端到端演示。
6. 完整 README：尚缺安装 Chrome、环境变量、系统打印权限和故障排查的完整用户文档。

## 对 CCPCOJ 的借鉴

- 任务编号是排查、重试和现场口头沟通的统一索引，因此气球票和源码页眉都显示 jobID。
- 队伍、赛区/比赛、房间和题号是现场分发的核心元数据，在两类模板中保持一致。
- 手动优先：本页面先提供显式选打印机、预览和重试，不在本日引入 OJ 自动触发。
- 源码打印必须有稳定行号和页码，便于仲裁、队伍和现场工作人员对齐纸面位置。

## 对 Lodop 的借鉴

- 本地服务：网页只调用 loopback API，渲染和后续打印均在本机完成。
- 纸张设置：气球票使用 80×120mm CSS page，源码使用 A4。
- HTML/PDF：业务模板先生成自包含 HTML，再转为可存档、可预览的 PDF。
- 预览与直接打印：当前已完成安全 PDF 预览；直接打印保留在 Adapter 边界，下一日接操作系统。
- 状态：任务状态明确区分排队、渲染、提交打印、成功与失败，不把 HTTP 成功误当作物理出纸。

## 明日计划

1. 实现 Windows 打印适配器与打印机枚举。
2. 实现 Linux/CUPS 打印适配器与打印机枚举。
3. 分别对气球小票和源码执行真实或虚拟打印自测，记录系统接受结果与实际出纸边界。
4. 在两类业务模块中闭环验证“渲染 → preview → 直接打印 → 状态”。
