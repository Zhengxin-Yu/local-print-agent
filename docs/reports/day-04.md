# 课题三日常报告（第 4 天）：HTTP API 与 Mock Web 闭环

## 基本信息与今日目标

- 完成方式：独立完成。
- 今日范围：接通 HTTP API、嵌入式页面和 Day 3 的任务流水线；渲染和打印仍使用明确标识的 Fake。
- 今日目标：让使用者能从浏览器完成探活、创建两类任务、查看状态和重试，为真实 PDF 渲染提供稳定上层入口。

## 今日交付

项目已形成可运行的纵向切片：

```text
启动脚本 -> 回环 HTTP 服务 -> 嵌入式 Mock Web -> Job Service
-> FIFO Worker -> Fake Renderer -> Fake Printer -> 状态回写
```

目录职责也已固定：`internal/httpapi` 负责协议和路由，`internal/jobs` 负责业务状态，`internal/store` 负责 JSON 持久化，`internal/worker` 负责顺序处理，`web` 只负责调用 API 和展示结果。网页不承担队列、渲染或打印逻辑。

## API 契约

| 方法 | 路径 | 作用 | 成功 | 主要失败 |
| --- | --- | --- | --- | --- |
| GET | `/health` | 探活与服务识别 | 200 | 405 |
| GET | `/api/v1/printers` | 枚举当前 Adapter 打印机 | 200 | 500、503 |
| POST | `/api/v1/print-jobs` | 创建气球或源码任务 | 202 | 400、413、415、500、503 |
| GET | `/api/v1/print-jobs` | 查询队列全貌 | 200 | 500、503 |
| GET | `/api/v1/print-jobs/{id}` | 查询任务详情 | 200 | 404、500、503 |
| POST | `/api/v1/print-jobs/{id}/retry` | 重试失败任务 | 200 | 404、409、500、503 |
| GET | `/api/v1/print-jobs/{id}/preview` | 读取生成 PDF | 本日未实现 | 501 |

所有 JSON 响应使用统一 envelope；创建接口要求 `application/json`、限制 1 MiB 并拒绝未知字段。重试只允许 `failed`，不能把成功任务重复排队。

## 启动与页面

Windows：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-windows.ps1
```

Linux：

```bash
bash ./scripts/run-linux.sh
```

服务会在 `17653-17660` 中绑定第一个可用端口并输出 URL。打开该 URL 后，页面自动确认 health 标识、显示 “Mock Printer（不执行实体打印）”、提供两类表单并每两秒刷新任务列表。

真实运行页面如下：

![第 4 天 Mock 控制台](assets/day-04-console.png)

## 今日验收

| 前提 | 操作 | 预期结果 | 实际结果与结论 |
| --- | --- | --- | --- |
| 服务以默认模式启动 | 打开根 URL | 页面加载并识别本服务，打印机明确标识不实体打印 | 通过，见页面截图 |
| 输入合法气球或源码 JSON | 点击创建并观察任务列表 | HTTP 202；生成唯一 ID；最终显示 `succeeded` | 通过 Fake 端到端测试 |
| 输入未知字段、错误类型或超大请求 | 调用创建接口 | 分别返回稳定 400/415/413，不创建任务 | 通过 API 测试 |
| 任务处于 `failed` | 点击重试 | 清理错误后重新排队；其他状态返回 409 | 通过 Service/API 测试 |
| 请求 preview | 打开任务预览 | 因真实 Renderer 尚未接入，明确返回 501 | 与本日范围一致 |
| 第一个端口被占用 | 启动并由页面探测 | 服务回退下一端口，页面按同一候选段发现 | 通过监听与 Web 契约测试 |

## Fake 边界与安全审查

Fake Renderer 生成可被 `pdfinfo` 识别的最小单页 PDF，并在内容中标明 `FAKE RENDERER` 和任务 ID；Fake Printer 只记录调用。页面显示 `succeeded` 仅说明这条演示流水线执行完成，不说明 Chrome 渲染、系统队列或物理出纸已经通过。

安全审查确认：

- 主进程只监听 `127.0.0.1`，页面默认与服务同源。
- 可选 `file://` 页面必须携带本次启动生成的随机能力值；普通网页 Origin 不获授权。
- 动态内容使用安全文本节点，不把任务错误或源码写入 `innerHTML`。
- 启动脚本不拼接用户命令，Worker 也不记录完整源码或底层错误文本。
- 队列恢复只补入 `queued`，超过容量时明确失败且不部分投递。

本日还用实际 Fake 源码任务运行 `pdfinfo`，返回 0，并报告 1 页、612 x 792 points、PDF 1.4。浏览器自动化受本机 DevTools 策略阻断，因此当日 Web 行为只由 Go/静态契约覆盖，未伪装成浏览器 E2E。

## 问题、AI 沟通与自检

早期表述容易把“页面任务成功”写成“打印成功”。与 AI 复核时加入了三层强制约束：Fake 只记录调用、PDF 预览只证明文件可读、系统队列和物理出纸必须另取证。修正后页面文字、日报和测试结论使用同一口径，避免演示效果超过实际证据。

自检结果：7 个路由、统一错误、Mock Web、状态刷新和失败重试已形成可操作闭环；preview、真实版式和平台打印仍明确未实现。

## 明日计划

交付两类 HTML 模板、Chroma 源码高亮、Chromedp PDF Renderer 和可读取的 preview 路由；使用含中文队名的气球样例与 140 行中文注释源码完成真实 Chrome 实跑，保存气球、源码首页和第二页三张截图。
