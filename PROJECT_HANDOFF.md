# local-print-agent 开发进度与 Code Buddy 交接说明

> 更新日期：2026-08-21
> 用途：让新的开发者在不了解历史对话的情况下，继续完成平台补验、课程证据整理和必要修订。

## 1. 项目目标与当前结论

本项目是一个用 Go 编写的跨平台本地打印代理。浏览器通过回环 HTTP 服务提交竞赛气球小票或源代码任务；服务负责校验、持久化、FIFO 排队、HTML/PDF 渲染、平台打印命令调用和状态回传。

当前结论分两层：

- **代码与默认安全演示主路径已完成。** 两类任务、PDF、7 个 HTTP 接口、嵌入式 Web、JSON 持久化、单 Worker FIFO、失败原因、失败重试、Windows/Linux Adapter 和启动文档均已存在。
- **Windows platform runtime 已于 2026-08-21 补验完成**（原 P0-A）：隔离安全队列 `ISO-PDF-Queue` 上真实调用 SumatraPDF 3.6.1，系统队列接受并落盘隔离 PDF，连续录屏与结构化证据见 `docs/reports/assets/windows-platform-evidence-2026-08-21.md`。
- **课程完整人工验收仍是部分完成。** 尚缺 Linux/CUPS runtime 实跑、Linux 连续录屏，以及真人只读 README 的冷启动测试。

接手后应优先补齐证据，不要先扩展范围。默认 `demo` 模式不会进入系统打印队列；只有显式 `platform` 模式才可能提交平台打印命令。

## 2. 已完成模块与代码位置

| 模块 | 已实现职责 | 主要位置 |
| --- | --- | --- |
| 程序装配与生命周期 | 配置、实例锁、恢复、Worker、HTTP 启停和排空 | `cmd/local-print-agent/main.go` |
| 配置与监听 | 固定回环监听；在 `17653–17660` 选择首个可用端口 | `internal/config/`、`internal/server/` |
| 任务模型与队列 | 两类任务、校验、32 位小写十六进制 ID、容量 100 的 FIFO、失败重试 | `internal/jobs/` |
| 持久化与恢复 | `jobs.json` 原子写入；启动恢复中断任务 | `internal/store/` |
| Worker | 单 Worker 顺序执行渲染、打印和状态回写；存储失败退避 | `internal/worker/` |
| PDF 渲染 | 气球窄纸模板；Chroma 源码高亮；Chrome/Chromium PDF；安全发布 | `internal/render/`、`templates/` |
| 打印适配 | Demo Fake、Windows SumatraPDF、Linux CUPS | `internal/printer/` |
| HTTP 与 Web | 探活、打印机、任务创建/列表/详情/预览/重试；嵌入式控制台 | `internal/httpapi/`、`web/` |
| 单实例与路径安全 | 同一 `data/` 目录只允许一个进程；拒绝越界和链接逃逸 | `internal/instance/`、`internal/render/` |
| 测试和课程材料 | 自动测试、API、演示步骤、九天日报和 PDF 截图 | `docs/`、`testdata/` |

详细接口以 `docs/api.md` 为准；运行与依赖以 `README.md` 为准；证据层级和真实环境步骤以 `docs/testing.md` 为准。

## 3. 架构、状态机与不可破坏约束

```text
Web -> HTTP API -> Job Service -> JSON Store
                         |
                         v
                  FIFO 100 / 单 Worker
                         |
              HTML + Chroma + Chrome PDF
                         |
               demo Fake / platform Adapter
                         |
             Windows SumatraPDF / Linux CUPS
```

状态主路径：

```text
queued -> rendering -> printing -> succeeded
                    \-> failed
failed --retry--> queued
```

后续修改必须保持以下约束，除非用户明确批准架构变更：

1. 服务只监听 `127.0.0.1`，端口段固定为 `17653–17660`。
2. 队列容量为 100，只有一个 Worker，按 FIFO 处理。
3. 任务 ID 是 32 位小写十六进制；客户端不能提供文件路径。
4. PDF 预览固定为 `jobs/<jobID>/preview.pdf`，不得允许目录穿越、symlink 或 Windows reparse point 逃逸。
5. 外部命令使用参数数组，不拼接 shell 命令；打印机名必须来自本轮枚举 allowlist。
6. 默认模式必须是 `demo`；`platform` 失败时不能静默退回 Fake。
7. API 错误和日志不得暴露绝对路径、完整源码、外部命令输出或启动能力值。
8. 同一 `data/` 目录必须由实例锁保护，不能靠改端口绕过。
9. 平台命令提交存在 at-least-once 崩溃窗口：系统接受命令后、成功状态落盘前崩溃，人工重试可能重复提交。未做产品级设计前不得宣称 exactly-once。

