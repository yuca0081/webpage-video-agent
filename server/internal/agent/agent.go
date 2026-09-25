// Package agent：主管对话 Agent（ReAct 循环 + 工具调用，Go 侧驱动）。
// 护栏：① 工具受状态机门禁（越界调用被拒、原因回喂）② 循环上限 8 步
// ③ LLM 永不直接碰文件——只发起工具调用，Go 校验后执行。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"webpage-video-agent/server/internal/contract"
	"webpage-video-agent/server/internal/llm"
	"webpage-video-agent/server/internal/methodlib"
	"webpage-video-agent/server/internal/pipeline"
	"webpage-video-agent/server/internal/store"
)

type Producer interface {
	Start(projectID string) (bool, error)
	Rework(projectID, segID, instruction string) (bool, error)
	IsRunning(projectID string) bool
}

type Agent struct {
	DataDir  string
	RootDir  string // 仓库根（读 ai/registry 默认样张）
	Store    store.Store
	Lib      *methodlib.Library
	Emit     func(projectID, typ, stage, detail string)
	Producer Producer
	OnEvent  func(projectID, event, detail string) // 状态回写钩子（storyboard/style 等里程碑）
}

// ── 项目状态（从磁盘产物读，磁盘是唯一事实源）──────────────────

type State struct {
	Name           string
	HasManuscript  bool
	HasStoryboard  bool
	SegCount       int
	TotalHint      float64
	HasStyle       bool
	StyleDirection string
	StyleConfirmed bool
	HasVideo       bool
	Producing      bool
}

func (a *Agent) State(id string) State {
	p := pipeline.NewProject(a.DataDir, id)
	st := State{Producing: a.Producer != nil && a.Producer.IsRunning(id)}
	// 文稿门：input/article.txt（创建即有）或管线产物 manuscript.json 任一存在
	if b, err := os.ReadFile(p.Artifact("input/article.txt")); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		st.HasManuscript = true
	} else if b, err := os.ReadFile(p.Artifact("manuscripts/manuscript.json")); err == nil && len(b) > 0 {
		st.HasManuscript = true
	}
	if sb, err := p.LoadStoryboard(); err == nil {
		st.HasStoryboard = true
		st.SegCount = len(sb.Segments)
		st.TotalHint = sb.TotalHint()
	}
	var ss contract.StyleSamples
	if b, err := os.ReadFile(p.Artifact("style/style_samples.json")); err == nil && json.Unmarshal(b, &ss) == nil {
		st.HasStyle, st.StyleDirection, st.StyleConfirmed = true, ss.Direction, ss.Confirmed
	}
	if _, err := os.Stat(p.Artifact("renders/main.mp4")); err == nil {
		st.HasVideo = true
	}
	if b, err := os.ReadFile(p.Artifact("project.json")); err == nil {
		var pj struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(b, &pj)
		st.Name = pj.Name
	}
	return st
}

func (s State) summary() string {
	lines := []string{}
	add := func(ok bool, what string) {
		if ok {
			lines = append(lines, "✓ "+what)
		} else {
			lines = append(lines, "✗ "+what)
		}
	}
	add(s.HasManuscript, "文稿")
	if s.HasStoryboard {
		add(true, fmt.Sprintf("分镜（%d 段 / %.0f 秒）", s.SegCount, s.TotalHint))
	} else {
		add(false, "分镜")
	}
	if s.HasStyle {
		if s.StyleConfirmed {
			add(true, fmt.Sprintf("风格已确认（%s）", s.StyleDirection))
		} else {
			lines = append(lines, "⏸ 风格样张已出，等用户在舞台点击确认（硬门）："+s.StyleDirection)
		}
	} else {
		add(false, "风格样张")
	}
	if s.HasVideo {
		lines = append(lines, "✓ 成片已就绪")
	}
	if s.Producing {
		lines = append(lines, "⏳ 制作管线运行中")
	}
	return strings.Join(lines, "\n")
}

// selectedPack 建项目时选定的方法库风格包（project.json stylepack_id）。
func (a *Agent) selectedPack(p *pipeline.Project) *methodlib.StylePack {
	b, err := os.ReadFile(p.Artifact("project.json"))
	if err != nil {
		return nil
	}
	var meta struct {
		StylePackID string `json:"stylepack_id"`
	}
	if json.Unmarshal(b, &meta) != nil || meta.StylePackID == "" {
		return nil
	}
	pack, ok := a.Lib.Get(meta.StylePackID)
	if !ok || !pack.Published {
		return nil
	}
	return pack
}

