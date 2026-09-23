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
