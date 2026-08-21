# 课题三日常报告（第 4 天）：HTTP API 与 Mock Web 工程起步

## 基本信息

- 课题：浏览器调用本地打印机（课题三）。
- 成员：独立完成。
- 日期：2026 年 8 月 15 日。

## 今日目标

把第 3 天的进程内流水线接到 HTTP：交付 7 个路由、统一响应 envelope 和嵌入式 Mock Web；同时形成一份他人可按文字跟做的启动说明，并对助手生成的初稿完成一次人工审查。

## 如何启动

依赖：Windows 或 Linux，Go 1.23 或更新版本。本日渲染与打印均为 Fake，无需安装浏览器或打印软件。

1. 安装 Go 后确认 `go version` 输出 1.23 以上。
2. 在项目根目录启动（Windows）：

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\scripts\run-windows.ps1
   ```

   Linux 为 `bash ./scripts/run-linux.sh`。
3. 终端输出监听地址（服务在 `127.0.0.1` 的 `17653-17660` 中绑定首个可用端口），浏览器打开该 URL。
4. 预期现象：页面自动确认 health 标识；打印机一栏显示“Mock Printer（不执行实体打印）”；两类任务表单可用；任务列表每 2 秒刷新。
5. 停止：在启动终端按 Ctrl+C，等待进程退出；再次启动可重新绑定端口。

运行数据保存在 `data/jobs.json`；删除 `data/` 目录即回到全新状态。

## 目录或工程结构

| 目录 | 职责 |
| --- | --- |
| `cmd/local-print-agent` | 装配：配置、实例锁、恢复、Worker、HTTP 启停 |
| `internal/httpapi` | 协议与 7 个路由、envelope、请求校验 |
| `internal/jobs` | Job 模型、状态机、Service（创建/重试） |
| `internal/store` | JSON 持久化与重启恢复 |
| `internal/worker` | 单 Worker 顺序消费 FIFO |
| `internal/render` | 本日为 Fake Renderer（Day 5 换真实 Chrome） |
| `internal/printer` | 本日为 Fake Printer（Day 6 换平台 Adapter） |
| `web` | 嵌入式页面，只调 API 和展示，不承担业务逻辑 |
| `scripts` | 双平台启动脚本 |
| `testdata` | 气球与源码固定样例 |

形成的纵向切片：启动脚本 -> 回环 HTTP -> Mock Web -> Job Service -> FIFO Worker -> Fake Renderer -> Fake Printer -> 状态回写。

## 审查纪要

审查人：本人。对象：当日由 AI 助手生成初稿、经本人修改后的 httpapi 路由、响应 envelope 和 web 页面。

- 目录与昨日约定一致性：路由错误码与第 3 天错误约定逐条核对（`INVALID_REQUEST`、`QUEUE_FULL`、`RETRY_NOT_ALLOWED` 等），一致。
- 配置与密钥：检查配置读取与脚本内容，无任何硬编码密钥或口令；监听固定为回环地址段。
- 发现问题一：初版页面把任务成功文案写成“打印成功”，会让演示证据越界。已修改为 `succeeded` 仅表示流水线执行完成，并在页面固定显示“不执行实体打印”。
- 发现问题二：preview 路由初稿对未实现场景直接返回 404，与“可诊断的未实现”不符。已改为显式 501 并列入明日计划。
- 发现问题三：本机浏览器自动化受 DevTools 策略阻断，当日 Web 行为只能由 Go/静态契约测试覆盖。如实记录，未伪装成浏览器 E2E；计划第 7 天以 Node VM 执行真实 `app.js` 补强。

三个问题当日均已修改并随代码提交。

## 协作与提交

独立完成。当日改动以“HTTP API 与 Mock Web 初版”提交至本人仓库主分支，提交前运行 `go test ./...` 确认全部通过。

## 今日完成与自检

| 前提 | 操作 | 预期结果 | 实际结果 |
| --- | --- | --- | --- |
| 默认模式启动 | 打开根 URL | 页面识别本服务，打印机明确标识不实体打印 | 通过，有页面截图 |
| 合法气球/源码 JSON | 创建并观察列表 | 202、唯一 ID、最终 `succeeded` | 通过 Fake 端到端测试 |
| 未知字段、错误类型、超大请求 | 调用创建接口 | 稳定 400/415/413，不创建任务 | 通过 API 测试 |
| 任务 `failed` | 点击重试 | 清错误重新排队；其他状态 409 | 通过 |
| 请求 preview | 打开任务预览 | 显式 501 | 与本日范围一致 |
| 第一个端口被占用 | 启动并探测 | 回退下一端口，页面同段发现 | 通过 |

自检：按“如何启动”一节在本机重新跟做一遍，可运行；审查发现的三个问题均已处理；Fake Renderer 与 Fake Printer 的边界在页面文案、日报和测试结论中口径一致。

真实运行页面：

![第 4 天 Mock 控制台](assets/day-04-console.png)

另用 Fake 源码任务产物实际运行 `pdfinfo`：退出码 0，报告 1 页、612 x 792 points、PDF 1.4——Fake Renderer 生成的最小 PDF 结构本身是有效的，为明日切换真实 Chrome 渲染提供了对照基线。

## 问题、计划与 AI 沟通

- 不成功的沟通一例：起初让助手“生成完整的前后端脚手架”，初稿夹带分页、审计日志等与主路径无关的目录和依赖。发现后限定提示为“只要 7 个主路径路由和统一 envelope，不要引入额外框架”，其余自行删减。
- 比较有效的沟通一例：与助手复核措辞时加入三层强制约束——Fake 只记录调用、PDF 预览只证明文件可读、系统队列和物理出纸必须另行取证。修正后页面文字、日报和测试结论使用同一口径。
- 明日安排：先补两类 HTML 模板与 Chromedp 真实渲染，再开放 preview 读取路由；用中文队名气球与 140 行中文注释源码完成 Chrome 实跑并保存截图。
