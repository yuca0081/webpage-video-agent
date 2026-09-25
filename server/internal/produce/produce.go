// Package produce：制作任务运行器（确定性编排，LLM 只在需要的点上介入）。
//
// 链路：tts（python）→ 每段语义 spec（DeepSeek + 契约校验 + 修复一轮）
//   → spec 渲染 HTML（python render_spec）→ 管线落盘合成物 → 组装（python assemble）
//   → check 门禁（失败带着报错重生成 spec 自修复，最多 2 轮）→ 渲染 → 成片。
// 每步 SSE 广播 stage 事件；产物落盘可从任意步重放（manifest 记账）。
package produce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"webpage-video-agent/server/internal/contract"
	"webpage-video-agent/server/internal/events"
	"webpage-video-agent/server/internal/llm"
	"webpage-video-agent/server/internal/pipeline"
)

type Runner struct {
	DataDir string // 仓库根 data/（项目在 data/projects/<id>）
	RootDir string // 仓库根（ai/ 脚本所在）
	Hub     *events.Hub
	OnDone  func(projectID, status string) // 状态回写（DB + project.json）
}

var (
	mu      sync.Mutex
	running = map[string]context.CancelFunc{}
)

// Start 启动制作（同项目幂等：已在跑则返回 false）。
func (r *Runner) Start(projectID string) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	if _, live := running[projectID]; live {
		return false, nil
	}
	p := pipeline.NewProject(r.DataDir, projectID)
	if _, err := os.Stat(p.Dir); err != nil {
		return false, fmt.Errorf("项目不存在: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	running[projectID] = cancel
	go func() {
		defer func() {
			mu.Lock()
			delete(running, projectID)
			mu.Unlock()
		}()
		err := r.run(ctx, p)
		if errors.Is(err, context.Canceled) {
			r.emit(projectID, "stage", "pipeline", "cancelled")
			r.emit(projectID, "cancelled", "", "用户取消制作")
			r.Manifest(p, "produce.cancelled", "")
			if r.OnDone != nil {
				r.OnDone(projectID, "cancelled")
			}
			return
		}
		if err != nil {
			r.emit(projectID, "stage", "pipeline", "error: "+err.Error())
			r.emit(projectID, "error", "", err.Error())
			r.Manifest(p, "produce.error", err.Error())
			if r.OnDone != nil {
				r.OnDone(projectID, "failed")
			}
		}
	}()
	return true, nil
}

// Cancel 请求取消进行中的制作/重做（协作式：杀子进程 + 阶段边界退出）。
// 返回 false = 该项目没有在跑的任务。
func (r *Runner) Cancel(projectID string) bool {
	mu.Lock()
	cancel, live := running[projectID]
	mu.Unlock()
	if !live {
		return false
	}
	cancel()
	return true
}

// Rework 段级重做（plan.md §3.5：修改单元可到元素，重做单元=段）。
// 只重生成该段画面（音频/时长不变）→ 组装 → 检查 → 整片重渲拼接。
func (r *Runner) Rework(projectID, segID, instruction string) (bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	mu.Lock()
	if _, live := running[projectID]; live {
		mu.Unlock()
		cancel()
		return false, nil
	}
	running[projectID] = cancel
	mu.Unlock()
	p := pipeline.NewProject(r.DataDir, projectID)
	sb, err := p.LoadStoryboard()
	if err != nil {
		cancel()
		mu.Lock()
		delete(running, projectID)
		mu.Unlock()
		return false, fmt.Errorf("分镜未生成: %w", err)
	}
	seg, found := findSegment(sb, segID)
	if !found {
		cancel()
		mu.Lock()
		delete(running, projectID)
		mu.Unlock()
		return false, fmt.Errorf("分镜里没有段 %s", segID)
	}
	go func() {
		defer func() {
			mu.Lock()
			delete(running, projectID)
			mu.Unlock()
		}()
		err := r.runRework(ctx, p, seg, instruction)
		if errors.Is(err, context.Canceled) {
			r.emit(projectID, "stage", "pipeline", "cancelled")
			r.emit(projectID, "cancelled", "", "用户取消重做")
			r.Manifest(p, "rework.cancelled", seg.ID)
			if r.OnDone != nil {
				r.OnDone(projectID, "cancelled")
			}
			return
		}
		if err != nil {
			r.emit(projectID, "stage", "pipeline", "error: "+err.Error())
			r.emit(projectID, "error", "", err.Error())
			r.Manifest(p, "rework.error", seg.ID+": "+err.Error())
			if r.OnDone != nil {
				r.OnDone(projectID, "failed")
			}
		}
	}()
	return true, nil
}

func findSegment(sb *contract.Storyboard, segID string) (contract.Segment, bool) {
	for _, s := range sb.Segments {
		if s.ID == segID {
			return s, true
		}
	}
	return contract.Segment{}, false
}

// runRework 单段重做管线。旁白/音频不动；instruction 作为画面改写要求回喂。
func (r *Runner) runRework(ctx context.Context, p *pipeline.Project, seg contract.Segment, instruction string) error {
	id := p.ID
	r.Manifest(p, "rework.start", fmt.Sprintf("%s: %s", seg.ID, firstLine(instruction, 80)))
	r.emit(id, "progress", "rework", fmt.Sprintf("重做 段%d「%s」", seg.Idx, seg.Key))

	// 1. 该段画面重生成（删旧 spec 强制重出；其余段产物原样复用）
	r.emit(id, "stage", "compositions", "running")
	// 用户改画要求走 instruction 位（最高优先级），修复轮（check/画面审查）也必须带上
	instr := "用户对这一段画面的修改要求（必须落实）：\n" + instruction
	if err := r.genSpecs(ctx, p, instr, "", seg.ID); err != nil {
		return err
	}
	if err := r.fetchImages(ctx, p); err != nil {
		return err
	}
	if err := r.renderSpecs(ctx, p); err != nil {
		return err
	}
	r.clearFrames(p, seg.ID)
	if err := pipeline.Run(ctx, p, "compositions"); err != nil {
		return fmt.Errorf("合成物落盘失败: %w", err)
	}
	r.emit(id, "stage", "compositions", "done")

	// 2. 重组装（时间轴不变）+ 检查（失败只修这一段）
	r.emit(id, "stage", "assemble", "running")
	if err := r.python(ctx, "ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
		return fmt.Errorf("组装失败: %w", err)
	}
	r.emit(id, "stage", "assemble", "done")
	if err := r.checkWithRepair(ctx, p, instr, seg.ID); err != nil {
		return err
	}

	// 3. 段级局部重渲：只删/重渲该段段片，其余段复用，再 concat 出成片
	r.emit(id, "stage", "render", "running")
	if err := os.Remove(p.Artifact("renders/segs/" + seg.ID + ".mp4")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := pipeline.Run(ctx, p, "render"); err != nil {
		return fmt.Errorf("渲染失败: %w", err)
	}
	if err := pipeline.Run(ctx, p, "stitch"); err != nil {
		return err
	}
	r.emit(id, "stage", "render", "done")

	// 4. 画面审查：只审重做段，不过带要求重出（用户要求经 instr 一路带到修复轮）
	if err := r.frameQA(ctx, p, instr, seg.ID); err != nil {
		return err
	}
	r.emit(id, "done", "", "renders/main.mp4")
	r.Manifest(p, "rework.done", seg.ID)
	if r.OnDone != nil {
		r.OnDone(id, "video")
	}
	return nil
}

func (r *Runner) IsRunning(projectID string) bool {
	mu.Lock()
	defer mu.Unlock()
	_, live := running[projectID]
	return live
}

func (r *Runner) emit(id, typ, stage, detail string) {
	r.Hub.Emit(id, typ, stage, detail)
}

func (r *Runner) Manifest(p *pipeline.Project, event, detail string) { p.Manifest(event, detail) }

func (r *Runner) run(ctx context.Context, p *pipeline.Project) error {
	id := p.ID

	// ── 1. TTS + 词级对齐（幂等：缺什么补什么）─────────────────
	r.emit(id, "stage", "tts", "running")
	if err := r.python(ctx, "ai/tts_align.py", p.Dir, 30*time.Minute); err != nil {
		return fmt.Errorf("TTS/对齐失败: %w", err)
	}
	r.emit(id, "stage", "tts", "done")
	r.Manifest(p, "produce.tts", "done")

	// ── 2. 语义 spec 生成（LLM）+ 确定性渲染 + 合成物落盘 ────────
	if err := r.compositions(ctx, p); err != nil {
		return err
	}

	// ── 3. 组装 ─────────────────────────────────────────────
	r.emit(id, "stage", "assemble", "running")
	if err := r.python(ctx, "ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
		return fmt.Errorf("组装失败: %w", err)
	}
	r.emit(id, "stage", "assemble", "done")

	// ── 4. check 门禁（自修复：报错回喂重生成 spec）────────────
	if err := r.checkWithRepair(ctx, p, "", ""); err != nil {
		return err
	}

	// ── 5. 渲染成片（按段增量渲 + concat，段片复用见 stageRender）──
	r.emit(id, "stage", "render", "running")
	if err := pipeline.Run(ctx, p, "render"); err != nil {
		return fmt.Errorf("渲染失败: %w", err)
	}
	if err := pipeline.Run(ctx, p, "stitch"); err != nil {
		return err
	}
	r.emit(id, "stage", "render", "done")

	// ── 6. 画面审查（抽帧+视觉模型，自修复见 frameQA）────────────
	if err := r.frameQA(ctx, p, "", ""); err != nil {
		return err
	}
	r.emit(id, "done", "", "renders/main.mp4")
	r.Manifest(p, "produce.done", "renders/main.mp4")
	if r.OnDone != nil {
		r.OnDone(id, "video")
	}
	return nil
}

// compositions 生成/复用 spec → 搜图本地化 → 渲染 HTML → 清旧帧 → 管线落盘。
func (r *Runner) compositions(ctx context.Context, p *pipeline.Project) error {
	r.emit(p.ID, "stage", "compositions", "running")
	if err := r.genSpecs(ctx, p, "", "", ""); err != nil {
		return err
	}
	if err := r.fetchImages(ctx, p); err != nil {
		return err
	}
	if err := r.renderSpecs(ctx, p); err != nil {
		return err
	}
	r.clearFrames(p, "")
	if err := pipeline.Run(ctx, p, "compositions"); err != nil {
		return fmt.Errorf("合成物落盘失败: %w", err)
	}
	r.emit(p.ID, "stage", "compositions", "done")
	return nil
}

// checkWithRepair check 失败 → 报错回喂 → 重生成 spec → 重渲染 → 重查（最多 3 轮，plan §4.5）。
// instruction 原样传给 genSpecs 的用户要求位（rework 时不能丢）。
// only 非空 = 修复范围限定该段（rework），空 = 全片（首次制作）。
func (r *Runner) checkWithRepair(ctx context.Context, p *pipeline.Project, instruction, only string) error {
	for attempt := 1; attempt <= 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.emit(p.ID, "stage", "check", fmt.Sprintf("running（第 %d 次）", attempt))
		_ = os.Remove(p.Artifact(".hyperframes-ok"))
		err := pipeline.Run(ctx, p, "check")
		if err == nil {
			r.emit(p.ID, "stage", "check", "done")
			return nil
		}
		if ctx.Err() != nil { // 用户取消 ≠ check 失败，别进修复轮
			return ctx.Err()
		}
		if attempt == 3 {
			r.emit(p.ID, "stage", "check", "error: "+err.Error())
			return fmt.Errorf("check 三轮未过: %w", err)
		}
		feedback := "上一版画面被自动布局检查拒绝，报错摘要：\n" + tail(err.Error(), 25) +
			"\n常见原因：元素重叠/越界。请整体重排：拉开间距、避开底部字幕带（禁放区见布局规则）、必要时减少元素。"
		r.emit(p.ID, "stage", "check", "repair：带着报错重生成画面")
		r.Manifest(p, "check.repair", tail(err.Error(), 10))
		if err := r.genSpecs(ctx, p, instruction, feedback, only); err != nil {
			return err
		}
		if err := r.fetchImages(ctx, p); err != nil {
			return err
		}
		if err := r.renderSpecs(ctx, p); err != nil {
			return err
		}
		r.clearFrames(p, only)
		if err := pipeline.Run(ctx, p, "compositions"); err != nil {
			return fmt.Errorf("合成物落盘失败: %w", err)
		}
		if err := r.python(ctx, "ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
			return fmt.Errorf("组装失败: %w", err)
		}
	}
	return errors.New("unreachable")
}

// ── spec 生成（LLM 作业：语义布局）─────────────────────────────

type voiceMeta struct {
	ID        string        `json:"id"`
	DurationS float64       `json:"duration_s"`
	Words     []wordTiming  `json:"words"`
}

type wordTiming struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End    float64 `json:"end"`
}

