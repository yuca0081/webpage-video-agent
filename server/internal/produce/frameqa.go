package produce

// 段级画面审查（plan §4.5「快照审查」）：渲出的每段段片抽 开场帧 + 落定帧，
// 视觉模型按 rubric 检查（提前穿帮/裂图/重叠/越界/截断/失衡），不过的段
// 带问题回喂重出 spec → 只重渲该段（自修复 ≤2 轮）。仍不过不阻塞成片：
// stage 事件标红 + manifest 记账，用户知情后可对该段 rework。
// 审查模型与生成共用 LLM_* 配置（glm-5.3-flash 已实测支持图片输入 + JSON 模式）。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"webpage-video-agent/server/internal/contract"
	"webpage-video-agent/server/internal/llm"
	"webpage-video-agent/server/internal/pipeline"
)

const (
	firstFrameAt  = "0.8"  // 开场帧时刻：首元素 reveal 最早 0.05s + 入场 ~0.35s，0.8s 已落定
	lastFrameFrom = "-0.3" // 落定帧：段尾倒数 0.3s（同 M0 visual_qa 的「段落定状态」口径）
)

type frameVerdict struct {
	Pass   bool `json:"pass"`
	Issues []struct {
		Frame      string `json:"frame"` // first|last
		Severity   string `json:"severity"`
		Element    string `json:"element"`
		Problem    string `json:"problem"`
		Suggestion string `json:"suggestion"`
	} `json:"issues"`
}

// frameQA 对 only 段（空=全片）做视觉审查 + 自修复。instruction 是 rework 的
// 用户改画要求：修复轮重出 spec 时必须原样带上，否则用户要求会在修复轮被丢掉。
func (r *Runner) frameQA(ctx context.Context, p *pipeline.Project, instruction, only string) error {
	id := p.ID
	r.emit(id, "stage", "frameqa", "running")
	r.Manifest(p, "frameqa.start", map[bool]string{true: only, false: "全片"}[only != ""])
	for round := 0; ; round++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		failed, err := r.reviewSegments(ctx, p, only)
		if err != nil {
			return err
		}
		if len(failed) == 0 {
			r.emit(id, "stage", "frameqa", "done")
			r.Manifest(p, "frameqa.pass", only)
			return nil
		}
		if round >= 2 { // 修复上限：不阻塞成片，标记问题交给用户决定（plan §4.5）
			summary := failSummary(failed)
			r.emit(id, "stage", "frameqa", "error: "+summary)
			r.Manifest(p, "frameqa.failed", summary)
			return nil
		}
		r.emit(id, "progress", "frameqa",
			fmt.Sprintf("%d 段审查未过，带问题重出画面（第 %d 轮）", len(failed), round+1))
		for segID, issues := range failed {
			if err := r.genSpecs(ctx, p, instruction, issues, segID); err != nil {
				return err
			}
			if err := r.fetchImages(ctx, p); err != nil {
				return err
			}
			if err := r.renderSpecs(ctx, p); err != nil {
				return err
			}
			r.clearFrames(p, segID)
			if err := pipeline.Run(ctx, p, "compositions"); err != nil {
				return fmt.Errorf("合成物落盘失败: %w", err)
			}
			_ = os.Remove(p.Artifact("renders/segs/" + segID + ".mp4"))
		}
		if err := r.python(ctx, "ai/assemble.py", p.Dir, 2*time.Minute); err != nil {
			return fmt.Errorf("组装失败: %w", err)
		}
		if err := pipeline.Run(ctx, p, "render"); err != nil {
			return fmt.Errorf("重渲失败: %w", err)
		}
	}
}

