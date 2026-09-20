---
format: 1920x1080
duration: 120s
message: "一个可信的编程 agent 不需要庞大黑盒——极简内核加可组合的零件就够了"
arc: Hook → Pain → Concept intro → Mechanism (parts → loop) → Proof (numbers → subtraction) → Implication → Thesis + CTA
audience: 对 AI 编程 agent 感兴趣的开发者
mode: autonomous
music: playful warm acoustic underscore, light hand-drawn explainer feel
---

## Video direction

- **Palette**（frame.md = daisy-days）：cream 纸面为 ground；pastel 表面（turquoise/pink/butter/mint/peach/sky）做卡片与便签轮换；charcoal 3px 描边 + 硬投影（6/4px 无模糊）；coral 只做马克笔强调（下划线/圈/章），永远不做大面积底色。
- **Type**（按 role 引用 frame.md 字阶）：display = Caveat（本地 woff2，拉丁手写）→ 楷体（中文，系统 simkai）；body/meta = 楷体；代码/包名/路径 = mono（本地 `assets/fonts/courier-prime-700.woff2`，合成物内自行 @font-face 声明）。**不引入任何网络字体**；拉丁 web 字体一律用 assets/fonts 里已有的 Caveat / Permanent Marker，需在合成物内以相对路径 @font-face 声明（渲染器不自动解析字体名）。中文一律走楷体（KaiTi/STKaiti），保持手写讲解气质。
- **手绘语言（全片统一）**：线条用 `hw-boil` 家族的确定性 boil（量化重摆，无随机、无循环）；强调用 `marker-highlight` / `hw-underline` / `hw-callout-circle` 的 draw-on 马克笔；框与连线用 `hw-box-label` / `hw-arrow` 的 wobble draw-on；纸面可贴便签（硬投影小卡）。
- **Motion grammar**：`power3` 长尾减速为默认（平滑优先，禁 bouncy/elastic）；**VO 逐 cue 揭示**——t=0 只出现旁白正在说的内容，其余元素等自己的语音点再进场，揭示分布在后 ~50%；落定后静止（至多 subtle jitter / boil 保持活性）；无懒呼吸、无后半程慢推拉；帧内接缝用 velocity-matched cut。
- **节奏与静止帧分配**：F6 尾拍与 F9 为 breather（落定即静读）；其余帧按 VO 揭示推进。
- **Negative list**：不出现真实浏览器/编辑器 chrome 与光标；不做紫蓝"AI 渐变"俗套；无 `repeat`/`yoyo`/`Math.random`；无 slideshow（前 25% 倾倒后冻结）；无 screensaver（万物漂浮）。字幕带（底部 ~17%）不承放任何主体内容。

## Frame 1 — 你看得懂你的 AI 吗

- scene: 手绘纸面上大标题「你看得懂你的 AI 吗？」，一个 agent 小人涂鸦耸肩，问号便签散落
- voiceover: "你每天让 AI 帮你写代码——可它背着你干了什么，你说不清。有个老哥干脆自己写了一个。"
- duration: 9.792s
- transition_in: cut
- status: animated
- src: compositions/frames/01-hook.html
- type: hook
- persuasion: Pain validation + curiosity gap
- beat: recognition + intrigue
- blueprint: kinetic-type-beats (Adapt)
- focal: 手写大标题「你看得懂你的 AI 吗？」（hw-title）
- roles: 大标题 = foreground subject（占 ~50%，centered 偏上）；纸面网格+角落涂鸦 = background（压暗 ~35%）；问号便签 ×3 = supporting（落定后 boil）
- sfx: pencil scribble, pop

Adapt: 保留 kinetic-type 的"节拍落字 + 弹出收尾"签名，但改为手写揭示（不是无衬线 slam）。
Scene 1 (0.0–2.6s): cream 纸面 + 极淡网格打底；大标题以 per-word staggered reveal（`dynamic-content-sequencing`）逐词落上，`power3` 长尾；标题词落完即静止。Centered，~50% 画幅。
Scene 2 (2.6–6.3s): 旁白说到"背着你干了什么"——一个方块黑盒涂鸦（`hw-box-label` draw-on）滑到标题右下，三张问号便签依次 pop 进（spring-pop 平滑版）；便签带低幅 boil（`hw-boil` calm）。
Scene 3 (6.3–9.79s): "干脆自己写了一个"——左侧一个手绘小人拿起巨大铅笔（SVG self-draw `svg-path-draw`），铅笔尖冒出一个小星标；全场静止读题，仅 boil 保活。

