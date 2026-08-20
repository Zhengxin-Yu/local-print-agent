# Task 8 实现报告

## 完成内容

- 嵌入 `web/index.html`、`web/app.js`、`web/styles.css`，并通过 `web.Assets` 精确暴露三项资源。
- API 优先于网页路由；根页和两项静态文件返回正确 MIME，其他静态路径、目录和穿越请求均返回 404。
- Task 8 初版仅凭 `Origin: null` 开放 API 的 GET/POST/Content-Type CORS；最终安全审查改为默认同源页面，并要求可选 `file://` 页面携带本次启动的随机能力值。
- 页面具备中文连接状态、端口/版本、打印机选择、气球和源码表单、倒序任务表、详情、失败原因、失败重试和预览 501 错误提示。
- 生产装配按 Config → JSONStore → RecoverInterrupted → Fake renderer/printer → 私有 Pipeline → ResumeQueued → Router → listener → http.Server 顺序进行；Ctrl+C 使用 `signal.NotifyContext` 与有时限的 `Shutdown`。
- 恢复时排队任务按创建时间重新入队；超过固定容量 100 会返回明确错误并不部分投递。
- Fake renderer 只写受控目录内、带 `FAKE RENDERER` 标识的占位 PDF；Fake Printer 名称为“Mock Printer（不执行实体打印）”。

## 测试与冒烟

- `go test -count=1 ./... -v`：通过。
- 聚焦 TDD 红/绿验证：网页资源/CORS、Fake renderer、恢复入队、主进程健康和关闭均已执行。
- 实际启动后 `GET http://127.0.0.1:17653/health` 返回 `{service: local-print-agent, api_version: v1, status: ok}`；实际创建气球任务后状态进入 `succeeded`。
- Headless Chrome 已打开运行中页面并生成真实截图：`docs/reports/assets/day-04-console.png`。

## 交付物

- 第 4 天报告：`docs/reports/day-04.md`
- 截图：`docs/reports/assets/day-04-console.png`
- 启动脚本：`scripts/run-windows.ps1`、`scripts/run-linux.sh`（Linux 脚本已标记可执行）

## 已知边界

真实 HTML/PDF 渲染、预览文件流和真实系统打印均不在本任务范围内；预览 API 有意返回 501，网页显示错误而不假装成功。

## 修正轮次 1

- Fake PDF 已升级为最小合法单页 PDF：Catalog、Pages、Page、Helvetica、文本内容流、xref、trailer、Root、`startxref` 与 `%%EOF` 均由测试解析验证。写入采用临时文件、Sync、Close、Rename；失败会清理临时半写文件。
- 真实端到端创建源码任务后，本机 MiKTeX 安装提供的 `pdfinfo` 返回 0，报告 1 页、612×792 points、PDF 1.4；机器安装路径不作为项目证据路径记录。
- 网页错误状态按来源（refresh/create/retry/detail/preview/connection/printer）隔离：后台刷新成功只清除 refresh 自己的错误；refresh 失败展示错误并 rethrow，创建和重试不会把该失败当作成功清除。
- 本机 Chrome 能打开页面；用于自动点击预览 501 的 DevTools WebSocket 被环境策略以 HTTP 500 拒绝，因此未将该外部阻断当作产品行为结论。静态浏览器契约测试覆盖 preview/refresh 来源隔离和 rethrow。