// resolveSegment 用户话里的段引用（3 / 段3 / seg03）→ 分镜里的段。
func (a *Agent) resolveSegment(p *pipeline.Project, ref string) (id, label string, err error) {
	sb, lerr := p.LoadStoryboard()
	if lerr != nil {
		return "", "", fmt.Errorf("分镜未生成")
	}
	ref = strings.TrimSpace(ref)
	n := -1
	if strings.HasPrefix(ref, "seg") {
		if s, e := strconv.Atoi(strings.TrimPrefix(ref, "seg")); e == nil {
			n = s
		}
	} else {
		s, e := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(ref, "段"), "第"))
		if e == nil {
			n = s
		}
	}
	if n > 0 {
		want := fmt.Sprintf("seg%02d", n)
		for _, seg := range sb.Segments {
			if seg.ID == want {
				return seg.ID, fmt.Sprintf("段%d「%s」", seg.Idx, seg.Key), nil
			}
		}
	}
	for _, seg := range sb.Segments {
		if seg.ID == ref {
			return seg.ID, fmt.Sprintf("段%d「%s」", seg.Idx, seg.Key), nil
		}
	}
	return "", "", fmt.Errorf("段 %q 不在分镜里（共 %d 段）", ref, len(sb.Segments))
}

// ── 工具定义 ─────────────────────────────────────────────────

type tool struct {
	def  openai.Tool
	gate func(s State) string      // 返回空串 = 放行；否则拒绝原因（回喂）
	run  func(a *Agent, p *pipeline.Project, args string) string
}

func noGate(State) string { return "" }