## Frame 2 — 越强，越黑

- scene: 便签与工具图标在纸面上越堆越多、向中心围拢，小人被埋住，马克笔重描「黑盒」二字
- voiceover: "主流的 agent 外壳越做越厚——偷偷塞上下文，提示词改了也不吭声，想看看它在干嘛，还得翻半天日志。越强，越黑。"
- duration: 13.312s
- transition_in: cut
- status: animated
- src: compositions/frames/02-pain.html
- type: pain_point
- persuasion: Common-belief vs reality（越强 ≠ 越可信）
- beat: frustration + concern
- blueprint: overwhelm-surround (Adapt)
- focal: 中心被便签围住的开发者小人，收束到「黑盒」二字重描
- roles: 开发者小人 = foreground subject（中心）；累积便签/工具涂鸦 = supporting（逐步围拢，8–10 张）；纸面 = background
- sfx: paper slide ×4, marker scribble

Adapt: 保留"累积围拢"签名（杂物从四面收进来，不是 zoom-in）；被围主体从头像改为手绘小人，收尾以 marker 重描「黑盒」替代 claustrophobic 特写。
Scene 1 (0.0–2.9s): 纸面中央开发者小人（draw-on）面对一台笔记本涂鸦；"越做越厚"——两张大便签从左右 pop 贴上（butter/peach 面）。
Scene 2 (2.9–6.7s): "偷偷塞上下文"——便签雨 staggered 落下（`center-outward-expansion` 反向：外向内贴），每张写一个词（上下文/钩子/注入）；密度渐增，layered-depth 3 层。
Scene 3 (6.7–9.8s): "提示词改了也不吭声"——一张便签上 prompt v1 被划掉、v2 写上（`hw-underline` 双道删除线 draw-on）；"翻半天日志"——纸堆从底部涨高。
Scene 4 (9.8–13.31s): 全部杂物向中心收拢一档（velocity-matched micro-shove），coral 马克笔把「黑盒」二字重重圈出（`hw-callout-circle`）；静止，仅 boil。

## Frame 3 — 它叫 Pi

- scene: 干净纸面，马克笔写下沉重的「π」与名字卡 Pi，旁注作者签名「badlogic · libGDX 作者」，原则一句手写下划线
- voiceover: "这位老哥是 libGDX 的作者，Mario Zechner。他给项目起名 Pi：一个 TypeScript 写的开源 agent，原则一句话——用不到的，就不造。"
- duration: 14.272s
- transition_in: push-slide LEFT
- status: animated
- src: compositions/frames/03-intro.html
- type: product_intro
- persuasion: Concept announcement + Coined principle（「用不到的就不造」）
- beat: clarity + anticipation
- blueprint: kinetic-type-beats (Reproduce)
- focal: 「π / Pi」名字卡（hw-write-title 真笔迹 write-on）
- roles: π 名字卡 = foreground subject（~45%，centered 偏上）；作者签名卡 = supporting（左上角）；两枚标签便签（TypeScript · 开源）= supporting；原则句 = 收尾主读
- sfx: pen write-on, pop ×2, marker underline

Reproduce: 名字 slam 收尾的 kinetic-type 名字卡形态。
Scene 1 (0.0–3.5s): "libGDX 的作者"——左上角签名卡（badlogic · libGDX 作者）以 per-word reveal 落定；纸面干净，rule-of-thirds 上带。
Scene 2 (3.5–7.0s): "起名 Pi"——中央 π 大字真笔迹 write-on（`hw-write-title` / `svg-path-draw`），旁边手写 "Pi" 小字跟上；smooth settle 无 overshoot。
Scene 3 (7.0–10.6s): "TypeScript 开源 agent"——两枚标签便签从右侧 pop 贴到名字卡下（turquoise/mint 面）。
Scene 4 (10.6–14.27s): "用不到的，就不造"——底部原则句 per-word 落上，coral squiggle 下划线 draw-on（`hw-underline`）；静止读。

