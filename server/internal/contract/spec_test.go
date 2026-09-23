package contract

import (
	"strings"
	"testing"
)

func TestValidateSpec16x9(t *testing.T) {
	cv := CanvasFor("16:9")
	if cv.W != 1920 || cv.H != 1080 || cv.CapTop != 896 {
		t.Fatalf("16:9 画幅几何不对: %+v", cv)
	}
	s := &CompSpec{Elements: []SpecElement{
		{Kind: "title", Y: 200, Text: "为什么猫摔不死"},
		{Kind: "note", X: 300, Y: 400, Text: "翻正反射", Bg: "butter"},
		{Kind: "disc", Cx: 1200, Cy: 500, R: 120, Bg: "mint"},
	}}
	if errs := ValidateSpec(s, 20, cv); len(errs) != 0 {
		t.Fatalf("合法布局被拒: %v", errs)
	}
	// 越界（9:16 坐标放进 16:9 画布）
	bad := &CompSpec{Elements: []SpecElement{
		{Kind: "note", X: 1000, Y: 1400, Text: "越界便签"},
		{Kind: "label", X: 300, Y: 400, Text: "标注"},
		{Kind: "big", X: 500, Y: 600, Text: "100"},
	}}
	errs := ValidateSpec(bad, 20, cv)
	if len(errs) != 1 || !strings.Contains(errs[0], "越出安全区") {
		t.Fatalf("应检出 1 处越界，得到: %v", errs)
	}
}

func TestValidateSpec9x16(t *testing.T) {
	cv := CanvasFor("9:16")
	if cv.W != 1080 || cv.H != 1920 || cv.CapTop != 1594 {
		t.Fatalf("9:16 画幅几何不对: %+v", cv)
	}
	// 纵向铺排合法
	s := &CompSpec{Elements: []SpecElement{
		{Kind: "title", Y: 260, Text: "三分钟看懂"},
		{Kind: "note", X: 160, Y: 600, Text: "第一层", Bg: "sky"},
		{Kind: "note", X: 160, Y: 900, Text: "第二层", Bg: "butter"},
		{Kind: "arrow", X1: 400, Y1: 700, X2: 400, Y2: 880},
	}}
	if errs := ValidateSpec(s, 30, cv); len(errs) != 0 {
		t.Fatalf("竖屏合法布局被拒: %v", errs)
	}
	// 16:9 的横向坐标在竖屏越界
	bad := &CompSpec{Elements: []SpecElement{
		{Kind: "note", X: 1200, Y: 400, Text: "横屏便签"},
		{Kind: "label", X: 200, Y: 700, Text: "标注"},
		{Kind: "big", X: 300, Y: 1000, Text: "42"},
	}}
	errs := ValidateSpec(bad, 30, cv)
	if len(errs) == 0 || !strings.Contains(errs[0], "越出安全区") {
		t.Fatalf("竖屏应检出越界，得到: %v", errs)
	}
}

func TestSanitizeSpecClamps(t *testing.T) {
	cv := CanvasFor("9:16")
	s := &CompSpec{Elements: []SpecElement{
		{Kind: "note", X: 5000, Y: -300, Text: "飞出画布", Reveal: 3},
		{Kind: "disc", Cx: 9000, Cy: 9000, R: 100, Reveal: 5},
		{Kind: "label", X: 200, Y: 300, Text: "正常", Reveal: 1},
	}}
	_ = SanitizeSpec(s, 10, cv)
	if s.Elements[0].X > cv.X1 || s.Elements[1].Cx > cv.X1 {
		t.Fatalf("清洗后仍越界: %+v %+v", s.Elements[0], s.Elements[1])
	}
}

func TestValidateSpecIconChart(t *testing.T) {
	cv := CanvasFor("16:9")
	// icon + 图表合法布局
	s := &CompSpec{Elements: []SpecElement{
		{Kind: "icon", X: 700, Y: 380, Size: 200, Name: "rocket", Reveal: 2},
		{Kind: "label", X: 300, Y: 250, Text: "运载火箭", Reveal: 1},
		{Kind: "chart_bar", X: 160, Y: 480, W: 520, H: 320,
			Values: []float64{12, 30, 8}, Labels: []string{"甲", "乙", "丙"}, Reveal: 5},
	}}
	if errs := ValidateSpec(s, 20, cv); len(errs) != 0 {
		t.Fatalf("icon/图表合法布局被拒: %v", errs)
	}
	// icon 缺 name；图表 values/labels 不一致 + 单值折线
	bad := &CompSpec{Elements: []SpecElement{
		{Kind: "icon", X: 700, Y: 380, Size: 200, Reveal: 1},
		{Kind: "chart_bar", X: 160, Y: 480, Values: []float64{1, 2}, Labels: []string{"a"}, Reveal: 2},
		{Kind: "chart_line", X: 160, Y: 480, Values: []float64{5}, Reveal: 3},
	}}
	errs := ValidateSpec(bad, 20, cv)
	joined := strings.Join(errs, "\n")
	for _, want := range []string{"缺 name", "labels 数 1 与 values 数 2 不一致", "数量 1 不在 3–8"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("应包含 %q，得到: %v", want, errs)
		}
	}
}
