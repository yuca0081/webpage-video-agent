package contract

import (
	"fmt"
	"time"
	"unicode/utf8"
)

// ── 三个前置件（硬门，plan.md §3.1）──────────────────────────────

// Manuscript 文稿：内容权威，分镜旁白只能由此改写。
type Manuscript struct {
	Content   string    `json:"content"`
	Source    string    `json:"source"` // paste | link | ai
	WordCount int       `json:"word_count"`
	CreatedAt time.Time `json:"created_at"`
}

func NewManuscript(content, source string) *Manuscript {
	return &Manuscript{
		Content:   content,
		Source:    source,
		WordCount: utf8.RuneCountInString(content),
		CreatedAt: time.Now(),
	}
}

// Storyboard 分镜：系统中枢数据结构（TTS 输入 / 合成物生成输入 / 重渲寻址单元）。
type Storyboard struct {
	TimingBasis string    `json:"timing_basis"` // narration | music
	StyleID     string    `json:"style_id,omitempty"`
	Plan        string    `json:"plan,omitempty"`
	Segments    []Segment `json:"segments"`
}

type Segment struct {
	ID           string  `json:"id"` // seg01, seg02, ...
	Idx          int     `json:"idx"`
	Key          string  `json:"key"` // 舞台展示的段落名
	Narration    string  `json:"narration"`
	VisualBrief  string  `json:"visual_brief"`
	DurationHint float64 `json:"duration_hint"` // 秒；TTS 后以实际音频时长为准
}

// MaxDurationSec 时长硬上限（2026-09-22 决策：忠实原文，上限 15 分钟）。
const MaxDurationSec = 900

func (sb *Storyboard) TotalHint() float64 {
	t := 0.0
	for _, s := range sb.Segments {
		t += s.DurationHint
	}
	return t
}

// ValidateStoryboard 自检四项中的「结构合法」（plan.md §3.2）。
func ValidateStoryboard(sb *Storyboard) error {
	if sb.TimingBasis != "narration" && sb.TimingBasis != "music" {
		return fmt.Errorf("timing_basis 必须是 narration 或 music，得到 %q", sb.TimingBasis)
	}
	if len(sb.Segments) == 0 {
		return fmt.Errorf("segments 为空")
	}
	for i, s := range sb.Segments {
		if s.Narration == "" {
			return fmt.Errorf("段 %d（%s）旁白为空", i+1, s.ID)
		}
		if s.DurationHint <= 0 {
			return fmt.Errorf("段 %d（%s）duration_hint 非正", i+1, s.ID)
		}
	}
	if sb.TotalHint() > MaxDurationSec {
		return fmt.Errorf("总时长 %.0fs 超过上限 %ds（需压缩取舍并与用户协商）", sb.TotalHint(), MaxDurationSec)
	}
	return nil
}

// StyleSamples 风格样张：唯一必须用户确认的前置件（不好看只有人能判断）。
type StyleSamples struct {
	Direction string   `json:"direction"` // 如「手绘叙事（纸面·马克笔）」
	Samples   []Sample `json:"samples"`   // 1–3 张
	Confirmed bool     `json:"confirmed"` // 硬门：用户点头后才允许开工
}

type Sample struct {
	Tag  string `json:"tag"`  // 「样张 A · 标题帧」
	Desc string `json:"desc"` // 一句话说明
	HTML string `json:"html"` // 样张内容（与成片同一套风格 token，所见即所得）
}

func ValidateStyleSamples(ss *StyleSamples) error {
	if ss.Direction == "" {
		return fmt.Errorf("direction 为空")
	}
	if len(ss.Samples) < 1 || len(ss.Samples) > 3 {
		return fmt.Errorf("样张数量须为 1–3，得到 %d", len(ss.Samples))
	}
	for i, s := range ss.Samples {
		if s.HTML == "" {
			return fmt.Errorf("样张 %d 缺少 HTML 内容", i+1)
		}
	}
	if !ss.Confirmed {
		return fmt.Errorf("风格图未经用户确认（confirmed=false）——硬门，不可跳过")
	}
	return nil
}

// ── 元素注册表（四级寻址·元素级的地基，plan.md §4.2）────────────

type ElementRegistry struct {
	Elements []NamedElement `json:"elements"`
}

type NamedElement struct {
	ID   string `json:"id"`   // 稳定 id，如 seg03-loop-arrow
	Name string `json:"name"` // 人话名，如「箭头：蓝光弹开」
}
