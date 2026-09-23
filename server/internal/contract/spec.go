package contract

import (
	"fmt"
	"unicode/utf8"
)

// ── 合成物语义 spec（M1：LLM 只出语义，渲染由 ai/render_spec.py 确定性完成）──
//
// LLM 产物不信任原则：spec 过 ValidateSpec（边界/重叠/揭示词位/文案长度）才落盘；
// 违规项回喂给模型修复一轮，仍不过则该段失败。

// CompSpec 一段画面的语义布局。
type CompSpec struct {
	Note     string        `json:"note"`
	Elements []SpecElement `json:"elements"`
}

// ── 画幅（2026-09-23：默认 9:16、可切 16:9，plan.md §7 M1）─────────

// Canvas 画幅几何：尺寸 + 安全区 + 字幕带顶（元素禁放线）。
type Canvas struct {
	Aspect string  // "16:9" | "9:16"
	W, H   float64 // 像素
	// 安全区（元素占位必须整体落在其中）
	X0, Y0, X1, Y1 float64
	CapTop        float64 // 字幕带顶：y ≥ CapTop 禁放
}

func CanvasFor(aspect string) Canvas {
	if aspect == "9:16" {
		return Canvas{Aspect: "9:16", W: 1080, H: 1920,
			X0: 60, Y0: 60, X1: 1020, Y1: 1550, CapTop: 1594}
	}
	return Canvas{Aspect: "16:9", W: 1920, H: 1080,
		X0: 60, Y0: 60, X1: 1860, Y1: 870, CapTop: 896}
}

// SpecElement 元素语义。坐标体系随画幅（CanvasFor），y ≥ CapTop 为字幕带禁放。
type SpecElement struct {
	Kind string `json:"kind"` // title|note|label|big|beam|disc|circle|arrow
	X    float64 `json:"x,omitempty"`  // 左上角（note/label/big/beam）
	Y    float64 `json:"y,omitempty"`
	X1   float64 `json:"x1,omitempty"` // arrow 起点
	Y1   float64 `json:"y1,omitempty"`
	X2   float64 `json:"x2,omitempty"` // arrow 终点
	Y2   float64 `json:"y2,omitempty"`
	Cx   float64 `json:"cx,omitempty"` // 圆心（disc/circle）
	Cy   float64 `json:"cy,omitempty"`
	R    float64 `json:"r,omitempty"`  // 半径（disc/circle）
	W    float64 `json:"w,omitempty"`  // 宽（beam）
	H    float64 `json:"h,omitempty"`
	Text string  `json:"text,omitempty"`
	Bg   string  `json:"bg,omitempty"`   // butter|mint|sky|coral|peach|pink|turq|white|ink
	Color string `json:"color,omitempty"`
	Fill string  `json:"fill,omitempty"`
	Rot  float64 `json:"rot,omitempty"`
	Fs   float64 `json:"fs,omitempty"`
	Reveal int   `json:"reveal,omitempty"` // 揭示词位（段内词序号，0 起）
}

var specKinds = map[string]bool{
	"title": true, "note": true, "label": true, "big": true,
	"beam": true, "disc": true, "circle": true, "arrow": true,
}