## Frame 4 — 四个零件

- scene: 手绘工作台：四张零件卡片（pi-ai / pi-agent-core / pi-tui / pi-coding-agent）逐张贴上，各配一行手写职责注释
- voiceover: "Pi 是搭出来的：pi-ai，一个接口接通所有大模型；pi-agent-core，包住 agent 循环；pi-tui，画出终端界面；pi-coding-agent，把它们接成命令行工具。四个零件，各干一件事。"
- duration: 18.091s
- transition_in: push-slide LEFT
- status: animated
- src: compositions/frames/04-parts.html
- type: feature_showcase
- persuasion: Frame-then-fill + Numbered enumeration
- beat: comprehension + momentum
- blueprint: grid-card-assemble (Reproduce)
- focal: 四张零件卡组成的 2×2 工作台网格（hw-box-label wobble 框）
- roles: 四张零件卡 = foreground subject（合计 ~55%，2×2 网格）；包名 = mono 卡头；一行手写职责 = 卡内 body；桌面纸纹 = background
- sfx: paper slide ×4, tick ×4, whoosh-soft

Reproduce: staggered cascade 自组装网格；每张卡等自己的 VO 点。
Scene 1 (0.0–2.3s): "Pi 是搭出来的"——顶部小标题落定（per-word reveal）；空 2×2 网格虚线位（极淡）先立起。
Scene 2 (2.3–5.2s): "pi-ai"——卡 1 pop 进（turquoise 面），卡内 mono 包名 + 手写"一个接口接通所有大模型"，四枚小厂牌点（OpenAI/Anthropic/Google/…）staggered 亮起。
Scene 3 (5.2–8.1s): "pi-agent-core"——卡 2 贴上（pink 面），内画一个小循环圈图标 draw-on。
Scene 4 (8.1–11.1s): "pi-tui"——卡 3 贴上（butter 面），内画终端窗涂鸦 + 光标块。
Scene 5 (11.1–14.0s): "pi-coding-agent"——卡 4 贴上（peach 面），`hw-arrow` 从前三卡各引一条 wobble 线汇入卡 4。
Scene 6 (14.0–18.09s): "四个零件，各干一件事"——右上角珊瑚手写计数「4」count-up（`counting-dynamic-scale`）+ 全卡同步 boil 一拍；静止读。

## Frame 5 — 心脏是循环

- scene: 同一工作台中央画出循环图（模型 → 工具 → 结果回填 → 模型），箭头逐段描出，旁边事件流小票滚动，最后盖「完成」章
- voiceover: "心脏是一个循环：模型开口，要工具就执行，结果回填，再问模型——直到它自己说完成。没有步数上限，每一步都发事件，全程看得见。"
- duration: 15.083s
- transition_in: push-slide LEFT
- status: animated
- src: compositions/frames/05-loop.html
- type: feature_showcase
- persuasion: Causal chain (A → B → C) + Demonstration
- beat: aha + orientation
- blueprint: agent-progress-theater (Adapt)
- focal: 中央循环图（hw-pipeline / hw-arrow 组合）
- roles: 循环图（模型↔工具 两节点 + 环形箭头）= foreground subject（~50%，centered）；事件流小票 = supporting（右侧长条）；「完成」章 = climax 道具；工作台纸面 = background
- sfx: tick ×3, whoosh-soft, stamp

Adapt: 保留"工作状态剧场 → 回执打钩"签名；把 loader/checklist 换成手绘循环图逐段 draw-on + 事件小票逐行 check + 盖章收尾。
Scene 1 (0.0–2.8s): "心脏是一个循环"——中央两个 wobble 节点「模型」「工具」先后 draw-on（`hw-box-label`），节点间第一条 `hw-arrow` 弯箭头描出。
Scene 2 (2.8–6.2s): "要工具就执行，结果回填"——环形第二、三条箭头顺时针逐段 draw-on，一个小代码片段图标沿箭头走到"工具"再返回（token 图标滑行）。
Scene 3 (6.2–9.0s): "直到它自己说完成"——循环圈上盖一枚 coral「完成」手绘章（stamp：scale 落定 + 微倾斜）；章体带 calm boil。
Scene 4 (9.0–12.5s): "每一步都发事件"——右侧事件小票（长条纸）从上往下逐行 reveal（`marker-checklist-card` 行式），每行左侧勾选 mark draw-on：读取文件 ✓ / 编辑代码 ✓ / 跑测试 ✓。
Scene 5 (12.5–15.08s): "全程看得见"——小票整体上提半档（velocity-matched），循环图与小票同框静止读；boil 保活。