var tools = []tool{
	{
		def: toolDef("draft_storyboard", "根据文稿生成分镜表（LLM 作业，自动完成）", nil),
		gate: func(s State) string {
			if !s.HasManuscript {
				return "还没有文稿，无法出分镜"
			}
			return ""
		},
		run: func(a *Agent, p *pipeline.Project, _ string) string {
			if err := pipeline.Run(context.Background(), p, "storyboard"); err != nil {
				return "分镜生成失败：" + err.Error()
			}
			sb, _ := p.LoadStoryboard()
			if a.OnEvent != nil {
				a.OnEvent(p.ID, "storyboard", "")
			}
			return fmt.Sprintf("分镜已生成：%d 段 / 约 %.0f 秒。思路：%s", len(sb.Segments), sb.TotalHint(), sb.Plan)
		},
	},
	{
		def: toolDef("update_storyboard", "按用户要求重新生成分镜表", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"instruction": map[string]any{"type": "string", "description": "用户对分镜的修改要求（如：每段更短、合并前两段）"},
			},
			"required": []string{"instruction"},
		}),
		gate: func(s State) string {
			if !s.HasManuscript {
				return "还没有文稿"
			}
			if s.Producing {
				return "制作运行中，先等本条完成"
			}
			return ""
		},
		run: func(a *Agent, p *pipeline.Project, args string) string {
			var in struct {
				Instruction string `json:"instruction"`
			}
			_ = json.Unmarshal([]byte(args), &in)
			if strings.TrimSpace(in.Instruction) == "" {
				return "缺少修改要求"
			}
			_ = os.WriteFile(p.Artifact("input/storyboard_notes.txt"), []byte(in.Instruction), 0o644)
			_ = os.Remove(p.Artifact("storyboards/storyboard.json"))
			_ = os.Remove(p.LLMOutput("storyboard"))
			if err := pipeline.Run(context.Background(), p, "storyboard"); err != nil {
				return "分镜重生成失败：" + err.Error()
			}
			sb, _ := p.LoadStoryboard()
			if a.OnEvent != nil {
				a.OnEvent(p.ID, "storyboard", "")
			}
			return fmt.Sprintf("分镜已按「%s」重生成：%d 段 / 约 %.0f 秒（若已有音频/画面需重新制作才会生效）",
				in.Instruction, len(sb.Segments), sb.TotalHint())
		},
	},
	{
		def: toolDef("draft_style_samples", "产出风格样张（方法库选定的风格包，或默认风格包冷启动），等用户在舞台确认", nil),
		gate: func(s State) string {
			if !s.HasStoryboard {
				return "先出分镜，再出风格样张"
			}
			return ""
		},
		run: func(a *Agent, p *pipeline.Project, _ string) string {
			out := p.Artifact("style/style_samples.json")
			if _, err := os.Stat(out); err == nil {
				return "风格样张已出，等用户在舞台点击「确认风格」"
			}
			// 来源优先级：项目选定的方法库风格包 → 默认风格包（冷启动）
			if pack := a.selectedPack(p); pack != nil {
				if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
					return err.Error()
				}
				if err := os.WriteFile(out, pack.StyleSamplesJSON(), 0o644); err != nil {
					return err.Error()
				}
				if a.OnEvent != nil {
					a.OnEvent(p.ID, "style_draft", pack.Direction)
				}
				return fmt.Sprintf("风格样张已出（复用方法库「%s」，origin=%s）。硬门——用户在舞台点头确认后才能开工", pack.Name, pack.OriginProject)
			}
			b, err := os.ReadFile(filepath.Join(a.RootDir, "ai", "registry", "style_samples_default.json"))
			if err != nil {
				return "默认风格包缺失：" + err.Error()
			}
			var ss contract.StyleSamples
			if err := json.Unmarshal(b, &ss); err != nil {
				return "默认风格包损坏：" + err.Error()
			}
			ss.Confirmed = false // 硬门：必须用户确认
			b, _ = json.MarshalIndent(ss, "", "  ")
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err.Error()
			}
			if err := os.WriteFile(out, b, 0o644); err != nil {
				return err.Error()
			}
			if a.OnEvent != nil {
				a.OnEvent(p.ID, "style_draft", ss.Direction)
			}
			return fmt.Sprintf("风格样张已出：%s（%d 张，舞台可预览）。这是硬门——用户在舞台点头确认后才能开工", ss.Direction, len(ss.Samples))
		},
	},
	{
		def: toolDef("run_selfcheck", "自检三前置硬门（文稿/分镜/风格确认）", nil),
		gate: noGate,
		run: func(a *Agent, p *pipeline.Project, _ string) string {
			s := a.State(p.ID)
			var r []string
			r = append(r, fmt.Sprintf("文稿：%v", s.HasManuscript))
			if s.HasStoryboard {
				r = append(r, fmt.Sprintf("分镜：%d 段 / %.0f 秒", s.SegCount, s.TotalHint))
			} else {
				r = append(r, "分镜：缺")
			}
			r = append(r, fmt.Sprintf("风格：%s", map[bool]string{true: "已确认", false: "未确认"}[s.StyleConfirmed]))
			if s.HasManuscript && s.HasStoryboard && s.StyleConfirmed {
				r = append(r, "结论：三前置齐备，可以开工")
			} else {
				r = append(r, "结论：前置未齐，不能开工（代码强制）")
			}
			return strings.Join(r, "；")
		},
	},
	{
		def: toolDef("start_production", "启动制作管线（TTS→画面→检查→渲染，全程自动，进度实时播报）", nil),
		gate: func(s State) string {
			var missing []string
			if !s.HasManuscript {
				missing = append(missing, "文稿")
			}
			if !s.HasStoryboard {
				missing = append(missing, "分镜")
			}
			if !s.HasStyle || !s.StyleConfirmed {
				missing = append(missing, "风格确认")
			}
			if s.Producing {
				return "制作已在运行中"
			}
			if s.HasVideo {
				return "成片已就绪，无需重新制作（要改画面/重做某段用 rework；要下载地址用 export）"
			}
			if len(missing) > 0 {
				return "前置未齐（缺 " + strings.Join(missing, "、") + "），先补齐"
			}
			return ""
		},
		run: func(a *Agent, p *pipeline.Project, _ string) string {
			ok, err := a.Producer.Start(p.ID)
			if err != nil {
				return "启动失败：" + err.Error()
			}
			if !ok {
				return "制作已在运行中"
			}
			a.Emit(p.ID, "stage", "pipeline", "running")
			return "制作已启动：配音 → 画面 → 布局检查 → 渲染，进度我随时报"
		},
	},
	{
		def: toolDef("rework", "重做某段画面（修改单元=段；元素/文案/布局的改动都整段重生成画面，音频不动，改完整片重渲）。改旁白请走 update_storyboard 重新制作", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"segment":     map[string]any{"type": "string", "description": "目标段：段号（如 3）或段 id（如 seg03）——从用户的 📎 引用或话里解析"},
				"instruction": map[string]any{"type": "string", "description": "画面修改要求（改什么元素、怎么改）"},
			},
			"required": []string{"segment", "instruction"},
		}),
		gate: func(s State) string {
			if s.Producing {
				return "制作/重做运行中，先等完成"
			}
			if !s.HasVideo {
				return "成片未就绪，先制作（首做走 start_production）"
			}
			return ""
		},
		run: func(a *Agent, p *pipeline.Project, args string) string {
			var in struct {
				Segment     string `json:"segment"`
				Instruction string `json:"instruction"`
			}
			_ = json.Unmarshal([]byte(args), &in)
			if strings.TrimSpace(in.Instruction) == "" {
				return "缺少修改要求"
			}
			segID, label, err := a.resolveSegment(p, in.Segment)
			if err != nil {
				return err.Error()
			}
			ok, err := a.Producer.Rework(p.ID, segID, in.Instruction)
			if err != nil {
				return "重做启动失败：" + err.Error()
			}
			if !ok {
				return "制作/重做已在运行中"
			}
			a.Emit(p.ID, "stage", "pipeline", "rework")
			return fmt.Sprintf("段级重做已启动（%s）：只重生成该段画面 → 检查 → 整片重渲，进度随时报", label)
		},
	},
	{
		def: toolDef("extract_stylepack", "把本片风格提炼成方法库 StylePack 草稿（成片出片后自动做，用户在首页确认入库后可复用）", nil),
		gate: func(s State) string {
			if !s.HasVideo {
				return "成片未就绪，出片后再提炼"
			}
			return ""
		},
		run: func(a *Agent, p *pipeline.Project, _ string) string {
			pack, err := a.Lib.DraftFromProject(p.Dir, p.ID, a.State(p.ID).Name)
			if err != nil {
				return "提炼失败：" + err.Error()
			}
			if pack.Published {
				return fmt.Sprintf("「%s」已在方法库（已发布）", pack.Name)
			}
			return fmt.Sprintf("风格包「%s」已起草（草稿）。用户在首页方法库点「入库」确认后，之后新建项目可直接复用", pack.Name)
		},
	},
	{
		def: toolDef("read_project", "查看项目当前状态与产物", nil),
		gate: noGate,
		run: func(a *Agent, p *pipeline.Project, _ string) string { return a.State(p.ID).summary() },
	},
	{
		def: toolDef("search_method_library", "检索方法库（StylePack：可复用风格）", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
			"required": []string{"query"},
		}),
		gate: noGate,
		run: func(a *Agent, p *pipeline.Project, args string) string {
			packs, err := a.Lib.List()
			if err != nil {
				return "方法库读取失败：" + err.Error()
			}
			var lines []string
			for _, pk := range packs {
				state := map[bool]string{true: "已发布", false: "草稿待入库"}[pk.Published]
				lines = append(lines, fmt.Sprintf("- %s（%s，来源 %s）", pk.Name, state, pk.OriginProject))
			}
			if len(lines) == 0 {
				return "方法库为空（冷启动零预置）：出片后我会自动起草风格包，用户确认入库"
			}
			return "方法库现有：\n" + strings.Join(lines, "\n")
		},
	},
	{
		def: toolDef("export", "导出成片（返回下载地址）", nil),
		gate: func(s State) string {
			if !s.HasVideo {
				return "成片未就绪"
			}
			return ""
		},
		run: func(a *Agent, p *pipeline.Project, _ string) string {
			return "成片下载地址：/api/projects/" + p.ID + "/video/main.mp4（1080p，含旁白与逐词字幕）"
		},
	},
}

