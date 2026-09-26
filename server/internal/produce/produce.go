// Package produce：制作任务运行器（确定性编排，LLM 只在需要的点上介入）。
//
// 链路：tts（python）→ 每段语义 spec（DeepSeek + 契约校验 + 修复一轮）
//
//	→ spec 渲染 HTML（python render_spec）→ 管线落盘合成物 → 组装（python assemble）
//	→ check 门禁（失败带着报错重生成 spec 自修复，最多 2 轮）→ 渲染 → 成片。
//
// 每步 SSE 广播 stage 事件；产物落盘可从任意步重放（manifest 记账）。
package produce

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"webpage-video-agent/server/internal/contract"
	"webpage-video-agent/server/internal/events"
	"webpage-video-agent/server/internal/llm"
	"webpage-video-agent/server/internal/pipeline"
)

type Runner struct {
	DataDir string // 仓库根 data/（项目在 data/projects/<id>）
	RootDir string // 仓库根（ai/ 脚本所在）
	Hub     *events.Hub
	OnDone  func(projectID, status, detail string) // 终态回写（status: video|failed|cancelled；detail 供聊天汇报失败原因等）
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
				r.OnDone(projectID, "cancelled", "用户取消制作")
			}
			return
		}
		if err != nil {
			r.emit(projectID, "stage", "pipeline", "error: "+err.Error())
			r.emit(projectID, "error", "", err.Error())
			r.Manifest(p, "produce.error", err.Error())
			if r.OnDone != nil {
				r.OnDone(projectID, "failed", err.Error())
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
				r.OnDone(projectID, "cancelled", "用户取消重做")
			}
			return
		}
		if err != nil {
			r.emit(projectID, "stage", "pipeline", "error: "+err.Error())
			r.emit(projectID, "error", "", err.Error())
			r.Manifest(p, "rework.error", seg.ID+": "+err.Error())
			if r.OnDone != nil {
				r.OnDone(projectID, "failed", err.Error())
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
		r.OnDone(id, "video", fmt.Sprintf("段%d「%s」画面重做完成", seg.Idx, seg.Key))
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
		r.OnDone(id, "video", "全片制作完成")
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
		// 廉价修复先行：checker 对对比度项给了建议色就直接回填 HTML 再检，
		// 不烧 LLM 修复轮——LLM 回喂修对比度从未收敛过（同色反复重生成）。
		if n := applyContrastFixes(p, err.Error()); n > 0 {
			r.emit(p.ID, "stage", "check", fmt.Sprintf("contrast-fix：按建议色回填 %d 处", n))
			r.Manifest(p, "check.contrast_fix", fmt.Sprintf("%d 处", n))
			if err := r.python(ctx, "ai/assemble.py", p.Dir, 2*time.Minute); err == nil {
				if pipeline.Run(ctx, p, "check") == nil {
					r.emit(p.ID, "stage", "check", "done")
					return nil
				}
			}
			// 回填后仍不过 → 落到下面的 LLM 修复轮
		}
		if attempt == 3 {
			r.emit(p.ID, "stage", "check", "error: "+err.Error())
			return fmt.Errorf("check 三轮未过: %w", err)
		}
		// 修复目标 = 重做段 ∪ 本轮报错涉及的段：check 是全片门禁，重做段之外的段
		// 报错时若只重生成重做段，那些错误永远修不掉（结构性死锁）。
		targets := failingSegs(err.Error())
		if only != "" && !slices.Contains(targets, only) {
			targets = append([]string{only}, targets...)
		}
		repairOnly := strings.Join(targets, ",")
		feedback := "上一版画面被自动检查拒绝，报错摘要（✗ 行逐项都要修）：\n" + tail(err.Error(), 25) +
			"\n修复规则：\n" +
			"- 重叠/越界：整体重排拉开间距，避开底部字幕带（禁放区见布局规则），必要时减少元素。\n" +
			"- 对比度 ✗：文字色与它实际压着的背景太接近。检查器的建议色不可信（实测往往不达标）——" +
			"文字压在照片/深色底上一律改极浅色（#EFE9DA、#E3E3E3 级），浅色底上用深色（#2D2D2D 级）；" +
			"中灰（#616161、#9E9E9E）做正文/标注色直接不达标，禁用。\n" +
			"逐项修完再输出。"
		r.emit(p.ID, "stage", "check", fmt.Sprintf("repair：重生成画面（段：%s）", repairOnly))
		r.Manifest(p, "check.repair", tail(err.Error(), 10))
		if err := r.genSpecs(ctx, p, instruction, feedback, repairOnly); err != nil {
			return err
		}
		if err := r.fetchImages(ctx, p); err != nil {
			return err
		}
		if err := r.renderSpecs(ctx, p); err != nil {
			return err
		}
		r.clearFrames(p, repairOnly)
		if err := pipeline.Run(ctx, p, "compositions"); err != nil {
			return fmt.Errorf("合成物落盘失败: %w", err)
		}
		if err := r.python(ctx, "ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
			return fmt.Errorf("组装失败: %w", err)
		}
	}
	return errors.New("unreachable")
}

