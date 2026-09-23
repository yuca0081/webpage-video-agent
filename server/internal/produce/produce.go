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
	running = map[string]bool{}
)

// Start 启动制作（同项目幂等：已在跑则返回 false）。
func (r *Runner) Start(projectID string) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	if running[projectID] {
		return false, nil
	}
	p := pipeline.NewProject(r.DataDir, projectID)
	if _, err := os.Stat(p.Dir); err != nil {
		return false, fmt.Errorf("项目不存在: %w", err)
	}
	running[projectID] = true
	go func() {
		defer func() {
			mu.Lock()
			delete(running, projectID)
			mu.Unlock()
		}()
		err := r.run(p)
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

// Rework 段级重做（plan.md §3.5：修改单元可到元素，重做单元=段）。
// 只重生成该段画面（音频/时长不变）→ 组装 → 检查 → 整片重渲拼接。
func (r *Runner) Rework(projectID, segID, instruction string) (bool, error) {
	mu.Lock()
	if running[projectID] {
		mu.Unlock()
		return false, nil
	}
	running[projectID] = true
	mu.Unlock()
	p := pipeline.NewProject(r.DataDir, projectID)
	sb, err := p.LoadStoryboard()
	if err != nil {
		mu.Lock()
		delete(running, projectID)
		mu.Unlock()
		return false, fmt.Errorf("分镜未生成: %w", err)
	}
	seg, found := findSegment(sb, segID)
	if !found {
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
		err := r.runRework(p, seg, instruction)
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
func (r *Runner) runRework(p *pipeline.Project, seg contract.Segment, instruction string) error {
	id := p.ID
	r.Manifest(p, "rework.start", fmt.Sprintf("%s: %s", seg.ID, firstLine(instruction, 80)))
	r.emit(id, "progress", "rework", fmt.Sprintf("重做 段%d「%s」", seg.Idx, seg.Key))

	// 1. 该段画面重生成（删旧 spec 强制重出；其余段产物原样复用）
	r.emit(id, "stage", "compositions", "running")
	feedback := "用户对这一段画面的修改要求（必须落实）：\n" + instruction
	if err := r.genSpecs(p, feedback, seg.ID); err != nil {
		return err
	}
	if err := r.renderSpecs(p); err != nil {
		return err
	}
	r.clearFrames(p, seg.ID)
	if err := pipeline.Run(p, "compositions"); err != nil {
		return fmt.Errorf("合成物落盘失败: %w", err)
	}
	r.emit(id, "stage", "compositions", "done")

	// 2. 重组装（时间轴不变）+ 检查（失败只修这一段）
	r.emit(id, "stage", "assemble", "running")
	if err := r.python("ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
		return fmt.Errorf("组装失败: %w", err)
	}
	r.emit(id, "stage", "assemble", "done")
	if err := r.checkWithRepair(p, seg.ID); err != nil {
		return err
	}

	// 3. 整片重渲（重渲单元=段；v1 渲染仍整条，段级并行是后续项）
	r.emit(id, "stage", "render", "running")
	if err := os.Remove(p.Artifact("renders/main.mp4")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := pipeline.Run(p, "render"); err != nil {
		return fmt.Errorf("渲染失败: %w", err)
	}
	if err := pipeline.Run(p, "stitch"); err != nil {
		return err
	}
	r.emit(id, "stage", "render", "done")
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
	return running[projectID]
}

func (r *Runner) emit(id, typ, stage, detail string) {
	r.Hub.Emit(id, typ, stage, detail)
}

func (r *Runner) Manifest(p *pipeline.Project, event, detail string) { p.Manifest(event, detail) }

func (r *Runner) run(p *pipeline.Project) error {
	id := p.ID

	// ── 1. TTS + 词级对齐（幂等：缺什么补什么）─────────────────
	r.emit(id, "stage", "tts", "running")
	if err := r.python("ai/tts_align.py", p.Dir, 30*time.Minute); err != nil {
		return fmt.Errorf("TTS/对齐失败: %w", err)
	}
	r.emit(id, "stage", "tts", "done")
	r.Manifest(p, "produce.tts", "done")

	// ── 2. 语义 spec 生成（LLM）+ 确定性渲染 + 合成物落盘 ────────
	if err := r.compositions(p); err != nil {
		return err
	}

	// ── 3. 组装 ─────────────────────────────────────────────
	r.emit(id, "stage", "assemble", "running")
	if err := r.python("ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
		return fmt.Errorf("组装失败: %w", err)
	}
	r.emit(id, "stage", "assemble", "done")

	// ── 4. check 门禁（自修复：报错回喂重生成 spec）────────────
	if err := r.checkWithRepair(p, ""); err != nil {
		return err
	}

	// ── 5. 渲染成片 ─────────────────────────────────────────
	r.emit(id, "stage", "render", "running")
	if err := os.Remove(p.Artifact("renders/main.mp4")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := pipeline.Run(p, "render"); err != nil {
		return fmt.Errorf("渲染失败: %w", err)
	}
	if err := pipeline.Run(p, "stitch"); err != nil {
		return err
	}
	r.emit(id, "stage", "render", "done")
	r.emit(id, "done", "", "renders/main.mp4")
	r.Manifest(p, "produce.done", "renders/main.mp4")
	if r.OnDone != nil {
		r.OnDone(id, "video")
	}
	return nil
}

// compositions 生成/复用 spec → 渲染 HTML → 清旧帧 → 管线落盘。
func (r *Runner) compositions(p *pipeline.Project) error {
	r.emit(p.ID, "stage", "compositions", "running")
	if err := r.genSpecs(p, "", ""); err != nil {
		return err
	}
	if err := r.renderSpecs(p); err != nil {
		return err
	}
	r.clearFrames(p, "")
	if err := pipeline.Run(p, "compositions"); err != nil {
		return fmt.Errorf("合成物落盘失败: %w", err)
	}
	r.emit(p.ID, "stage", "compositions", "done")
	return nil
}

// checkWithRepair check 失败 → 报错回喂 → 重生成 spec → 重渲染 → 重查（最多 2 轮）。
// only 非空 = 修复范围限定该段（rework），空 = 全片（首次制作）。
func (r *Runner) checkWithRepair(p *pipeline.Project, only string) error {
	for attempt := 1; attempt <= 2; attempt++ {
		r.emit(p.ID, "stage", "check", fmt.Sprintf("running（第 %d 次）", attempt))
		_ = os.Remove(p.Artifact(".hyperframes-ok"))
		err := pipeline.Run(p, "check")
		if err == nil {
			r.emit(p.ID, "stage", "check", "done")
			return nil
		}
		if attempt == 2 {
			r.emit(p.ID, "stage", "check", "error: "+err.Error())
			return fmt.Errorf("check 两轮未过: %w", err)
		}
		feedback := "上一版画面被自动布局检查拒绝，报错摘要：\n" + tail(err.Error(), 25) +
			"\n常见原因：元素重叠/越界。请整体重排：拉开间距、避开底部字幕带（禁放区见布局规则）、必要时减少元素。"
		r.emit(p.ID, "stage", "check", "repair：带着报错重生成画面")
		r.Manifest(p, "check.repair", tail(err.Error(), 10))
		if err := r.genSpecs(p, feedback, only); err != nil {
			return err
		}
		if err := r.renderSpecs(p); err != nil {
			return err
		}
		r.clearFrames(p, only)
		if err := pipeline.Run(p, "compositions"); err != nil {
			return fmt.Errorf("合成物落盘失败: %w", err)
		}
		if err := r.python("ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
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
// feedback 非空 = 带要求重生成；only 非空 = 只处理该段（rework），空 = 全片。
func (r *Runner) genSpecs(p *pipeline.Project, feedback, only string) error {
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
		return fmt.Errorf("spec 生成需要 API 模式（DEEPSEEK_API_KEY）: %w", perr)
	}
	cv := ProjectCanvas(p)
	system := specSystemFor(p)
	if feedback != "" && only == "" { // 全片重生成：先清旧 spec
		for _, seg := range sb.Segments {
			_ = os.Remove(p.Artifact("llm/comp-" + seg.ID + ".spec.json"))
		}
	}
	// rework：feedback 是用户的画面修改要求，置顶为最高优先级；
	// 校验报错的回喂仍走 feedback 位（追加在其后）。
	instruction := ""
	if only != "" && feedback != "" {
		instruction, feedback = feedback, ""
	}
	for _, seg := range sb.Segments {
		if only != "" && seg.ID != only {
			continue
		}
		specPath := p.Artifact("llm/comp-" + seg.ID + ".spec.json")
		if _, err := os.Stat(specPath); err == nil && feedback == "" && instruction == "" {
			continue // 已有产物，跳过（重放语义）
		}
		v := voices[seg.ID]
		var spec contract.CompSpec
		u1, err := prov.GenerateJSON(context.Background(), system, specUserPrompt(seg, v, instruction, feedback, cv), &spec)
		if err != nil {
			return fmt.Errorf("%s spec 生成失败: %w", seg.ID, err)
		}
		errs := contract.ValidateSpec(&spec, len(v.Words), cv)
		for repair := 0; repair < 2 && len(errs) > 0; repair++ { // 修复：违规项回喂，最多 2 轮
			fb := instruction + feedback + "\n上一版 spec 被契约校验拒绝，必须逐条修正：\n- " + strings.Join(errs, "\n- ")
			var fixed contract.CompSpec
			u2, err2 := prov.GenerateJSON(context.Background(), system, specUserPrompt(seg, v, "", fb, cv), &fixed)
			if err2 != nil {
				return fmt.Errorf("%s spec 修复失败: %w", seg.ID, err2)
			}
			u1, spec = u2, fixed
			errs = contract.ValidateSpec(&spec, len(v.Words), cv)
		}
		if len(errs) > 0 { // 保底：确定性清洗（删冲突/夹越界），不让整条任务死掉
			remain := contract.SanitizeSpec(&spec, len(v.Words), cv)
			p.Manifest("spec.sanitized", fmt.Sprintf("%s: 清洗后剩余 %d 元素；遗留问题: %s", seg.ID, len(spec.Elements), strings.Join(remain, "; ")))
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

func specUserPrompt(seg contract.Segment, v voiceMeta, instruction, feedback string, cv contract.Canvas) string {
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

## 可用元素（kind 与参数；坐标基于 %d×%d 画布%s）
- title：大标题（整行居中，只给 y）。y, text(≤12字), fs(默认84), reveal
- note：马克笔便签（给左上角坐标）。x, y, text(≤12字), bg(butter/mint/sky/coral/peach/pink), rot(±3), fs(默认40), reveal
- label：文字标注。x, y, text(≤14字), fs(默认38), reveal
- big：大数字/短语强调。x, y, text(≤8字), fs(默认110), reveal
- disc：实心圆盘（主体物，给圆心）。cx, cy, r(60–180), bg(mint/sky/butter/coral), reveal
- circle：小圆点（小物体/角色）。cx, cy, r(20–60), fill(white/mint/sky/butter), reveal
- beam：粗条（条状物/光束，给左上角）。x, y, w, h, rot, bg, reveal
- arrow：箭头（给起终点）。x1, y1, x2, y2, text(可选), reveal

## 词序表（reveal = 揭示时刻的词序号，0 起）
%s

## 布局硬规则（校验器会拒收）
%s
- reveal 按讲解顺序递增、铺满词序（别堆在开头；最大词号 %d）
- 语义呼应画面提示：对比→双色便签左右分置；流程→箭头串联；主体→disc+label 命名；数字→big
%s

## 输出（只输出 JSON，无围栏）
{"note":"布局思路一句话","elements":[…]}
`, instrBlock, seg.Key, len(v.Words), v.DurationS, seg.Narration, briefLabel, seg.VisualBrief,
		int(cv.W), int(cv.H), map[bool]string{true: " 竖屏 9:16", false: ""}[cv.Aspect == "9:16"],
		wb.String(), rules, len(v.Words)-1, feedbackBlock(feedback))
}

func feedbackBlock(f string) string {
	if f == "" {
		return ""
	}
	return "\n## 必须修正\n" + f + "\n"
}

// ── 子进程与工具 ─────────────────────────────────────────────

// python 跑仓库 ai/ 脚本（cwd = 仓库根）。
func (r *Runner) python(script string, projDir string, timeout time.Duration) error {
	py, err := exec.LookPath("python")
	if err != nil {
		return fmt.Errorf("python 不在 PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, py, filepath.Join(r.RootDir, script), projDir)
	cmd.Dir = r.RootDir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s 退出: %w\n%s", script, err, tail(buf.String(), 15))
	}
	fmt.Println(buf.String())
	return nil
}

// renderSpecs spec → llm/comp-segNN.json（确定性渲染层）。
func (r *Runner) renderSpecs(p *pipeline.Project) error {
	return r.python("ai/render_spec.py", p.Dir, 2*time.Minute)
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