func loadAudioMeta(p *pipeline.Project) (map[string]voiceMeta, error) {
	b, err := os.ReadFile(p.Artifact("audio_meta.json"))
	if err != nil {
		return nil, fmt.Errorf("缺 audio_meta.json: %w", err)
	}
	var m struct {
		Voices []voiceMeta `json:"voices"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	out := map[string]voiceMeta{}
	for _, v := range m.Voices {
		out[v.ID] = v
	}
	return out, nil
}

// genSpecs 每段一份语义 spec（llm/comp-segNN.spec.json）。
// instruction = 用户改画要求（最高优先级位，修复轮必须带上否则会丢）；
// feedback = 校验/审查报错回喂（必须修正位）。only 非空 = 只处理该段（rework），空 = 全片。
func (r *Runner) genSpecs(ctx context.Context, p *pipeline.Project, instruction, feedback, only string) error {
	sb, err := p.LoadStoryboard()
	if err != nil {
		return fmt.Errorf("先完成 storyboard: %w", err)
	}
	voices, err := loadAudioMeta(p)
	if err != nil {
		return err
	}
	prov, perr := llm.FromEnv(llm.RoleVisual)
	if perr != nil {
		return fmt.Errorf("spec 生成需要 API 模式（LLM_API_KEY）: %w", perr)
	}
	cv := ProjectCanvas(p)
	system := specSystemFor(p)
	style := styleNoteFor(p, r.RootDir)
	icons := scanIcons(r.RootDir)
	menu, kinds, err := elementRegistry(r.RootDir)
	if err != nil {
		return err
	}
	if feedback != "" && only == "" { // 全片重生成：先清旧 spec
		for _, seg := range sb.Segments {
			_ = os.Remove(p.Artifact("llm/comp-" + seg.ID + ".spec.json"))
		}
	}
	for _, seg := range sb.Segments {
		if only != "" && seg.ID != only {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		specPath := p.Artifact("llm/comp-" + seg.ID + ".spec.json")
		if _, err := os.Stat(specPath); err == nil && feedback == "" && instruction == "" {
			continue // 已有产物，跳过（重放语义）
		}
		v := voices[seg.ID]
		var spec contract.CompSpec
		u1, err := prov.GenerateJSON(ctx, system, specUserPrompt(seg, v, instruction, feedback, cv, style, menu), &spec)
		if err != nil {
			return fmt.Errorf("%s spec 生成失败: %w", seg.ID, err)
		}
		errs := contract.ValidateSpec(&spec, len(v.Words), cv)
		errs = append(errs, iconErrors(&spec, icons)...)
		for repair := 0; repair < 2 && len(errs) > 0; repair++ { // 修复：违规项回喂，最多 2 轮
			fb := instruction + feedback + "\n上一版 spec 被契约校验拒绝，必须逐条修正：\n- " + strings.Join(errs, "\n- ")
			var fixed contract.CompSpec
			u2, err2 := prov.GenerateJSON(ctx, system, specUserPrompt(seg, v, "", fb, cv, style, menu), &fixed)
			if err2 != nil {
				return fmt.Errorf("%s spec 修复失败: %w", seg.ID, err2)
			}
			u1, spec = u2, fixed
			errs = contract.ValidateSpec(&spec, len(v.Words), cv)
			errs = append(errs, iconErrors(&spec, icons)...)
		}
		if len(errs) > 0 { // 保底：确定性清洗（删未知 kind/删冲突/夹越界），不让整条任务死掉
			remain := contract.SanitizeSpec(&spec, len(v.Words), cv)
			// 清洗后元素所剩无几 = 该段画面已不成立（如模型整段漏填 kind）：
			// 带着明确要求整段重出一次，仍不行才报错——空白段比失败更隐蔽。
			if len(spec.Elements) < 3 {
				r.emit(p.ID, "progress", "compositions", fmt.Sprintf("%s 清洗后仅 %d 元素，整段重出", seg.ID, len(spec.Elements)))
				var retry contract.CompSpec
				u2, err2 := prov.GenerateJSON(ctx, system,
					specUserPrompt(seg, v, instruction,
						fmt.Sprintf("上一版几乎每个元素的 kind 都是空的或不认识的。kind 必须从可用元素菜单里逐字选取（%s），每个元素都必须有 kind。", kinds),
						cv, style, menu), &retry)
				if err2 == nil {
					errs2 := contract.ValidateSpec(&retry, len(v.Words), cv)
					errs2 = append(errs2, iconErrors(&retry, icons)...)
					if len(errs2) > 0 {
						contract.SanitizeSpec(&retry, len(v.Words), cv)
					}
					if len(retry.Elements) >= 3 {
						u1, spec = u2, retry
						errs = nil
					}
				}
			}
			if len(errs) > 0 {
				p.Manifest("spec.sanitized", fmt.Sprintf("%s: 清洗后剩余 %d 元素；遗留问题: %s", seg.ID, len(spec.Elements), strings.Join(remain, "; ")))
			}
		}
		if len(spec.Elements) < 3 {
			return fmt.Errorf("%s spec 元素不足 3 个（模型输出异常，重试后仍失败）", seg.ID)
		}
		b, _ := json.MarshalIndent(spec, "", "  ")
		if err := os.WriteFile(specPath, b, 0o644); err != nil {
			return err
		}
		p.Manifest("llm.usage", fmt.Sprintf("spec %s prompt=%d completion=%d tokens", seg.ID, u1.PromptTokens, u1.CompletionTokens))
		r.emit(p.ID, "progress", "compositions", fmt.Sprintf("%s 画面 spec ✓（%d 元素）", seg.ID, len(spec.Elements)))
	}
	return nil
}

const specSystemFallback = "你是科普视频的画面设计师，只输出 JSON。风格：手绘叙事（纸面·马克笔）——米黄纸面、深灰线稿、彩色便签、楷体。"

// specSystemFor 系统提示跟随项目已确认的风格方向（方法库复用时随包走）。
func specSystemFor(p *pipeline.Project) string {
	var ss contract.StyleSamples
	if b, err := os.ReadFile(p.Artifact("style/style_samples.json")); err == nil && json.Unmarshal(b, &ss) == nil && ss.Direction != "" {
		return "你是科普视频的画面设计师，只输出 JSON。风格：" + ss.Direction + "。"
	}
	return specSystemFallback
}

// ProjectCanvas 项目画幅（project.json aspect；旧项目无字段按 16:9，与已渲成片一致）。
func ProjectCanvas(p *pipeline.Project) contract.Canvas {
	b, err := os.ReadFile(p.Artifact("project.json"))
	if err != nil {
		return contract.CanvasFor("16:9")
	}
	var meta struct {
		Aspect string `json:"aspect"`
	}
	_ = json.Unmarshal(b, &meta)
	if meta.Aspect == "" {
		meta.Aspect = "16:9"
	}
	return contract.CanvasFor(meta.Aspect)
}

// scanIcons 本地图标库清单（ai/assets/icons，lucide 全量）。
func scanIcons(rootDir string) map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(rootDir, "ai", "assets", "icons"))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if name := strings.TrimSuffix(e.Name(), ".svg"); name != e.Name() {
			out[name] = true
		}
	}
	return out
}

// iconErrors 图标名不在本地库 → 校验错误（回喂换名；渲染层另有兜底）。
func iconErrors(s *contract.CompSpec, icons map[string]bool) []string {
	if len(icons) == 0 {
		return nil
	}
	var errs []string
	for i := range s.Elements {
		e := &s.Elements[i]
		if e.Kind == "icon" && e.Name != "" && !icons[e.Name] {
			errs = append(errs, fmt.Sprintf("元素%d(icon): name %q 不在本地图标库，换一个", i+1, e.Name))
		}
	}
	return errs
}

func specUserPrompt(seg contract.Segment, v voiceMeta, instruction, feedback string, cv contract.Canvas, style, menu string) string {
	var wb strings.Builder
	for i, w := range v.Words {
		if i > 0 && i%10 == 0 {
			wb.WriteString("\n")
		}
		fmt.Fprintf(&wb, "%d:%s@%.1fs ", i, w.Text, w.Start)
	}
	// 布局规则随画幅：竖屏纵向铺排，横屏横向铺排
	rules := "- 安全区 x∈[160,900]、y∈[120,1450]；底部 1594px 起是字幕带，禁放\n" +
		"- 竖屏 1080 宽：元素横向少列（≤2 列），纵向多行铺排；主视觉放 y≈300–1200\n" +
		"- 4–7 个元素；元素间距保守留 80px，禁止重叠"
	if cv.Aspect == "16:9" {
		rules = "- 安全区 x∈[160,1760]、y∈[120,850]；底部 896px 起是字幕带，禁放\n" +
			"- 4–7 个元素；主视觉放 y≈260–560 一带；元素间距保守留 80px，禁止重叠"
	}
	instrBlock := ""
	briefLabel := "画面提示"
	if instruction != "" {
		instrBlock = "\n## 用户修改要求（最高优先级——这是重画指令，与下方原画面提示冲突时一律以用户要求为准，逐字落实）\n" + instruction + "\n"
		briefLabel = "原画面提示（旧版，仅参考）"
	}
	return fmt.Sprintf(`为这一段视频生成语义布局 spec JSON。
%s
## 段信息
- 标题：%s
- 旁白（%d 词，%.1f 秒）：%s
- %s：%s
%s
## 可用元素（kind 与参数；坐标基于 %d×%d 画布%s）
%s

## 可用图标名（节选，语义匹配优先；必须是列表或其近似的名字）
rocket cat dog sun moon star atom brain heart zap cloud flame droplet eye bone dna
battery lightbulb cog wrench search book pen mail clock thermometer coins gift lock
key phone laptop car plane house tree-deciduous tree-pine sprout flower fish bird
coffee music camera mic wifi shield flag target users user map compass calendar
hourglass microscope flask-conical test-tube stethoscope pill syringe leaf mountain
waves wind snowflake umbrella globe graduation-cap calculator cpu database server
satellite antenna fuel bug virus magnet telescope orbit trending-up trending-down
cloud-rain-wind cloud-sun robot-arm baby person-circle-stop dices trophy medal crown
scale ruler clipboard lightbulb-off zap-off anchor truck bike train bus ship send

## 词序表（reveal = 揭示时刻的词序号，0 起）
%s

## 布局硬规则（校验器会拒收）
%s
- reveal 按讲解顺序递增、铺满词序（别堆在开头；最大词号 %d）
- 语义呼应画面提示：主体物→image（实体名词首选）或 icon（抽象概念）；数据对比→chart_bar；趋势→chart_line；占比→chart_donut 或 chart_pie；对比→双色便签左右分置或 table；流程/步骤→timeline 或箭头串联；要点/卖点→checklist；金句/名言→quote；关键数字→stat 或 big；指向→barrow；向量/维度/批量→strip；分组圈注→zone(+bracket)；一屏最多一个图表（图表占主视觉位）
- 画面丰富度（重要）：每屏至少一个视觉锚点（image / 大 icon / 图表 / big / panel 之一），大小拉开层次（主体 300px+、次级 120–200px），禁止全屏小元素平铺
%s

## 输出（只输出 JSON，无围栏）
{"note":"布局思路一句话","elements":[…]}
`, instrBlock, seg.Key, len(v.Words), v.DurationS, seg.Narration, briefLabel, seg.VisualBrief, style,
		int(cv.W), int(cv.H), map[bool]string{true: " 竖屏 9:16", false: ""}[cv.Aspect == "9:16"],
		menu,
		wb.String(), rules, len(v.Words)-1, feedbackBlock(feedback))
}

// elementRegistry 元素菜单与 kind 清单（ai/registry/elements.json，元素库单一事实源；
// spec.go 的 specKinds 由漂移测试 registry_test.go 强制对齐）。
// 菜单行拼进 spec prompt；kind 清单用于「整段 kind 全空重出」的修复提示。
func elementRegistry(rootDir string) (menu, kinds string, err error) {
	b, err := os.ReadFile(filepath.Join(rootDir, "ai", "registry", "elements.json"))
	if err != nil {
		return "", "", fmt.Errorf("元素注册表缺失（ai/registry/elements.json）: %w", err)
	}
	var els []struct {
		Kind string `json:"kind"`
		Menu string `json:"menu"`
	}
	if err := json.Unmarshal(b, &els); err != nil {
		return "", "", fmt.Errorf("elements.json 损坏: %w", err)
	}
	if len(els) == 0 {
		return "", "", fmt.Errorf("elements.json 为空")
	}
	lines := make([]string, 0, len(els))
	ks := make([]string, 0, len(els))
	for _, e := range els {
		if e.Kind == "" || e.Menu == "" {
			return "", "", fmt.Errorf("elements.json 存在空 kind/menu 条目")
		}
		lines = append(lines, "- "+e.Kind+"："+e.Menu)
		ks = append(ks, e.Kind)
	}
	return strings.Join(lines, "\n"), strings.Join(ks, "/"), nil
}

// styleNoteFor 项目风格方向 → spec 提示里的风格说明（题材 + 配图调性）。
// 方向在 ai/registry/styles.json 注册表命中时给出题材与配图 query 调性，未命中只给方向名。
func styleNoteFor(p *pipeline.Project, rootDir string) string {
	b, err := os.ReadFile(p.Artifact("style/style_samples.json"))
	if err != nil {
		return ""
	}
	var ss struct {
		Direction string `json:"direction"`
	}
	if json.Unmarshal(b, &ss) != nil || ss.Direction == "" {
		return ""
	}
	note := "\n## 本片风格\n- 方向：" + ss.Direction
	reg, err := os.ReadFile(filepath.Join(rootDir, "ai", "registry", "styles.json"))
	if err != nil {
		return note
	}
	var styles []struct {
		Direction string   `json:"direction"`
		Keywords  []string `json:"keywords"`
		Genre     string   `json:"genre"`
		Photo     string   `json:"photo"`
	}
	if json.Unmarshal(reg, &styles) != nil {
		return note
	}
	for _, st := range styles {
		hit := st.Direction != "" && strings.Contains(ss.Direction, st.Direction)
		if !hit {
			for _, kw := range st.Keywords {
				if kw != "" && strings.Contains(ss.Direction, kw) {
					hit = true
					break
				}
			}
		}
		if hit {
			if st.Genre != "" {
				note += "\n- 适用题材：" + st.Genre
			}
			if st.Photo != "" {
				note += "\n- 配图 query 调性：" + st.Photo + "（image 的 query 往这个调性上靠）"
			}
			break
		}
	}
	return note
}

func feedbackBlock(f string) string {
	if f == "" {
		return ""
	}
	return "\n## 必须修正\n" + f + "\n"
}

// ── 子进程与工具 ─────────────────────────────────────────────

// python 跑仓库 ai/ 脚本（cwd = 仓库根）。任务取消时连带杀掉子进程。
func (r *Runner) python(ctx context.Context, script string, projDir string, timeout time.Duration) error {
	py, err := exec.LookPath("python")
	if err != nil {
		return fmt.Errorf("python 不在 PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, py, filepath.Join(r.RootDir, script), projDir)
	cmd.Dir = r.RootDir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil { // 被取消/超时杀掉：报可识别的 ctx 错误而非 killed
			return ctx.Err()
		}
		return fmt.Errorf("%s 退出: %w\n%s", script, err, tail(buf.String(), 15))
	}
	fmt.Println(buf.String())
	return nil
}

// renderSpecs spec → llm/comp-segNN.json（确定性渲染层）。
func (r *Runner) renderSpecs(ctx context.Context, p *pipeline.Project) error {
	return r.python(ctx, "ai/render_spec.py", p.Dir, 2*time.Minute)
}

// fetchImages spec 里 image 元素的搜图本地化（必应 → Commons，失败便签兜底）。
func (r *Runner) fetchImages(ctx context.Context, p *pipeline.Project) error {
	return r.python(ctx, "ai/fetch_images.py", p.Dir, 10*time.Minute)
}

// clearFrames 清旧帧（落盘阶段只补缺失文件，不清不会重写）。
// only 非空只清该段；空清全部。注意不动 llm/comp-*.json——那是
// render_spec 的产物、compositions 落盘阶段的输入，删了会退化成等待人工。
func (r *Runner) clearFrames(p *pipeline.Project, only string) {
	if only != "" {
		_ = os.Remove(p.Artifact("compositions/frames/" + only + ".html"))
		_ = os.Remove(p.Artifact("compositions/registry/" + only + ".json"))
		return
	}
	files, _ := filepath.Glob(filepath.Join(p.Dir, "compositions", "frames", "*.html"))
	for _, f := range files {
		_ = os.Remove(f)
	}
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func firstLine(s string, n int) string {
	s = strings.SplitN(s, "\n", 2)[0]
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