## Frame 6 — 四个工具，一千 token

- scene: 大数字手写计数：工具「4」、系统提示「< 1000 token」，旁边一封「周报」便签作对比，圈出重点
- voiceover: "极简到什么程度？核心工具只有四个：读、写、改、跑命令。系统提示加工具定义，不到一千个 token——比一封周报还短。"
- duration: 12.821s
- transition_in: push-slide LEFT
- status: animated
- src: compositions/frames/06-numbers.html
- type: social_proof
- persuasion: Statistical proof + Anchoring（「比一封周报还短」）
- beat: surprise + delight
- blueprint: dataviz-countup (Adapt)
- focal: 两个手写大数字「4」与「< 1000 token」
- roles: 大数字对 = foreground subject（左右不对称 60/40）；四格工具名 = supporting（读/写/改/跑命令 mini 卡）；周报便签 = 锚点道具；纸面 = background
- sfx: tick ×4, count-up ticks, marker circle

Adapt: 保留 count-up 签名；图表换成手写大数字 + 四格枚举，锚定物为"一封周报"便签。
Scene 1 (0.0–2.0s): "极简到什么程度？"——问句小字 pop 在顶部；两个空的圆圈占位 draw-on。
Scene 2 (2.0–5.1s): "只有四个：读、写、改、跑命令"——四枚 mini 卡 staggered 贴成一排（`center-outward-expansion`），左侧大数字「4」随之 count-up 写出（`counting-dynamic-scale`，手写感数字）。
Scene 3 (5.1–8.8s): "不到一千个 token"——右侧第二个圈里数字从 0 快速滚到 999 再定格「<1000」（value-scaled counter），单位 token 用 mono 小字标出。
Scene 4 (8.8–12.82s): "比一封周报还短"——下方贴一封写满小字的"周报"便签（ink 密集但小），coral 圈把两个大数字一起圈住（`hw-callout-circle`）；breather：静止读，仅 boil。

## Frame 7 — 没有，是用别的东西

- scene: 左列「没有：待办 / 计划模式 / MCP / 子代理」逐条划掉，右侧对应替代（TODO.md、PLAN.md、tmux、再起一个 Pi）便利贴拍上
- voiceover: "它没有内置待办、计划模式、MCP、子代理。待办就是 TODO.md，计划就是 PLAN.md，后台任务丢给 tmux，子代理？在 bash 里再起一个 Pi。"
- duration: 16.768s
- transition_in: cut
- status: animated
- src: compositions/frames/07-subtraction.html
- type: feature_showcase
- persuasion: Subtractive framing（用「没有什么」定义）+ Counterexample mapping
- beat: amusement + conviction
- blueprint: kinetic-type-beats (Adapt)
- focal: 左栏固定词「没有：」+ 槽位词轮换（in-place token swap 签名）
- roles: 左栏槽位词 = foreground subject；右侧替代便签 = supporting（每条一贴）；箭头 = 连接道具；纸面 = background
- sfx: swish ×4, paper slide ×3, pop

Adapt: 保留"in-place token swap"签名——左栏固定「没有：」，槽位逐词轮换并被划掉，右侧同步拍上替代便签。
Scene 1 (0.0–2.9s): "它没有内置"——左栏标题「没有：」落定（per-word），一个空槽位框 draw-on。Split 60/40（左栏 60%）。
Scene 2 (2.9–5.8s): 槽位词「待办」翻入（`discrete-text-sequence` 硬切），随"待办就是 TODO.md"——coral 双删除线划掉，右侧 TODO.md 便签 pop 贴上，`hw-arrow` 连一条 wobble 线。
Scene 3 (5.8–8.6s): 槽位换「计划模式」→ 划掉 → PLAN.md 便签贴上。
Scene 4 (8.6–11.5s): 槽位换「MCP」→ 划掉 → 右侧手写小注"不需要"（无替代，直接一个摊手小人涂鸦）。
Scene 5 (11.5–14.9s): 槽位换「子代理？」（问号加重）→ 右侧便签"在 bash 里再起一个 Pi"拍上，便签里画两个小 π 对话气泡（`hw-text-cloud`）。
Scene 6 (14.9–16.77s): 四组对照同框静止读；左栏全部带删除线，boil 保活。

