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

func TestValidateSpecPanelChip(t *testing.T) {
	cv := CanvasFor("16:9")
	// panel + chip 合法布局
	s := &CompSpec{Elements: []SpecElement{
		{Kind: "panel", X: 300, Y: 300, W: 620, H: 380, Title: "必须保留的部分",
			Text: "厚描边卡片承载说明文字", Bg: "green", Reveal: 1},
		{Kind: "chip", X: 1100, Y: 340, Text: "虚拟机", Bg: "navy", Reveal: 3},
		{Kind: "arrow", X1: 1050, Y1: 300, X2: 1180, Y2: 380, Reveal: 4},
	}}
	if errs := ValidateSpec(s, 20, cv); len(errs) != 0 {
		t.Fatalf("panel/chip 合法布局被拒: %v", errs)
	}
	// panel 缺 title/text；title 超长；chip 缺 text；w 超范围
	bad := &CompSpec{Elements: []SpecElement{
		{Kind: "panel", X: 300, Y: 300, W: 2000, Reveal: 1},
		{Kind: "chip", X: 1100, Y: 340, Reveal: 2},
		{Kind: "label", X: 300, Y: 800, Text: "占位", Reveal: 3},
	}}
	errs := ValidateSpec(bad, 20, cv)
	joined := strings.Join(errs, "\n")
	for _, want := range []string{"缺 title/text", "超范围 240–1400", "缺 text"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("应包含 %q，得到: %v", want, errs)
		}
	}
}

func TestValidateSpecDiagramKinds(t *testing.T) {
	cv := CanvasFor("16:9")
	// zone 垫底 + timeline/bracket/strip/barrow/table 合法布局；
	// 卡片在 zone 内部不算重叠（zone 豁免）
	s := &CompSpec{Elements: []SpecElement{
		{Kind: "zone", X: 1060, Y: 260, W: 700, H: 560, Bg: "mint", Reveal: 0},
		{Kind: "timeline", X: 1160, Y: 300, Nodes: []string{"初始化服务", "启动操作系统", "执行命令"},
			Color: "green", Reveal: 1},
		{Kind: "bracket", X: 1540, Y: 300, H: 420, Text: "多次重复执行", Color: "green", Reveal: 4},
		{Kind: "strip", X: 300, Y: 300, N: 6, Color: "sky", Text: "512维", Reveal: 2},
		{Kind: "barrow", X: 420, Y: 450, W: 300, H: 100, Color: "sky", Reveal: 3},
		{Kind: "table", X: 300, Y: 620, W: 560,
			Rows: [][]string{{"0.21", "-0.11"}, {"0.73", "-0.23"}, {"0.56", "0.22"}}, Reveal: 5},
	}}
	if errs := ValidateSpec(s, 30, cv); len(errs) != 0 {
		t.Fatalf("图解积木合法布局被拒: %v", errs)
	}
	// 违规：timeline 节点超 6；table 行列不一致；strip n 超范围
	bad := &CompSpec{Elements: []SpecElement{
		{Kind: "timeline", X: 300, Y: 200,
			Nodes: []string{"一", "二", "三", "四", "五", "六", "七"}, Reveal: 1},
		{Kind: "table", X: 300, Y: 400, Rows: [][]string{{"a", "b"}, {"c"}}, Reveal: 2},
		{Kind: "strip", X: 300, Y: 600, N: 9, Reveal: 3},
	}}
	errs := ValidateSpec(bad, 30, cv)
	joined := strings.Join(errs, "\n")
	for _, want := range []string{"nodes 7 条不在 2–6", "列数须一致", "n 9 不在 2–8"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("应包含 %q，得到: %v", want, errs)
		}
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
