# 第 2 天报告定向精修实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将第 2 天日报改写为严格符合教师七段结构、组成与需求可逐项对应、选型理由完整的 800～2000 字正文。

**Architecture:** 只修改 `docs/reports/day-02.md`。以 Day 1 的六项必做和当前代码事实为来源，使用一张组成表、一张需求映射表和三组选型比较，使报告能够直接指导 Day 3 的接口与数据约定。

**Tech Stack:** Markdown 表格、现有 Go 项目事实、仓库文档契约测试。

## Global Constraints

- 七个二级标题与教师模板完全一致且顺序不变。
- 正文保持在 800～2000 个中文字符附近。
- 不修改其余八篇报告、代码、API 或项目行为。
- 不写仓库地址、录屏外链、密钥、账号、提交哈希或大段代码。
- 不把方案选择写成平台实测，不把 Fake 或交叉编译写成真实打印。

---

### Task 1: 重写并验证第 2 天报告

**Files:**
- Modify: `docs/reports/day-02.md`
- Test: `docs/path_contract_test.go`

**Interfaces:**
- Consumes: `docs/reports/day-01.md` 的六项必做、既有组件和技术选型事实。
- Produces: 可供 Day 3 定义任务字段、状态机、错误码和 Store/Worker 接口的组成与选型依据。

- [ ] **Step 1: 按七个固定标题重组正文**

将基本信息与今日目标分开；使用“组成部分”“与需求的对应”“技术选型”“今日完成与自检”“问题、计划与 AI 沟通”原名。

- [ ] **Step 2: 完成组成与需求映射**

组成表必须包含名称、职责、输入、输出/上下游、负责人；需求表逐项覆盖气球、源码、队列、状态/失败、本地 API/Web、双平台和交付材料，并说明无无人承担项。

- [ ] **Step 3: 完成三组选型比较**

分别比较本地服务、PDF 渲染和平台打印组织方式；每个方案均写优点、限制、选择或放弃理由。

- [ ] **Step 4: 完成自检与 AI 沟通**

区分已确定与未确定事项；写一个范围过大的无效沟通案例、一个带需求和验收约束的有效案例；明日计划落到任务对象、状态机、错误码和接口约定。

- [ ] **Step 5: 验证结构、篇幅与文档契约**

```powershell
$text = Get-Content -LiteralPath 'docs/reports/day-02.md' -Raw
[regex]::Matches($text, '(?m)^## (.+)$').Groups[1].Value
([regex]::Matches($text, '[\u4e00-\u9fff]')).Count
rg -n 'https?://|TBD|TODO|待填写|BEGIN.*PRIVATE|密码|密钥' docs/reports/day-02.md
go test ./docs -count=1
git diff --check -- docs/reports/day-02.md
```

预期：七标题名称和顺序完全一致；中文字符数在 800～2000；禁用内容无命中；文档测试和 Git 检查通过。

- [ ] **Step 6: 提交**

```powershell
git add -- docs/reports/day-02.md
git commit -m "docs: align day 2 report with course rubric"
```