// segIDRe 段 id（seg01、seg02…）。
var segIDRe = regexp.MustCompile(`seg\d{2,}`)

// failingSegs 从 check 诊断提取报错（✗ 行）涉及的段 id：选择器前缀 #segNN-… 与
// 对比度项的 source 路径都算。info 行不取——健康段没必要重生成。
func failingSegs(checkOut string) []string {
	set := map[string]bool{}
	var order []string
	add := func(id string) {
		if !set[id] {
			set[id] = true
			order = append(order, id)
		}
	}
	lines := strings.Split(checkOut, "\n")
	for i, ln := range lines {
		if !strings.HasPrefix(strings.TrimSpace(ln), "✗") {
			continue
		}
		for _, id := range segIDRe.FindAllString(ln, -1) {
			add(id)
		}
		if i+1 < len(lines) { // 对比度项的选择器在 ✗ 行，段文件在下一行 Try/source
			for _, id := range segIDRe.FindAllString(lines[i+1], -1) {
				add(id)
			}
		}
	}
	return order
}

// ── spec 生成（LLM 作业：语义布局）─────────────────────────────

type voiceMeta struct {
	ID        string       `json:"id"`
	DurationS float64      `json:"duration_s"`
	Words     []wordTiming `json:"words"`
}

type wordTiming struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
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
	// 版式库：命中注册表风格的每段先指定一个构图版式（段号轮换），治布局单调
	layouts := styleLayoutsFor(p, r.RootDir)
	// 样张截图（ai/snap_samples.py，Edge 无头，mtime 缓存）→ spec 生成改走多模态：
	// 模型看着真图写 spec，替代纯文字风格描述；截图失败不挡管线（退回纯文本）。
	if err := r.python(ctx, "ai/snap_samples.py", p.Dir, time.Minute); err != nil {
		r.emit(p.ID, "progress", "compositions", "样张截图失败，风格注入退回纯文本")
	}
	sampleImgs := styleSampleImages(p)
	// NoThink：画面 spec 是结构化 HTML 作业，开思考单段 5–13 分钟（reasoning 占 1 万+ token）；
	// 关思考后单段分钟级，配合段间并发把全片从半小时级压进几分钟。
	gen := func(user string, out any) (openai.Usage, error) {
		if len(sampleImgs) > 0 {
			return prov.GenerateJSONVisionNoThink(ctx, system, user, sampleImgs, 0.7, out)
		}
		return prov.GenerateJSONNoThink(ctx, system, user, out)
	}
	if feedback != "" && only == "" { // 全片重生成：先清旧 spec
		for _, seg := range sb.Segments {
			_ = os.Remove(p.Artifact("llm/comp-" + seg.ID + ".spec.json"))
		}
	}
	// 先筛出待生成段（已有产物跳过，重放语义），再并发填坑。
	// only 逗号分隔多段（rework 主目标段 + check 报错段一起修）。
	onlySet := map[string]bool{}
	for _, id := range strings.Split(only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			onlySet[id] = true
		}
	}
	var todo []contract.Segment
	for _, seg := range sb.Segments {
		if len(onlySet) > 0 && !onlySet[seg.ID] {
			continue
		}
		specPath := p.Artifact("llm/comp-" + seg.ID + ".spec.json")
		if _, err := os.Stat(specPath); err == nil && feedback == "" && instruction == "" {
			continue // 已有产物，跳过
		}
		todo = append(todo, seg)
	}
	// 段间零依赖（输入只读、spec 各落各的文件）→ 有界并发；429/网络抖动由 provider 重试兜底
	sem := make(chan struct{}, specConcurrency())
	var wg sync.WaitGroup
	var errMu sync.Mutex
	firstErr := error(nil)
	fail := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}
	work := func(seg contract.Segment, specPath string) {
		v := voices[seg.ID]
		lb := layoutNote(layouts, seg.Idx)
		var spec contract.CompSpec
		u1, err := gen(specUserPrompt(seg, v, instruction, feedback, cv, style, menu, lb), &spec)
		if err != nil {
			fail(fmt.Errorf("%s spec 生成失败: %w", seg.ID, err))
			return
		}
		errs := contract.ValidateSpec(&spec, len(v.Words), cv)
		errs = append(errs, iconErrors(&spec, icons)...)
		for repair := 0; repair < 2 && len(errs) > 0; repair++ { // 修复：违规项回喂，最多 2 轮
			fb := instruction + feedback + "\n上一版 spec 被契约校验拒绝，必须逐条修正：\n- " + strings.Join(errs, "\n- ")
			var fixed contract.CompSpec
			u2, err2 := gen(specUserPrompt(seg, v, "", fb, cv, style, menu, lb), &fixed)
			if err2 != nil {
				fail(fmt.Errorf("%s spec 修复失败: %w", seg.ID, err2))
				return
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
					u2, err2 := gen(specUserPrompt(seg, v, instruction,
						fmt.Sprintf("上一版几乎每个元素的 kind 都是空的或不认识的。kind 必须从可用元素菜单里逐字选取（%s），每个元素都必须有 kind。", kinds),
						cv, style, menu, lb), &retry)
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
			fail(fmt.Errorf("%s spec 元素不足 3 个（模型输出异常，重试后仍失败）", seg.ID))
			return
		}
		b, _ := json.MarshalIndent(spec, "", "  ")
		if err := os.WriteFile(specPath, b, 0o644); err != nil {
			fail(err)
			return
		}
		p.Manifest("llm.usage", fmt.Sprintf("spec %s prompt=%d completion=%d tokens", seg.ID, u1.PromptTokens, u1.CompletionTokens))
		r.emit(p.ID, "progress", "compositions", fmt.Sprintf("%s 画面 spec ✓（%d 元素）", seg.ID, len(spec.Elements)))
	}
	for _, seg := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				fail(err)
				return
			}
			work(seg, p.Artifact("llm/comp-"+seg.ID+".spec.json"))
		}()
	}
	wg.Wait()
	return firstErr
}