// bbox 保守估计元素占位（供边界与重叠检查）。
func (e *SpecElement) bbox(cv Canvas) (x0, y0, w, h float64, ok bool) {
	runeLen := utf8.RuneCountInString(e.Text)
	fs := e.Fs
	switch e.Kind {
	case "title":
		if fs == 0 {
			fs = 84
		}
		w = float64(runeLen)*fs*1.18 + 40
		if max := cv.W - 2*cv.X0; w > max {
			w = max
		}
		return (cv.W - w) / 2, e.Y, w, fs * 1.4, true
	case "note":
		if fs == 0 {
			fs = 40
		}
		return e.X, e.Y, float64(runeLen)*fs + 56, fs + 40, true
	case "label":
		if fs == 0 {
			fs = 38
		}
		return e.X, e.Y, float64(runeLen)*fs + 8, fs * 1.3, true
	case "big":
		if fs == 0 {
			fs = 110
		}
		return e.X, e.Y, float64(runeLen)*fs*0.62, fs * 1.15, true
	case "beam":
		if e.H == 0 {
			e.H = 22
		}
		return e.X, e.Y, e.W, e.H, true
	case "disc", "circle":
		return e.Cx - e.R, e.Cy - e.R, 2 * e.R, 2 * e.R, true
	case "arrow":
		x, y := e.X1, e.Y1
		if e.X2 < x {
			x = e.X2
		}
		if e.Y2 < y {
			y = e.Y2
		}
		w, h := abs(e.X2-e.X1), abs(e.Y2-e.Y1)
		if w < 68 {
			w = 68
		}
		if h < 68 {
			h = 68
		}
		return x, y, w, h, true
	}
	return 0, 0, 0, 0, false
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ValidateSpec wordCount = 该段词级时间戳词数（reveal 上界）。
// cv = 画幅几何（安全区与字幕带随 9:16 / 16:9 变化）。
// 返回违规列表（空 = 通过）；违规项回喂模型修复。
// 箭头不参与重叠判定（它的职责就是连接/跨越其他元素）。
func ValidateSpec(s *CompSpec, wordCount int, cv Canvas) []string {
	var errs []string
	if n := len(s.Elements); n < 3 || n > 10 {
		errs = append(errs, fmt.Sprintf("元素数 %d 不在 3–10 范围（尽量 4–7）", n))
	}
	type box struct {
		i int
		b specBox
	}
	var boxes []box
	for i := range s.Elements {
		e := &s.Elements[i]
		tag := fmt.Sprintf("元素%d(%s)", i+1, e.Kind)
		if !specKinds[e.Kind] {
			errs = append(errs, tag+": 未知 kind")
			continue
		}
		if e.Reveal < 0 || (wordCount > 0 && e.Reveal >= wordCount) {
			errs = append(errs, fmt.Sprintf("%s: reveal=%d 超出词数 %d", tag, e.Reveal, wordCount))
		}
		needText := e.Kind == "title" || e.Kind == "note" || e.Kind == "label" || e.Kind == "big"
		if needText && e.Text == "" {
			errs = append(errs, tag+": 缺 text")
		}
		lim := map[string]int{"title": 14, "note": 14, "label": 16, "big": 9}
		if lim[e.Kind] > 0 && utf8.RuneCountInString(e.Text) > lim[e.Kind] {
			errs = append(errs, fmt.Sprintf("%s: 文案「%s」超长（≤%d 字）", tag, e.Text, lim[e.Kind]))
		}
		if e.Kind == "disc" || e.Kind == "circle" {
			if e.R < 18 || e.R > 220 {
				errs = append(errs, fmt.Sprintf("%s: 半径 %v 超范围 18–220", tag, e.R))
			}
		}
		x0, y0, w, h, ok := e.bbox(cv)
		if !ok {
			continue
		}
		x1, y1 := x0+w, y0+h
		if x0 < cv.X0 || x1 > cv.X1 || y0 < cv.Y0 || y1 > cv.Y1 {
			errs = append(errs, fmt.Sprintf("%s: 占位 (%.0f,%.0f)-(%.0f,%.0f) 越出安全区 x[%.0f,%.0f] y[%.0f,%.0f]",
				tag, x0, y0, x1, y1, cv.X0, cv.X1, cv.Y0, cv.Y1))
		}
		boxes = append(boxes, box{i: i, b: specBox{x0, y0, x1, y1}})
	}
	// 重叠：交叠面积超过较小方块的 25% 判违规。豁免：
	//   箭头（职责就是连接元素）；形状全包含嵌套（disc/circle 同心构图，如主体+内核）
	for a := 0; a < len(boxes); a++ {
		if s.Elements[boxes[a].i].Kind == "arrow" {
			continue
		}
		for b := a + 1; b < len(boxes); b++ {
			if s.Elements[boxes[b].i].Kind == "arrow" {
				continue
			}
			A, B := boxes[a], boxes[b]
			if nestedShapes(&s.Elements[A.i], A.b, &s.Elements[B.i], B.b) {
				continue
			}
			ow := min(A.b.x1, B.b.x1) - max(A.b.x0, B.b.x0)
			oh := min(A.b.y1, B.b.y1) - max(A.b.y0, B.b.y0)
			if ow <= 0 || oh <= 0 {
				continue
			}
			smaller := min((A.b.x1-A.b.x0)*(A.b.y1-A.b.y0), (B.b.x1-B.b.x0)*(B.b.y1-B.b.y0))
			if smaller > 0 && ow*oh > smaller*0.25 {
				errs = append(errs, fmt.Sprintf("元素%d(%s) 与 元素%d(%s) 重叠 %.0f%%",
					A.i+1, s.Elements[A.i].Kind, B.i+1, s.Elements[B.i].Kind, ow*oh/smaller*100))
			}
		}
	}
	return errs
}

type specBox struct{ x0, y0, x1, y1 float64 }

func boxOf(e *SpecElement, cv Canvas) (specBox, bool) {
	x0, y0, w, h, ok := e.bbox(cv)
	return specBox{x0, y0, x0 + w, y0 + h}, ok
}

// nestedShapes disc/circle 全包含嵌套（同心构图豁免）。
func nestedShapes(a *SpecElement, A specBox, b *SpecElement, B specBox) bool {
	shape := func(e *SpecElement) bool { return e.Kind == "disc" || e.Kind == "circle" }
	if !shape(a) || !shape(b) {
		return false
	}
	in := func(outer, inner specBox) bool {
		return inner.x0 >= outer.x0 && inner.y0 >= outer.y0 && inner.x1 <= outer.x1 && inner.y1 <= outer.y1
	}
	return in(A, B) || in(B, A)
}

// SanitizeSpec 保底清洗：越界坐标夹进安全区，仍冲突的元素成对删后进者。
// 用于模型多轮修复仍不过校验时——确定性降级优于任务失败。
func SanitizeSpec(s *CompSpec, wordCount int, cv Canvas) []string {
	for i := range s.Elements {
		e := &s.Elements[i]
		if e.Reveal < 0 {
			e.Reveal = 0
		}
		if wordCount > 0 && e.Reveal >= wordCount {
			e.Reveal = wordCount - 1
		}
		// 文本/条状类夹进安全区上半段；圆类圆心留出半径余量
		if e.Kind == "note" || e.Kind == "label" || e.Kind == "big" || e.Kind == "beam" {
			e.X = clamp(e.X, cv.X0+100, cv.X1-260)
			e.Y = clamp(e.Y, cv.Y0+60, cv.Y1-120)
		}
		if e.Kind == "disc" || e.Kind == "circle" {
			e.Cx = clamp(e.Cx, cv.X0+180, cv.X1-180)
			e.Cy = clamp(e.Cy, cv.Y0+160, cv.Y1-260)
		}
	}
	// 删除仍互相重叠的元素（每轮删编号最大的冲突方，保留先出现的）
	for len(s.Elements) > 3 {
		victim := -1
		for a := 0; a < len(s.Elements) && victim < 0; a++ {
			if s.Elements[a].Kind == "arrow" {
				continue
			}
			A, okA := boxOf(&s.Elements[a], cv)
			if !okA {
				continue
			}
			for b := a + 1; b < len(s.Elements); b++ {
				if s.Elements[b].Kind == "arrow" {
					continue
				}
				B, okB := boxOf(&s.Elements[b], cv)
				if !okB || nestedShapes(&s.Elements[a], A, &s.Elements[b], B) {
					continue
				}
				ow := min(A.x1, B.x1) - max(A.x0, B.x0)
				oh := min(A.y1, B.y1) - max(A.y0, B.y0)
				smaller := min((A.x1-A.x0)*(A.y1-A.y0), (B.x1-B.x0)*(B.y1-B.y0))
				if ow > 0 && oh > 0 && smaller > 0 && ow*oh > smaller*0.25 {
					victim = b // 删后进者；外层循环停止
					break
				}
			}
		}
		if victim < 0 {
			break
		}
		s.Elements = append(s.Elements[:victim], s.Elements[victim+1:]...)
	}
	return ValidateSpec(s, wordCount, cv)
}

func clamp(v, lo, hi float64) float64 { return min(max(v, lo), hi) }