func toolDef(name, desc string, params map[string]any) openai.Tool {
	if params == nil {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return openai.Tool{Type: openai.ToolTypeFunction, Function: &openai.FunctionDefinition{
		Name: name, Description: desc, Parameters: params,
	}}
}

func toolSchemas() []openai.Tool {
	out := make([]openai.Tool, len(tools))
	for i, t := range tools {
		out[i] = t.def
	}
	return out
}

func findTool(name string) *tool {
	for i := range tools {
		if tools[i].def.Function.Name == name {
			return &tools[i]
		}
	}
	return nil
}

// ── ReAct 循环 ───────────────────────────────────────────────

const maxSteps = 8

func (a *Agent) systemPrompt(id string, s State) string {
	return fmt.Sprintf(`你是「帧述」的主管 Agent——靠谱的剪辑搭档：懂行、话少、主动汇报、给专业建议但尊重用户决定。

当前项目「%s」（ID: %s）状态（唯一事实源，说话前先对照它）：
%s

工作准则：
- 上面这份状态是唯一事实：状态说成片就绪就是已就绪，直接给下载地址 /api/projects/%s/video/main.mp4；不确定就先 read_project 核对，禁止按聊天历史想象状态。
- 三前置硬门：文稿、分镜、风格样张（用户确认）——齐了才能 start_production，代码强制，别硬闯；成片就绪后不再 start_production。
- 成片就绪后：改某段画面用 rework（整段重生成画面、音频不动）；用户带 📎 段/元素引用的消息几乎都是 rework 意图。元素引用指名了改哪个元素，rework 的 instruction 里点名它（人话名，必要时带元素 id 与时刻）。出片后主动 extract_stylepack 沉淀风格（一次就够，已提炼过不必重复）。
- 默认自主连贯：能做的直接做（出分镜→出样张→自检一路做下去），到用户门（风格确认）停下说清楚等什么。
- 工具被拒就换路或向用户解释，不重复硬试；每轮最多 %d 步。
- 回复短：一段话讲清做了什么、下一步是什么，不堆术语、不复述参数。`, s.Name, id, s.summary(), id, maxSteps)
}

// Run 一轮对话：userText 与 refs 已入库。Agent 循环产出最终回复入库并广播。
func (a *Agent) Run(projectID, userText string, refs []store.Ref) {
	p := pipeline.NewProject(a.DataDir, projectID)
	prov, err := llm.FromEnv(llm.RoleDialogue)
	if err != nil {
		a.finish(projectID, "当前未配置 LLM API（LLM_API_KEY），我只能看不能想。配好 .env 再聊。", "error")
		return
	}
	msgs := []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleSystem, Content: a.systemPrompt(projectID, a.State(projectID))}}
	for _, h := range a.history(projectID, 10) {
		msgs = append(msgs, h)
	}
	// 实时状态随最新 user 消息注入（模型对最新消息权重最高，防聊天历史锚定幻觉）
	msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content:
		fmt.Sprintf("［系统·实时项目状态，以此为准］\n%s\n%s\n用户消息：%s",
			a.State(projectID).summary(), a.formatRefs(p, refs), userText)})

	final := ""
	for step := 0; step < maxSteps; step++ {
		msg, _, cerr := prov.Chat(context.Background(), msgs, toolSchemas())
		if cerr != nil {
			a.finish(projectID, "模型调用失败："+cerr.Error(), "error")
			return
		}
		if len(msg.ToolCalls) == 0 {
			final = msg.Content
			break
		}
		msgs = append(msgs, msg)
		for _, call := range msg.ToolCalls {
			obs := a.execTool(p, call.Function.Name, call.Function.Arguments)
			msgs = append(msgs, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    obs,
				ToolCallID: call.ID,
				Name:       call.Function.Name,
			})
		}
	}
	if final == "" {
		final = "我先停一下：这轮动作做完了，等你的下一步指示。"
	}
	a.finish(projectID, final, "text")
}