## 4. 当前测试与证据边界

### 4.1 第 9 天报告记录的自动验证

`docs/reports/day-09-final.md` 记录：152 个顶层测试中 146 通过、6 跳过、0 失败；12 个含测试包通过；race 未报告竞争；vet、module verify 和 diff 检查通过。

这是第 9 天报告保存的历史结果。接手者应在当前 HEAD 重新运行第 8 节命令，不能仅引用该数字作为之后修改的验证结果。跳过项主要涉及真实 Chrome/服务 E2E、外部 `pdfinfo` 和当前账户无权创建 symlink 的场景，跳过不能写成通过。

生成本交接文档时于 2026-08-21 重新运行了 `go test ./docs -count=1` 和 `go test ./... -count=1`，两条命令退出码均为 0，12 个含测试包显示 `ok`。本次未用详细输出重新统计测试与 skip 数量，也未运行真实平台队列，因此不能用这次普通回归替代第 4.2 节的高层证据。

### 4.2 证据分层

| 层级 | 含义 | 当前情况 |
| --- | --- | --- |
| 1. 代码存在 | 实现和测试文件可定位 | 已有 |
| 2. 受控命令测试 | Fake 或受控 runner 验证参数、错误和状态 | 已有 |
| 3. 对应平台 runtime | 在真实 Windows/Linux 环境运行目标 Adapter | Windows 已完成（2026-08-21）；Linux 未完成 |
| 4. 系统队列接受 | 系统队列产生可审计作业记录或 request id | Windows 已完成（隔离队列落盘证据）；Linux 未完成 |
| 5. 物理或隔离输出 | 实体出纸或安全虚拟目标生成隔离文件 | Windows 隔离输出已验证；物理出纸未验证且不应在安全队列环境宣称 |

必须保持以下事实：

- Windows Adapter 的受控 runner 已通过，但这不等于真实 SumatraPDF 或 Windows 系统队列已运行。
- Linux Adapter 已通过代码测试和交叉编译，但这不等于在 Linux 内核调用过 CUPS。
- Day 5 保存了 Chrome 151 生成 PDF 的真实截图：`docs/reports/assets/day-05-balloon.png`、`docs/reports/assets/day-05-source-page-1.png`、`docs/reports/assets/day-05-source-page-2.png`。
- Day 7 在受限环境复跑 Chrome E2E 时出现 `context canceled`；该失败记录必须保留，不能被单元测试覆盖。
- 当前没有 Windows/Linux 系统队列连续录屏，也没有“真人只看 README”验收记录。
- `demo succeeded` 只表示 Fake Adapter 接受调用；`platform succeeded` 只表示平台命令成功返回；二者都不自动证明系统队列接受或物理出纸。

## 5. 按优先级排列的后续工作

### P0-A：Windows 安全平台 runtime 与队列证据（2026-08-21 已完成）

**完成记录：** 在 Windows 11 Pro build 22621 上，使用本地文件端口队列 `ISO-PDF-Queue`（Microsoft Print To PDF 驱动，固定输出到本机 print-iso 隔离目录下的 `iso-output.pdf`，仓库外，不出纸、不弹窗，提交前经 SumatraPDF 冒烟验证）完成补验。platform 模式真实枚举系统队列，气球任务 `97654d58a864b473735d097571ba6b15` 经 `queued -> succeeded`（attempts=1），系统队列落盘 289,600 字节隔离 PDF，preview HTTP 200，Ctrl+C 优雅停止后端口释放。连续录屏约 4 分 13 秒。证据：`docs/reports/assets/windows-platform-evidence-2026-08-21.md`、`windows-platform-queue-2026-08-21.mp4`、`windows-iso-output-2026-08-21.pdf`；Day 7–9 报告已回填。该证据证明系统队列接受与隔离输出，不等于物理出纸。

**原始目标（存档）：** 在确认不会实体出纸、不会因交互式保存窗口阻塞的虚拟或隔离队列上，验证 Windows `platform` 模式。

**先读/可能修改：** `README.md`、`docs/testing.md`、`docs/demo-script.md`、`internal/printer/windows.go`、`scripts/run-windows.ps1`；完成后按真实结果更新 `docs/reports/day-07.md`、`docs/reports/day-08.md` 和 `docs/reports/day-09-final.md`。

**验收标准：**