## Frame 8 — 小，不是抠门

- scene: 三句手写标语逐条落在纸面（读得完才敢信 / 拆得开就能改 / 同场竞技），角落 Terminal-Bench 计分卡涂鸦
- voiceover: "小，不是抠门——读得完，才敢信；拆得开，就能改。它在 Terminal-Bench 上跟那些大家伙同场竞技，技能还兼容 Claude Code 和 Codex。"
- duration: 14.016s
- transition_in: push-slide LEFT
- status: animated
- src: compositions/frames/08-why.html
- type: benefit_highlight
- persuasion: Causal chain + Citation（Terminal-Bench / 技能兼容）
- beat: conviction + respect
- blueprint: kinetic-type-beats (Reproduce)
- focal: 两句对仗手写标语「读得完，才敢信 / 拆得开，就能改」
- roles: 对仗标语 = foreground subject（~50%，居中偏上两行）；Terminal-Bench 计分卡 = supporting（右下角）；兼容标签 ×2 = supporting（mint/sky 面）；纸面 = background
- sfx: marker slam ×2, tick, pop ×2

Reproduce: 声明句逐条落的 statement relay。
Scene 1 (0.0–2.8s): "小，不是抠门"——顶部短句 pop 落定，coral squiggle 下划线跟上。
Scene 2 (2.8–6.5s): "读得完，才敢信"——第一行大字 per-word reveal，旁边一本薄薄的小书涂鸦 draw-on（几笔线装书）。
Scene 3 (6.5–9.5s): "拆得开，就能改"——第二行大字落上，旁边一个被拆成四块的拼图涂鸦（四块 staggered 弹开再合拢一次，velocity-matched）。
Scene 4 (9.5–14.02s): "Terminal-Bench 同场竞技"——右下角计分卡便签贴上（写 Terminal-Bench 2.0 + 一排小柱状涂鸦）；"兼容 Claude Code 和 Codex"——两枚兼容标签 pop 在计分卡上方；全场静止读。

## Frame 9 — 自己读一遍

- scene: π 标志手绘描边收笔定格，下方手写仓库地址 github.com/badlogic/pi-mono，收尾定格
- voiceover: "别人家把外壳越做越厚，Pi 反着走：内核读得完，零件换得动。想弄懂 agent 到底怎么工作——GitHub 搜 pi-mono，自己读一遍。"
- duration: 13.803s
- transition_in: crossfade
- status: animated
- src: compositions/frames/09-outro.html
- type: cta
- persuasion: Callback（黑盒 → 读得完）+ Distillation + 行动号召
- beat: resolve + satisfaction
- blueprint: logo-assemble-lockup (Reproduce)
- focal: π 标志 + 仓库地址的收尾 lockup
- roles: π lockup + 地址 = foreground subject（~45%，centered 偏上）；左右对比小涂鸦（厚外壳 vs 小内核）= supporting（开场一闪）；纸面 = background
- sfx: pen draw-on, soft stamp

Reproduce: 标志"从无到有"描边收笔、解析为 lockup + 地址尾卡的完整形态。
Scene 1 (0.0–3.5s): "别人家越做越厚，Pi 反着走"——左上/右上两个小涂鸦对比出现：左边一摞厚壳便签（staggered 叠高），右边一个小小的 π；右侧 π 一笔画出（`svg-path-draw`）。
Scene 2 (3.5–7.3s): "内核读得完，零件换得动"——中央大 π 标志真笔迹 write-on 收笔（`hw-write-title`），两行小副标 per-word 落在标志下方。
Scene 3 (7.3–10.8s): "GitHub 搜 pi-mono"——mono 手写地址 `github.com/badlogic/pi-mono` type-on with caret（`discrete-text-sequence` + 光标闪烁有限拍数）。
Scene 4 (10.8–13.8s): 尾拍：coral 「自己读一遍」小章盖在地址旁（stamp 落定）；全场静止，breather 收尾，仅 boil。
