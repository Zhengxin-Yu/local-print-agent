# 九天课程报告精修实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变项目事实的前提下，将九份课程报告统一改写为范围清楚、验收可执行、问题闭环、次日计划具体的高质量提交材料。

**Architecture:** 以现有报告、截图、测试统计和提交历史为事实源；第 1 至第 8 天使用统一日报信息顺序，第 9 天采用结课报告结构并前置最终结论。修改分三批进行，每批完成后检查事实边界与 Markdown，再做跨报告一致性验证。

**Tech Stack:** Markdown、Mermaid、Go 项目现有文档测试、Git 文本检查。

## Global Constraints

- 不虚构 Windows/Linux 录屏、Windows 系统队列、Linux/CUPS runtime 或真人同学验收。
- `demo` 成功、平台命令成功和物理出纸必须保持为不同证据层级。
- 保留现有图片路径和能够复现的测试、命令及提交事实。
- AI 沟通只概括现有材料能够佐证的方法改进，不虚构逐字提示词。
- 不新增功能，不修改 API、代码、测试或项目运行行为。
- 没有统一字数限制，以互评可扫描性和信息完整性为准。

---

### Task 1: 精修第 1 至第 3 天

**Files:**
- Modify: `docs/reports/day-01.md`
- Modify: `docs/reports/day-02.md`
- Modify: `docs/reports/day-03.md`

**Interfaces:**
- Consumes: 设计说明、现有 Day 1-3 报告、既有端口/状态机/错误码事实。
- Produces: 范围基线、技术选型和主路径契约，供后续日报引用。

- [ ] **Step 1: 重写 Day 1 的范围与场景**

补足独立完成、竞赛现场用途、必做/不做边界，并将“两类打印”明确为气球小票与源代码经 HTML 渲染后输出 PDF。

- [ ] **Step 2: 将 Day 1 验收改写为前提-操作-结果**

覆盖 health/端口回退、服务未启动、两类业务主路径和打印失败；对尚未实现的项目标记“待后续核对”。

- [ ] **Step 3: 精修 Day 2 的方案比较与交付物**

保留架构、方案表、状态机和真实测试证据；新增范围、当日验收、AI 沟通与可检查的次日交付物。

- [ ] **Step 4: 精修 Day 3 的服务主路径与失败语义**

突出 Job/Store/Worker 接口、FIFO、恢复与 at-least-once；新增验收表、AI 沟通、自检和明确的 Day 4 交付物。

- [ ] **Step 5: 检查 Day 1-3 一致性**

运行：

```powershell
rg -n "17653|queued|rendering|printing|succeeded|failed|AI 沟通|明日计划" docs/reports/day-01.md docs/reports/day-02.md docs/reports/day-03.md
git diff --check -- docs/reports/day-01.md docs/reports/day-02.md docs/reports/day-03.md
```

预期：端口与五状态口径一致；三篇均有 AI 沟通和具体明日计划；无空白错误。

### Task 2: 精修第 4 至第 6 天

**Files:**
- Modify: `docs/reports/day-04.md`
- Modify: `docs/reports/day-05.md`
- Modify: `docs/reports/day-06.md`

**Interfaces:**
- Consumes: Day 1-3 范围与状态机基线、现有 Web/PDF/Adapter 事实和图片。
- Produces: 可操作 Mock Web、真实渲染证据及双平台适配证据分层。

- [ ] **Step 1: 精修 Day 4 的可操作闭环**

前置今日目标和范围；用验收表说明启动、探活、创建、状态、预览未实现及安全来源限制；明确 Fake 边界。

- [ ] **Step 2: 精修 Day 5 的真实渲染证据**

保留三张图片和真实 Chrome 结果，以前提-操作-预期-实际表为主；压缩重复路径安全说明，保留关键差距。

- [ ] **Step 3: 精修 Day 6 的平台证据分层**

明确代码、受控 runner、交叉编译、系统队列、物理输出五层证据；保留平台命令边界和未完成项。

- [ ] **Step 4: 补充 Day 4-6 的 AI 沟通与次日交付**

分别记录 Fake 不等于实打、版式几何验证、交叉编译不等于 Linux 实跑三个可追溯方法改进。

- [ ] **Step 5: 检查图片与边界**

运行：

```powershell
rg -n "assets/day-|Fake|受控|交叉编译|系统队列|物理出纸|AI 沟通|明日计划" docs/reports/day-04.md docs/reports/day-05.md docs/reports/day-06.md
git diff --check -- docs/reports/day-04.md docs/reports/day-05.md docs/reports/day-06.md
```