1. 记录 Windows 版本、SumatraPDF 版本和安全队列名称。
2. `platform` 模式能枚举目标队列并提交一张气球任务。
3. 保存任务 ID、状态流转、平台命令结果、系统队列接受证据，以及可获得时的隔离输出。
4. 录屏连续展示启动、探活、提交、预览、队列证据和停止。

**风险：** Print to PDF 等目标可能弹出“另存为”窗口并阻塞；未明确安全目标时禁止试投。

**禁止误报：** 仅 SumatraPDF 返回成功时，不能写成“Windows 已打印”或“已物理出纸”。

### P0-B：Linux/CUPS runtime 与 request id

**目标：** 在真实 Linux 内核环境中验证普通测试、CUPS 枚举和安全队列提交。

**先读/可能修改：** `README.md`、`docs/testing.md`、`internal/printer/linux.go`、`scripts/run-linux.sh`；完成后更新 Day 7–9 报告。

**验收标准：**

1. 记录发行版、内核、Go、Chrome/Chromium、CUPS client 版本。
2. 运行普通测试；具备 cgo 和 C 编译器时再运行 race。
3. 保存 `lpstat` 枚举结果，向已确认安全的队列提交源码任务。
4. 保存 CUPS request id、任务 ID、状态、队列结果和连续录屏。

**风险：** 容器、WSL 或无 system service 的环境可能有 `lp` 命令但没有可用队列；先确认 runtime 和队列，再提交。

**禁止误报：** Windows 上的 `linux/amd64` 交叉编译不能写成 Linux/CUPS runtime 通过。

### P0-C：双平台课程证据整理

**目标：** 形成 Windows、Linux 各一段可从头复核的连续录像和配套文字记录。

**先读/可能修改：** `docs/demo-script.md`、`docs/reports/day-07.md`、`docs/reports/day-08.md`、`docs/reports/day-09-final.md`。

**验收标准：** 每段录像包含系统信息、依赖版本、启动命令、health、打印机枚举、两类任务中至少一类、状态、preview、队列证据和安全退出；报告中的每个结论能指向录像中的可观察画面。

**风险：** 录像可能泄露用户名目录、启动能力值或实体打印机名称；录制前清理画面并使用安全队列。

**禁止误报：** 剪辑后的单张截图不能替代要求中的连续主路径录屏。

### P1-A：真人 README 冷启动

**目标：** 验证未参与开发的人是否能仅凭 README 从干净副本启动 demo 并完成一次预览。

**先读/可能修改：** `README.md`；如发现通用问题，可同步更新 `docs/testing.md`。

**验收标准：** 记录测试者环境、开始和完成时间、原始操作、原话卡点、失败命令、README 修订，以及同一测试者按修订稿重新完成的结果。

**风险：** 开发者口头提示会污染测试；除非涉及安全，应让测试者先按文档独立操作。

**禁止误报：** 开发者本人复跑或 AI 阅读审查不能替代真人冷启动。

### P1-B：可选的平台证据自动化

**目标：** 只有在安全队列可重复提供作业号时，增加显式 opt-in 的平台 E2E，降低人工补验成本。

**可能修改：** `internal/printer/`、`docs/testing.md`、平台专用测试或脚本。

**验收标准：** 默认 `go test ./...` 永不进入系统队列；只有显式环境开关和明确队列名称同时存在时才运行；失败输出保留平台原始证据但不向公开 API 泄露路径或敏感内容。

**风险：** 自动选择默认打印机可能误出纸；不得实现无确认的默认队列试投。

**禁止误报：** opt-in 测试未实际运行时只能写成“已实现”，不能写成“平台已验证”。

### P2：课程验收后的可选扩展

仅在课程验收完成且用户明确批准后，再考虑数据库队列、平台 job id 持久化、多 Worker、多模板、发布打包或显式 OJ 集成。这些不属于当前 P0 缺口，不应抢占补验时间。

## 6. 已知问题与环境依赖

- Go 要求 1.23 或更新版本；Chrome/Chromium 要求 131+。
- Windows `platform` 依赖用户自行提供 SumatraPDF；仓库不应包含未经许可的第三方二进制。
- Linux `platform` 依赖 `lp`、`lpstat` 和预先配置的 CUPS 队列；启动脚本不安装服务、不创建队列、不修改默认打印机。
- 真实 Chrome E2E 可能被桌面权限、容器或组策略阻断并返回 `context canceled`，必须保留原始失败。
- race 测试依赖受支持的 OS/架构、cgo 和 C 编译器；环境不足时应单独记录，不能掩盖普通测试结果。
- 运行数据位于 `data/`，重启会恢复 JSON 任务；不要把 `data/` 或 `.cache/` 加入交付包。
- 当前没有可配置 host、端口或数据目录的 CLI 参数，不要在交接补验中临时改变这些边界。

