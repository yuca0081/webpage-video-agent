---
workflow: faceless-explainer
flow: automation
storyboard: no
message: "一个可信的编程 agent 不需要庞大黑盒——极简内核加可组合的零件就够了"
destination: youtube
aspect: 1920x1080
language: zh
length: 120s
angle: concept
narration: yes
---

## Intent

科普视频：讲解 Pi 这个开源编程 agent 框架（Mario Zechner / badlogic 的 pi-mono）。
用户点名**手绘风格**（画在纸上的讲解感，不是科技渐变风）、时长 2 分钟、中文旁白。
受众：对 AI 编程 agent 感兴趣的开发者。语气：轻松、清晰、略带调侃（呼应 Pi 作者的反潮流气质）。

这是平台 M0 的最小实现预演：LLM 角色由编排者本人担任（无外部 LLM API），TTS/BGM 走工作流自带管线。

## Notes

- 事实来源（内容必须可溯源）：
  - Mario Zechner 博文 "What I learned building an opinionated and minimal coding agent"（2025-11-30，mariozechner.at）
  - 仓库 github.com/badlogic/pi-mono（TypeScript/Node）
- 必须讲到的硬事实：四个核心工具（read/write/edit/bash）；系统提示+工具 <1000 token；pi-ai（统一 LLM API，多供应商）/ pi-agent-core（agent 循环+事件流）/ pi-tui（终端 UI）/ pi-coding-agent（CLI：会话/斜杠命令/headless 模式）；循环无步数上限（"循环一直转到 agent 说完成"）；没有内置待办/计划模式/MCP/子代理——用文件（TODO.md/PLAN.md）、CLI 工具、tmux 替代；作者 libGDX（Java 游戏框架）出身。
- 不要贬低具体竞品名称，用"主流 agent 外壳"指代。
- 视觉基调：手绘纸面、铅笔/马克笔线条、便利贴、白板感的图解。
