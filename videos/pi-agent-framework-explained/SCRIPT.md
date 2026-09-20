# SCRIPT — pi-agent-framework-explained

**Voice:** Kokoro（离线引擎）· 中文男声，具体音色以 Step 3.1 供应商实况锁定
**Voice settings:** 默认（Kokoro 无稳定性参数）
**Voice direction:** 像朋友在白板前讲解：轻松、清楚、略带调侃，不端着，不喊口号。

---

## Line 1 — Hook (Frame 1)

**Time:** 0.0 – 9.0s
**Delivery:** 先共情后转折，"干脆自己写了一个"要带一点笑意。

    你每天让 AI 帮你写代码——可它背着你干了什么，你说不清。有个老哥干脆自己写了一个。

## Line 2 — Pain (Frame 2)

**Time:** 9.0 – 21.0s
**Delivery:** 三宗罪逐条数落，节奏渐紧，最后"越强，越黑"一字一顿。

    主流的 agent 外壳越做越厚——偷偷塞上下文，提示词改了也不吭声，想看看它在干嘛，还得翻半天日志。越强，越黑。

## Line 3 — Intro (Frame 3)

**Time:** 21.0 – 34.0s
**Delivery:** 介绍人物时放慢半拍，原则那句加重。

    这位老哥是 libGDX 的作者，Mario Zechner。他给项目起名 Pi：一个 TypeScript 写的开源 agent，原则一句话——用不到的，就不造。

## Line 4 — Parts (Frame 4)

**Time:** 34.0 – 50.0s
**Delivery:** 四个零件逐个点名，每个名字后微顿，让画面跟上。

    Pi 是搭出来的：pi-ai，一个接口接通所有大模型；pi-agent-core，包住 agent 循环；pi-tui，画出终端界面；pi-coding-agent，把它们接成命令行工具。四个零件，各干一件事。

## Line 5 — Loop (Frame 5)

**Time:** 50.0 – 65.0s
**Delivery:** 循环那句像转圈一样有推进感，"全程看得见"是本句落点。

    心脏是一个循环：模型开口，要工具就执行，结果回填，再问模型——直到它自己说完成。没有步数上限，每一步都发事件，全程看得见。

## Line 6 — Numbers (Frame 6)

**Time:** 65.0 – 79.0s
**Delivery:** 报数字干脆利落，"比一封周报还短"带调侃上扬。

    极简到什么程度？核心工具只有四个：读、写、改、跑命令。系统提示加工具定义，不到一千个 token——比一封周报还短。

## Line 7 — Subtraction (Frame 7)

**Time:** 79.0 – 93.0s
**Delivery:** 前半句平铺直叙，"子代理？"停半拍，答案轻快甩出。

    它没有内置待办、计划模式、MCP、子代理。待办就是 TODO.md，计划就是 PLAN.md，后台任务丢给 tmux，子代理？在 bash 里再起一个 Pi。

## Line 8 — Why (Frame 8)

**Time:** 93.0 – 106.0s
**Delivery:** 两句对仗（读得完/拆得开）稳重，证据句陈述即可不吹。

    小，不是抠门——读得完，才敢信；拆得开，就能改。它在 Terminal-Bench 上跟那些大家伙同场竞技，技能还兼容 Claude Code 和 Codex。

## Line 9 — Outro (Frame 9)

**Time:** 106.0 – 118.0s
**Delivery:** 收束放慢，最后一句是邀请不是命令。

    别人家把外壳越做越厚，Pi 反着走：内核读得完，零件换得动。想弄懂 agent 到底怎么工作——GitHub 搜 pi-mono，自己读一遍。