// reviewSegments 范围内每段抽两帧送审，返回 未过段 → 问题回喂文本。
// 单段审查调用失败视为审查器抖动：manifest 记账后跳过（宁可放过不误杀管线）。
func (r *Runner) reviewSegments(ctx context.Context, p *pipeline.Project, only string) (map[string]string, error) {
	sb, err := p.LoadStoryboard()
	if err != nil {
		return nil, err
	}
	voices, err := loadAudioMeta(p)
	if err != nil {
		return nil, err
	}
	prov, err := llm.FromEnv(llm.RoleVisual)
	if err != nil {
		return nil, fmt.Errorf("画面审查需要 API 模式（LLM_API_KEY）: %w", err)
	}
	failed := map[string]string{}
	for _, seg := range sb.Segments {
		if only != "" && seg.ID != only {
			continue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		mp4 := p.Artifact("renders/segs/" + seg.ID + ".mp4")
		if !existsFile(mp4) {
			continue // 无段片：交给渲染阶段的报错，这里不背
		}
		first, last, err := extractFrames(ctx, p, seg.ID)
		if err != nil {
			return nil, err
		}
		var vd frameVerdict
		_, rerr := prov.GenerateJSONVision(ctx,
			"你是科普视频的画面质检员，只输出 JSON。",
			reviewPrompt(p, seg, voices[seg.ID]),
			[]string{dataURL(first), dataURL(last)}, &vd)
		if rerr != nil {
			r.Manifest(p, "frameqa.review_error", seg.ID+": "+rerr.Error())
			continue
		}
		if vd.Pass && !hasHigh(vd) {
			r.Manifest(p, "frameqa.seg", fmt.Sprintf("%s: pass", seg.ID))
			continue
		}
		r.Manifest(p, "frameqa.seg", fmt.Sprintf("%s: fail（%d 个问题）", seg.ID, len(vd.Issues)))
		failed[seg.ID] = issuesFeedback(vd)
	}
	return failed, nil
}

// extractFrames 段片抽 开场帧/落定帧 各一张（缩到 1280 宽的 JPEG，控制多模态请求体积）。
// 指定时刻抽不到帧（极短段）回退直接取第一帧；段长 8–16s 正常走不到回退。
func extractFrames(ctx context.Context, p *pipeline.Project, segID string) (string, string, error) {
	ff, err := pipeline.FFmpegPath()
	if err != nil {
		return "", "", err
	}
	mp4 := p.Artifact("renders/segs/" + segID + ".mp4")
	dir := p.Artifact("reports/frameqa")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	first := filepath.Join(dir, segID+".first.jpg")
	last := filepath.Join(dir, segID+".last.jpg")
	const vf = "scale=1280:-2"
	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, ff, args...)
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%w\n%s", err, tail(buf.String(), 5))
		}
		return nil
	}
	if err := run("-y", "-v", "error", "-ss", firstFrameAt, "-i", mp4, "-frames:v", "1", "-vf", vf, "-q:v", "4", first); err != nil || !existsFile(first) {
		if err := run("-y", "-v", "error", "-i", mp4, "-frames:v", "1", "-vf", vf, "-q:v", "4", first); err != nil {
			return "", "", fmt.Errorf("%s 抽开场帧失败: %w", segID, err)
		}
	}
	if err := run("-y", "-v", "error", "-sseof", lastFrameFrom, "-i", mp4, "-frames:v", "1", "-vf", vf, "-q:v", "4", last); err != nil || !existsFile(last) {
		if err := run("-y", "-v", "error", "-i", mp4, "-frames:v", "1", "-vf", vf, "-q:v", "4", last); err != nil {
			return "", "", fmt.Errorf("%s 抽落定帧失败: %w", segID, err)
		}
	}
	return first, last, nil
}