预期：四张既有截图引用仍存在；没有把受控或交叉编译证据升级为平台实测。

### Task 3: 精修第 7 至第 8 天

**Files:**
- Modify: `docs/reports/day-07.md`
- Modify: `docs/reports/day-08.md`

**Interfaces:**
- Consumes: 现有回归结果、缺陷记录、干净副本启动和人工缺口。
- Produces: 问题闭环报告与提交准备报告。

- [ ] **Step 1: 重组 Day 7 回归与问题闭环**

保留代表性成功、失败与环境阻断用例；将四个真实缺陷统一写成现象-根因-处理-复测；压缩目录式测试罗列。

- [ ] **Step 2: 重组 Day 8 可复现交付**

前置今日目标、完成范围与验收表；保留干净副本证据和自动验证；明确真人启动、双平台录屏和 Linux runtime 未完成。

- [ ] **Step 3: 补充 Day 7-8 AI 沟通**

Day 7 记录从静态字符串检查升级为 Node VM 行为验证；Day 8 说明代理只读审查不能替代真人验收。

- [ ] **Step 4: 检查完成/未完成结论**

运行：

```powershell
rg -n "通过|未完成|未做|未验证|仅编译|环境阻断|AI 沟通|明日计划" docs/reports/day-07.md docs/reports/day-08.md
git diff --check -- docs/reports/day-07.md docs/reports/day-08.md
```

预期：没有互相冲突的完成状态；Day 8 明确第 9 天只整理和核验材料。

### Task 4: 精修第 9 天结课报告

**Files:**
- Modify: `docs/reports/day-09-final.md`

**Interfaces:**
- Consumes: Day 1-8 精修后的范围、证据和遗留项。
- Produces: 可独立阅读的最终报告与九天索引。

- [ ] **Step 1: 前置最终范围与验收结论**

在摘要之后给出完成、部分完成、未完成的最终矩阵，并明确不同成功证据层级。

- [ ] **Step 2: 合并重复章节**

合并原范围矩阵、双平台证据表和完成情况对照中的重复信息；保留架构、核心实现、安全、测试、问题复盘和参考资料。

- [ ] **Step 3: 强化问题复盘与 AI 说明**

保留真实缺陷闭环；将 AI 辅助写成约束、复核和证据口径改进，不替代本人责任或人工验收。

- [ ] **Step 4: 更新索引与后续计划**

确认 Day 1-9 文件路径、图片路径、启动命令和未完成补验清单准确。

- [ ] **Step 5: 检查最终报告**

运行：

```powershell
rg -n "完成|部分完成|未完成|Fake|受控|交叉编译|物理出纸|AI" docs/reports/day-09-final.md
git diff --check -- docs/reports/day-09-final.md
```

预期：最终结论前置且无证据越界；Markdown 无空白错误。

### Task 5: 九篇报告统一验证

**Files:**
- Verify: `docs/reports/day-01.md` through `docs/reports/day-09-final.md`

**Interfaces:**
- Consumes: 全部精修报告。
- Produces: 一致、可提交的九天报告集。

- [ ] **Step 1: 检查必需章节和占位符**

```powershell
rg -L "AI 沟通" docs/reports/day-0[1-8].md
rg -n "TBD|TODO|待填写|占位符" docs/reports/day-*.md
```

预期：Day 1-8 都包含 AI 沟通；没有待填写占位符。

- [ ] **Step 2: 检查图片引用目标**

逐一解析 `docs/reports/*.md` 中的本地图片引用，确认文件位于 `docs/reports/assets/` 且存在。

- [ ] **Step 3: 检查事实一致性**

核对端口 `17653-17660`、五状态、队列容量 100、Windows 受控 runner、Linux 仅交叉编译、录屏/真人验收未完成等关键事实。

- [ ] **Step 4: 运行文档与项目验证**

```powershell
$env:GOTOOLCHAIN = 'local'
$cacheDir = New-Item -ItemType Directory -Force '.cache\report-refinement-verify'
$env:GOCACHE = $cacheDir.FullName
go test ./docs -count=1
go test ./... -count=1
git diff --check
```

预期：文档路径契约与完整测试通过，Git 无空白错误。

- [ ] **Step 5: 检查最终差异**

阅读 `git diff --stat` 和九篇报告的标题/小节索引，确认只修改约定文档且没有删除图片或代码。