// specConcurrency spec 生成并发度（段间零依赖；GLM 编码套餐并发有限，默认 3，
// 超限 429 由 provider 重试兜底；SPEC_CONCURRENCY 环境变量可调）。
func specConcurrency() int {
	if n, err := strconv.Atoi(os.Getenv("SPEC_CONCURRENCY")); err == nil && n >= 1 && n <= 9 {
		return n
	}
	return 3
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

func specUserPrompt(seg contract.Segment, v voiceMeta, instruction, feedback string, cv contract.Canvas, style, menu, layout string) string {
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

## 配图（image 元素的 query 与 source）
- source="search"（默认/不写）：query 是 ≤12 字中文搜索词，配真实照片——动物/地标/物品/场景/产品
- source="gen"：AI 生图，query 是一句完整画面描述（20–60 字中文，具体到主体/构图/配色），配插画/概念示意/抽象背景；描述尽量往风格样张的气质上靠
- 真实存在的名人/品牌/产品/新闻实物禁止 gen（生图会编造事实），一律 search
- 全幅背景：image 加 role="bg"（照片满画幅垫底+自动纸色压暗，每段最多 1 张；dim 控制压暗 0–1 默认 0.55，blur 0–12 毛玻璃）。用一张背景图撑住整段氛围，其余元素叠在上面；背景图 query 走 gen 描述大场景（如渐变宇宙/俯瞰城市/抽象流体），也放 elements 最前

## 词序表（reveal = 揭示时刻的词序号，0 起）
%s

## 布局硬规则（校验器会拒收）
%s
- reveal 按讲解顺序递增、铺满词序（别堆在开头；最大词号 %d）
- 动效增强（可选）：元素可加 anim 换入场（pop/fade/rise/slide/wipe=左→右揭示/blur=失焦聚焦/chars=逐字，仅 title·big/none=直出）与 exit=词序号（讲完该词退场，给后续元素腾画面；须大于 reveal）；段级顶层可加 camera（zoom_in/zoom_out/pan_left/pan_right/drift）整屏缓推，一屏最多一个，信息密集段别用
- 语义呼应画面提示：主体物→image（实体名词首选）或 icon（抽象概念）；数据对比→chart_bar；趋势→chart_line；占比→chart_donut 或 chart_pie；对比→双色便签左右分置或 table；流程/步骤→timeline 或箭头串联；要点/卖点→checklist；金句/名言→quote；关键数字→stat 或 big；指向→barrow；向量/维度/批量→strip；分组圈注→zone(+bracket)；整屏氛围底→image role="bg"（照片背景）或 custom role="bg"（渐变/粒子/光斑特效底，每段各≤1）；斜切大字板/故障标题/纹理装饰→custom（每段≤3个，垫底放 elements 最前）；一屏最多一个图表（图表占主视觉位）
- 画面丰富度（重要）：每屏至少一个视觉锚点（image / 大 icon / 图表 / big / panel 之一），大小拉开层次（主体 300px+、次级 120–200px），禁止全屏小元素平铺；段落适合铺满氛围时优先给背景层（照片 bg 或特效底），别让画面停在纯色底
%s

## 输出（只输出 JSON，无围栏）
{"note":"布局思路一句话","camera":"（可选）zoom_in/zoom_out/pan_left/pan_right/drift","elements":[…]}
`, instrBlock, seg.Key, len(v.Words), v.DurationS, seg.Narration, briefLabel, seg.VisualBrief, style, layout,
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

// styleLayout 风格版式（构图模式）：styles.json layouts 字段，描述本风格的标准构图，
// spec 生成每段先选一个版式再填元素——治"整齐网格 PPT 感"。
type styleLayout struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// styleRegEntry styles.json 条目（Go 侧只消费方向/题材/配图调性/版式）。
type styleRegEntry struct {
	Direction string        `json:"direction"`
	Genre     string        `json:"genre"`
	Keywords  []string      `json:"keywords"`
	Photo     string        `json:"photo"`
	Layouts   []styleLayout `json:"layouts"`
}

// projectDirection 项目已确认/已起草的风格方向（style_samples.json）。
func projectDirection(p *pipeline.Project) string {
	var ss struct {
		Direction string `json:"direction"`
	}
	if b, err := os.ReadFile(p.Artifact("style/style_samples.json")); err == nil &&
		json.Unmarshal(b, &ss) == nil {
		return ss.Direction
	}
	return ""
}

// matchStyleReg 方向名 → 注册表条目。两轮匹配：完整方向名子串优先，再关键词最长命中
// （与 ai/engines/stylepack_generic.py match 同语义）。未命中返回 nil。
func matchStyleReg(direction, rootDir string) *styleRegEntry {
	if direction == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(rootDir, "ai", "registry", "styles.json"))
	if err != nil {
		return nil
	}
	var styles []styleRegEntry
	if json.Unmarshal(b, &styles) != nil {
		return nil
	}
	for i := range styles {
		if styles[i].Direction != "" && strings.Contains(direction, styles[i].Direction) {
			return &styles[i]
		}
	}
	best, bestLen := -1, 0
	for i := range styles {
		for _, kw := range styles[i].Keywords {
			n := len([]rune(kw))
			if kw != "" && strings.Contains(direction, kw) && n > bestLen {
				best, bestLen = i, n
			}
		}
	}
	if best >= 0 {
		return &styles[best]
	}
	return nil
}

// styleNoteFor 项目风格方向 → spec 提示里的风格说明（题材 + 配图调性）。
func styleNoteFor(p *pipeline.Project, rootDir string) string {
	direction := projectDirection(p)
	if direction == "" {
		return ""
	}
	note := "\n## 本片风格\n- 方向：" + direction
	st := matchStyleReg(direction, rootDir)
	if st == nil {
		return note
	}
	if st.Genre != "" {
		note += "\n- 适用题材：" + st.Genre
	}
	if st.Photo != "" {
		note += "\n- 配图 query 调性：" + st.Photo + "（image 的 query 往这个调性上靠）"
	}
	return note
}

// styleLayoutsFor 项目风格 → 注册表命中的版式列表（未命中/老注册表返回 nil）。
func styleLayoutsFor(p *pipeline.Project, rootDir string) []styleLayout {
	st := matchStyleReg(projectDirection(p), rootDir)
	if st == nil {
		return nil
	}
	return st.Layouts
}

// layoutNote 本段版式注入块：指定版式（按段号轮换，相邻段天然不同）+ 全表可改选。
// layouts 为空返回 ""，行为与旧版一致。校验硬底线（≥3 元素、不重叠）不放松。
func layoutNote(layouts []styleLayout, segIdx int) string {
	if len(layouts) == 0 {
		return ""
	}
	if segIdx < 1 {
		segIdx = 1
	}
	asg := layouts[(segIdx-1)%len(layouts)]
	var b strings.Builder
	b.WriteString("\n## 本段版式（先定构图，再往里填元素；禁止退回「标题+小卡平铺」的默认网格）\n")
	fmt.Fprintf(&b, "- 本段指定版式：%s（%s）——%s\n", asg.ID, asg.Name, asg.Desc)
	b.WriteString("- 本风格可用版式（画面提示与指定版式明显冲突时可改选，note 首句写「版式：<id>」）：\n")
	for _, l := range layouts {
		fmt.Fprintf(&b, "  - %s（%s）：%s\n", l.ID, l.Name, l.Desc)
	}
	b.WriteString("- 元素数量与铺排以版式描述为准（下方 4–7 个的一般规则让位于版式；仍须 ≥3 个且不重叠）")
	return b.String()
}

// styleSampleImages 已截图的风格样张（style/sample-N.png，ai/snap_samples.py 产物）→ data URL。
// spec 生成时附给多模态模型：看真图选 kind/配色/调性，替代纯文字风格注入。最多 3 张，缺文件返回 nil。
func styleSampleImages(p *pipeline.Project) []string {
	var urls []string
	for i := 0; i < 3; i++ {
		b, err := os.ReadFile(p.Artifact(fmt.Sprintf("style/sample-%d.png", i)))
		if err != nil {
			break
		}
		urls = append(urls, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(b))
	}
	return urls
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
		for _, id := range strings.Split(only, ",") {
			if id = strings.TrimSpace(id); id == "" {
				continue
			}
			_ = os.Remove(p.Artifact("compositions/frames/" + id + ".html"))
			_ = os.Remove(p.Artifact("compositions/registry/" + id + ".json"))
		}
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

// contrastSuggRe 抓 checker 对比度诊断对：✗ 选择器 比值:1 (need …) \n Try rgb(..); source frames/segNN.html
var contrastSuggRe = regexp.MustCompile(
	`✗[ ]*([^\r\n]+?)\s+[0-9]+(?:\.[0-9]+)?:1\s*\(need[^)]*\)\s*\n\s*Try\s+(rgb\([^)]*\))\s*;\s*source\s+(\S+)`)

// applyContrastFixes 把 checker 给的对比度建议色直接回填到段帧 HTML（追加
// data-contrast-fix 样式块，!important 压过内联样式），返回本次新回填的处数。
// 已在 HTML 里的同条规则跳过（内容去重）：LLM 修复轮重生成 HTML 会冲掉旧补丁，
// 此时规则缺席会重新回填；规则在却还报同项，说明该建议色无效，不重复打转。
func applyContrastFixes(p *pipeline.Project, checkOut string) int {
	type fix struct {
		file, css string
	}
	var fixes []fix
	for _, m := range contrastSuggRe.FindAllStringSubmatch(checkOut, -1) {
		sel, color, src := strings.TrimSpace(m[1]), m[2], m[3]
		if !strings.HasPrefix(src, "compositions/frames/") || !strings.HasSuffix(src, ".html") {
			continue // 只动段帧，别处一概不碰
		}
		fixes = append(fixes, fix{src, sel + "{color:" + color + "!important}"})
	}
	if len(fixes) == 0 {
		return 0
	}
	perFile := map[string][]string{}
	for _, f := range fixes {
		perFile[f.file] = append(perFile[f.file], f.css)
	}
	n := 0
	for rel, rules := range perFile {
		path := p.Artifact(rel)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		html := string(b)
		var fresh []string
		for _, css := range rules {
			if !strings.Contains(html, css) {
				fresh = append(fresh, css)
			}
		}
		if len(fresh) == 0 {
			continue
		}
		block := "<style data-contrast-fix=\"1\">" + strings.Join(fresh, "") + "</style>"
		if i := strings.Index(html, `<style data-contrast-fix`); i >= 0 {
			if j := strings.Index(html[i:], "</style>"); j >= 0 {
				html = html[:i] + block + html[i+j+len("</style>"):]
			} else {
				continue
			}
		} else if i := strings.Index(html, "</body>"); i >= 0 {
			html = html[:i] + block + "\n" + html[i:]
		} else {
			html += block
		}
		if err := os.WriteFile(path, []byte(html), 0o644); err == nil {
			n += len(fresh)
		}
	}
	return n
}