## 7. 启动命令

Windows 安全 demo：

```powershell
.\scripts\run-windows.ps1 -Mode demo -GoCachePath '.cache\go-build'
```

Windows 平台模式仅在已确认安全队列后使用：

```powershell
.\scripts\run-windows.ps1 `
  -Mode platform `
  -SumatraPath '.\tools\sumatra\SumatraPDF.exe' `
  -GoCachePath '.cache\go-build'
```

Linux 安全 demo：

```bash
chmod +x ./scripts/run-linux.sh
./scripts/run-linux.sh --mode demo --go-cache .cache/go-build
```

Linux 平台模式前必须先检查安全队列：

```bash
command -v lp lpstat
lpstat -p
./scripts/run-linux.sh --mode platform --go-cache .cache/go-build
```

## 8. 接手基线与验证命令

先确认工作区，不覆盖未知改动：

```powershell
git status --short --branch
git log --oneline --decorate -n 15
```

Windows 完整普通回归：

```powershell
$env:GOTOOLCHAIN = 'local'
$cacheDir = New-Item -ItemType Directory -Force '.cache\go-test'
$env:GOCACHE = $cacheDir.FullName
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go mod verify
git diff --check
```

Linux 对应回归：

```bash
export GOTOOLCHAIN=local
mkdir -p .cache/go-test
export GOCACHE="$PWD/.cache/go-test"
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go mod verify
git diff --check
```

真实 Chrome 和平台专项步骤见 `docs/testing.md`。默认自动测试只使用 Fake 或受控 runner，不应进入系统打印队列。

## 9. 推荐接手顺序

1. 阅读本文件、`README.md`、`docs/testing.md`、`docs/demo-script.md` 和 `docs/reports/day-09-final.md`。
2. 检查 Git 状态，运行普通测试，记录当前基线；不要先改代码。
3. ~~准备并确认 Windows 安全虚拟/隔离队列，完成 P0-A。~~（已完成，2026-08-21。）
4. 在真实 Linux/CUPS 环境完成 P0-B。
5. 按 P0-C 整理双平台连续录屏并回填 Day 7–9（Windows 段已有，缺 Linux 段）。
6. 邀请真人执行 README 冷启动，完成 P1-A。
7. 只有证据流程稳定且确有收益时，再评估 P1-B；课程验收前不做 P2。
8. 每完成一项，运行相关测试和全量普通测试，复核 diff，再提交小而清晰的 commit。

## 10. 当前阶段完成定义

只有同时满足以下条件，才能把课程交付状态从“部分完成”改为“完整完成”：

- ~~Windows 安全队列 runtime、系统队列接受和连续录屏已有可审计证据。~~（已完成，2026-08-21。）
- Linux/CUPS runtime、request id、系统队列结果和连续录屏已有可审计证据。
- 真人仅凭 README 从干净副本完成 demo，卡点和修订有记录。
- Day 7、Day 8、Day 9 报告与最新证据一致，不把低层证据冒充高层证据。
- 当前 HEAD 的普通测试、适用环境下的 race、vet、module verify 和 diff 检查完成；所有 skip 和环境限制被单列说明。
- 文档、录像和截图不包含密钥、启动能力值、无关个人路径或未确认的实体打印机信息。

## 11. 可直接复制给 Code Buddy 的开场指令

```text
请先阅读仓库根目录的 PROJECT_HANDOFF.md，并把当前 Git 仓库作为唯一事实源。随后阅读 README.md、docs/testing.md、docs/demo-script.md 和 docs/reports/day-09-final.md，检查 git status 并运行普通测试建立基线。不要覆盖已有改动，也不要先做范围外功能。

请严格按 PROJECT_HANDOFF.md 的 P0 顺序继续：先完成 Windows 安全虚拟/隔离队列 runtime，再完成真实 Linux/CUPS runtime 和 request id，最后整理双平台连续录屏并回填 Day 7–9。任何 platform 打印操作前都必须向我确认具体队列及其不会实体出纸、不会弹出阻塞式保存窗口；禁止自动选择未确认的默认打印机。

每项工作开始前说明目标和将修改的文件；完成后提供环境、命令、输出摘要、任务 ID、平台作业号或队列证据、测试结果和仍未覆盖的边界。始终区分代码存在、受控 runner、平台 runtime、系统队列接受和物理或隔离输出，不得把 demo succeeded、platform succeeded、交叉编译或 AI 审查写成更高层证据。
```
