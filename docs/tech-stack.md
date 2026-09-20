# 技术选型（定稿 v1）

> 状态：2026-09-20 定稿。对应 plan.md 第 5 节，本文是唯一权威版本。
> 原则：团队技能 Java/Go/Python/Vue；渲染引擎当黑盒 CLI（Node 只是运行时依赖，不写 TS 应用代码）；
> 一切服务于 M0（管线打通与实测数据），不上任何为"未来规模"买单的设施。

## 1. 选型总览

| 层 | 选型 | 锁定策略 | 弃选 |
|---|---|---|---|
| 前端框架 | Vue 3（组合式 API）+ Vite | 最新稳定 | React（不熟） |
| 前端语言 | **JavaScript，不用 TS** | — | TypeScript（明确不用） |
| UI 组件库 | **Naive UI** | 最新稳定 | Element Plus（偏后台风，定稿不二选） |
| 状态/路由 | Pinia + vue-router | 最新稳定 | — |
| 后端 | **Go + Gin**（单二进制：API + 编排 + 渲染 worker） | Go 最新 stable | Echo/chi（等价无增益）；Java/Spring（不进 MVP） |
| ORM | GORM | 最新稳定 | sqlc/ent（生成器学习成本 > 收益） |
| 数据库 | SQLite，驱动 **glebarez/sqlite（纯 Go 无 CGO）** | — | mattn/go-sqlite3（CGO，Windows 交叉编译痛） |
| 队列 | SQLite jobs 表 + Go 协程轮询（单并发） | — | Redis/RabbitMQ（单用户阶段纯负担） |
| LLM | **DeepSeek 主选**，GLM / Qwen 备选；OpenAI 兼容协议 + go-openai 库直调 | 型号与价格 M0 锁定日冻结 | LangChain 系（线性管线不需要） |
| TTS | **火山引擎豆包语音主选**；CosyVoice 自托管备选 | M0 实测后冻结 | — |
| 时间戳对齐 | **funASR（Paraformer 字级时间戳）作兜底层** | M0 实测精度 | whisper 类（中文标点/多音字弱于 Paraformer） |
| 渲染引擎 | HyperFrames（黑盒 CLI） | **锁 patch 版本** | Remotion（>3 人公司需付费许可） |
| 渲染运行时 | Node 22 LTS + FFmpeg | 锁 minor / 锁 major | — |
| Python | 仅 M0 验证脚本 + 可能的 TTS 旁路（FastAPI） | — | 主链路零 Python |
| 日志 | Go 标准库 slog（结构化 JSON）+ 每项目 run manifest | — | zerolog（少一个依赖） |
| 部署 | M0：Windows 本机直跑；M1+：go:embed 前端 → 单二进制 + Linux 渲染容器 | — | K8s/微服务 |

## 2. 关键决策的理由

### 2.1 AI 供应商：LLM 与 TTS 分开决策，时间戳能力不绑定 TTS 供应商

**LLM（生成 script.json 与合成物 HTML）**：DeepSeek 主选——中文质量、JSON 结构化输出稳定性、价格
（单条视频 LLM 用量约几万 token，成本可忽略）。GLM、Qwen 作为备选，三家都是 OpenAI 兼容协议，
Go 侧一个 `generate(prompt) → schema 校验` 的薄函数即可切换。**M0 第一周用 10 篇文章跑三家 bake-off
（script.json 一次通过率），锁定一家并冻结型号**。所有 LLM 输出按外部输入做 schema 校验，不信任。

**TTS（单一时间基准的源头）**：火山引擎豆包语音主选，硬门槛是**字级时间戳**。两条路径：
- 路径 A：TTS API 直接返回字级时间戳（火山官方能力，**M0 实测确认精度**）；
- 路径 B（兜底，永远可用）：任意 TTS 出音频 → funASR 强制对齐反打字级时间戳（开源、本地、离线）。