// reviewPrompt 两帧一调（省钱且模型可对照）：开场帧查穿帮/裂图，落定帧查布局全项。
// 开场应有元素清单从 spec+词级时间戳算出，给「提前穿帮」一个客观对照物。
// 图片按 图1=开场帧、图2=落定帧 顺序附在消息里。
func reviewPrompt(p *pipeline.Project, seg contract.Segment, v voiceMeta) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`这是同一段视频的两帧渲染画面（底部字幕带是画面一部分）。
图1 = 开场帧（约 0.8s，开场元素刚落定）；图2 = 落定帧（段尾，元素齐全）。

## 段信息
- 标题：%s
- 旁白：%s
`, seg.Key, seg.Narration))
	if early := earlyElements(p, seg.ID, v); len(early) > 0 {
		b.WriteString("\n## 开场帧此时应有的元素（清单外的元素尚未出现是正常渐入设计，不算问题）\n")
		for _, e := range early {
			b.WriteString("- " + e + "\n")
		}
	}
	b.WriteString(`
## 检查（宁严勿松，只报画面上真实可见的问题）
图1 开场帧：
1. 穿帮：不应出现的元素提前出现
2. 裂图：图片未加载/占位符裸奔/空白主体
3. 文字截断、元素出画、明显错位
图2 落定帧：
1. 排列：元素重叠遮挡（文字压文字/压图形主体）、越界出画、贴边过紧
2. 布局：视觉重心失衡、大面积空白（>25% 画面）、元素挤在一角
3. 可读性：文字过小、对比不足；裂图/截断
两帧通用：动效中间态（半透明/位移中）不算问题；轻微审美偏好不算问题。

## 输出 JSON
{"pass": true/false, "issues": [{"frame": "first|last", "severity": "high|medium|low", "element": "哪个元素", "problem": "什么问题", "suggestion": "怎么改"}]}
pass 标准：无 high 问题即 true；medium/low 记录但不算失败。`)
	return b.String()
}

// earlyElements 揭示时刻 ≤ 开场帧时刻的元素（口径与 ai/render_spec.py reveal_t 一致：
// max(0.05, 词start - 0.12)）。spec 缺失（异常）返回 nil，提示词降级为无清单。
func earlyElements(p *pipeline.Project, segID string, v voiceMeta) []string {
	b, err := os.ReadFile(p.Artifact("llm/comp-" + segID + ".spec.json"))
	if err != nil {
		return nil
	}
	var spec contract.CompSpec
	if json.Unmarshal(b, &spec) != nil {
		return nil
	}
	var out []string
	for i := range spec.Elements {
		e := &spec.Elements[i]
		reveal := e.Reveal
		t := 0.05
		if len(v.Words) > 0 {
			idx := reveal
			if idx < 0 {
				idx = 0
			}
			if idx > len(v.Words)-1 {
				idx = len(v.Words) - 1
			}
			if s := v.Words[idx].Start - 0.12; s > 0.05 {
				t = s
			}
		}
		if t > 0.8+0.05 {
			continue
		}
		if label := elemLabel(e); label != "" {
			out = append(out, label)
		}
	}
	return out
}

func elemLabel(e *contract.SpecElement) string {
	main := e.Text
	if main == "" {
		main = e.Title
	}
	if main == "" && len(e.Nodes) > 0 {
		main = strings.Join(e.Nodes, "、")
	}
	if main == "" && len(e.Labels) > 0 {
		main = strings.Join(e.Labels, "、")
	}
	if main == "" && e.Name != "" {
		main = e.Name
	}
	if main == "" && e.Query != "" {
		main = "照片:" + e.Query
	}
	if main == "" {
		return e.Kind
	}
	return e.Kind + ":" + runeTrunc(main, 16)
}

func runeTrunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func dataURL(file string) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
}

func hasHigh(v frameVerdict) bool {
	for _, i := range v.Issues {
		if i.Severity == "high" {
			return true
		}
	}
	return false
}

// issuesFeedback high/medium 问题 → 回喂 genSpecs 的修正要求（low 噪音不回喂）。
func issuesFeedback(v frameVerdict) string {
	var b strings.Builder
	b.WriteString("渲染后的画面审查发现以下问题，重出 spec 时必须逐条修正（调整坐标/大小/删减元素）：\n")
	for _, i := range v.Issues {
		if i.Severity != "high" && i.Severity != "medium" {
			continue
		}
		frame := "落定帧"
		if i.Frame == "first" {
			frame = "开场帧"
		}
		b.WriteString(fmt.Sprintf("- [%s][%s] %s: %s → %s\n", frame, i.Severity, i.Element, i.Problem, i.Suggestion))
	}
	return b.String()
}

func failSummary(failed map[string]string) string {
	segs := make([]string, 0, len(failed))
	for id := range failed {
		segs = append(segs, id)
	}
	return "两轮修复后仍未过审查: " + strings.Join(segs, ",")
}

func existsFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