// formatRefs 引用卡片 → 模型可读的引用块（空则空串，不占位）。
// 时刻引用换算段内偏移（口径与 MP4 拼接一致：配音时长+0.35 尾垫），模型能精确理解"这里"。
func (a *Agent) formatRefs(p *pipeline.Project, refs []store.Ref) string {
	if len(refs) == 0 {
		return ""
	}
	starts := segStarts(p)
	var b strings.Builder
	b.WriteString("用户引用（点选自舞台，精确上下文，优先于口头描述）：\n")
	for _, r := range refs {
		line := fmt.Sprintf("- 段%d", r.Idx)
		if r.Key != "" {
			line += fmt.Sprintf("「%s」", r.Key)
		}
		if r.T > 0 {
			line += fmt.Sprintf(" · 全片 %.1fs（第 %d 帧）", r.T, int(r.T*30+0.5))
			if start, ok := starts[r.Idx]; ok && r.T >= start {
				line += fmt.Sprintf("，该段第 %.1fs", r.T-start)
			}
		}
		if r.ElementName != "" {
			line += fmt.Sprintf(" · 元素「%s」", r.ElementName)
			if r.ElementID != "" {
				line += fmt.Sprintf("（%s）", r.ElementID)
			}
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// segStarts 段起点表：audio duration_s + 0.35 尾垫累计（与 ai/assemble.py、前端 segtime.ts 同口径）；
// 无音频对齐产物时退回分镜 duration_hint。
func segStarts(p *pipeline.Project) map[int]float64 {
	sb, err := p.LoadStoryboard()
	if err != nil {
		return nil
	}
	durs := map[string]float64{}
	var meta struct {
		Voices []struct {
			ID        string  `json:"id"`
			DurationS float64 `json:"duration_s"`
		} `json:"voices"`
	}
	if b, rerr := os.ReadFile(p.Artifact("audio_meta.json")); rerr == nil && json.Unmarshal(b, &meta) == nil {
		for _, v := range meta.Voices {
			durs[v.ID] = v.DurationS + 0.35
		}
	}
	starts := map[int]float64{}
	acc := 0.0
	for _, seg := range sb.Segments {
		starts[seg.Idx] = acc
		if d, ok := durs[seg.ID]; ok && d > 0 {
			acc += d
		} else {
			acc += seg.DurationHint
		}
	}
	return starts
}

// execTool 门禁 + 执行 + 记账（工具轨迹入聊天案卷）。
func (a *Agent) execTool(p *pipeline.Project, name, args string) string {
	t := findTool(name)
	if t == nil {
		return "未知工具 " + name
	}
	s := a.State(p.ID)
	if reason := t.gate(s); reason != "" {
		obs := "拒绝：" + reason
		a.saveToolMsg(p.ID, name, args, obs)
		a.Emit(p.ID, "tool_result", "", name+" "+obs)
		return obs
	}
	a.Emit(p.ID, "tool_call", "", name+" "+compactArgs(args))
	obs := t.run(a, p, args)
	a.saveToolMsg(p.ID, name, args, obs)
	a.Emit(p.ID, "tool_result", "", name+" → "+firstLine(obs, 120))
	return obs
}

func (a *Agent) finish(projectID, content, typ string) {
	if typ == "error" {
		_ = a.Store.SaveMsg(&store.Msg{ProjectID: projectID, Role: "agent", Type: "error", Content: content})
	} else {
		_ = a.Store.SaveMsg(&store.Msg{ProjectID: projectID, Role: "agent", Type: "text", Content: content})
	}
	a.Emit(projectID, "agent_msg", "", content)
}

func (a *Agent) saveToolMsg(projectID, name, args, result string) {
	_ = a.Store.SaveMsg(&store.Msg{
		ProjectID: projectID, Role: "agent", Type: "tool_call",
		Content: name, ToolName: name, ToolArgs: args, ToolResult: result,
	})
}

func (a *Agent) history(projectID string, n int) []openai.ChatCompletionMessage {
	msgs, err := a.Store.ListMsgs(projectID, 0, 100)
	if err != nil {
		return nil
	}
	var hist []openai.ChatCompletionMessage
	for _, m := range msgs {
		if m.Type != "text" || m.Role == "system" {
			continue
		}
		role := openai.ChatMessageRoleUser
		if m.Role == "agent" {
			role = openai.ChatMessageRoleAssistant
		}
		hist = append(hist, openai.ChatCompletionMessage{Role: role, Content: m.Content})
	}
	if len(hist) > n {
		hist = hist[len(hist)-n:]
	}
	return hist
}

func compactArgs(args string) string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) == nil {
		b, _ := json.Marshal(m)
		return string(b)
	}
	return args
}

func firstLine(s string, n int) string {
	s = strings.SplitN(s, "\n", 2)[0]
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