路径 B 的存在意味着：**TTS 供应商随时可换，时间戳能力不随供应商锁定**——这是选型上最重要的对冲。
CosyVoice 自托管（Python 旁路）作为成本/合规备选；Azure TTS（WordBoundary 事件成熟）仅在海外
合规场景考虑。

### 2.2 渲染层：HyperFrames 黑盒化 + 版本冻结

- Go 通过 `exec.CommandContext` 调 hyperframes CLI（`lint` / `check` / `render --quality draft|delivery`），
  上层只见 `submit(project, quality) → job → mp4` 抽象，未来换容器集群/Lambda 不动编排层。
- **锁 patch 版本**（0.8.x 迭代极快，API 可能变），渲染 Dockerfile 里固定精确版本；
  Node 22 LTS 锁 minor，FFmpeg 锁 major。
- 确定性规则（无网络/无时钟/资产本地化）在合成物生成 prompt 与 check 门禁双重强制。

### 2.3 前端预览的简化推论

字幕、逐词高亮都**烧在合成物画面里**（HyperFrames 渲染产物的一部分），所以草稿预览就是一个
`<video>` 标签播 draft.mp4，前端不需要实现任何卡拉OK/字幕引擎。demo 里的逐词高亮是演示效果，
不是 M1 前端工作量。进度推送用原生 EventSource（SSE）。

### 2.4 Go 侧库清单（定稿）

| 用途 | 库 |
|---|---|
| HTTP | gin-gonic/gin |
| 数据访问 | gorm.io/gorm + github.com/glebarez/sqlite |
| LLM 调用 | github.com/sashabaranov/go-openai（OpenAI 兼容，DeepSeek/GLM/Qwen 通吃） |
| JSON Schema 校验 | santhosh-tekuri/jsonschema（LLM 输出门禁） |
| 配置 | 环境变量 + godotenv（.env） |
| 调度 | 标准库 time.Ticker（jobs 轮询） |
| 日志 | 标准库 slog |
| 子进程 | 标准库 os/exec（exec.CommandContext 包 hyperframes CLI） |

M0 无 UI：一个 `cmd/m0` 单命令 Go 程序串全管线（文章 → script.json → TTS → 合成物 → lint/check
→ draft → delivery），不引 cobra。

## 3. M0 第一周验证清单（唯一待实测项，测完即冻结）

| # | 验证项 | 通过标准 |
|---|---|---|
| 1 | 火山 TTS 字级时间戳（10 段旁白） | 路径 A 可用：时间戳与音频对齐无肉眼可辨偏差 |
| 2 | funASR 对齐精度（同 10 段） | 路径 B 可用：字级误差 < 50ms，作为兜底达标 |
| 3 | LLM 三家 bake-off（10 篇文章） | script.json schema 一次通过率 + 单条成本，取最优定主选 |
| 4 | HyperFrames 本机渲染基线 | Windows 本机跑通 draft + delivery 各 10 次，记录耗时/失败率 |

四项全绿 → 供应商与版本冻结，M0 剩余工作纯工程。

## 4. 明确不引入（负面清单）

LangChain 系 / Redis / RabbitMQ / K8s / 微服务 / TypeScript 应用代码 / MongoDB / GraphQL /
任何 agent 编排框架（管线是"线性步骤 + 重试"，Go 循环 + 落盘 manifest 足够）。

## 5. 仓库结构（无 workspace 工具，目录即模块，第一步 git init）

```
webpage-video-agent/
  web/           # Vue 3 + Vite + Naive UI：贴文 / 分镜编辑（门1）/ 进度 SSE / 预览 / 下载
  server/        # Go：cmd/m0(M0 管线) + cmd/server(API+SSE+编排+渲染 worker+jobs)
  ai/            # Python 验证脚本（funASR 对齐 / CosyVoice 试听），M0 后按需保留
  data/          # 运行时产物（gitignore）：projects/<id>/{script.json, audio/, compositions/, renders/, manifest.jsonl}
  docs/          # plan.md / tech-stack.md
  demo/          # ui-demo.html（M1 交互规格演示，非产品代码）
```
