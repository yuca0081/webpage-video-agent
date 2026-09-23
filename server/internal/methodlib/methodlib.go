// Package methodlib：方法库 v1（plan.md §4.6）。
// StylePack 条目存 data/library/stylepacks.json（磁盘为事实源，与项目产物同哲学）：
// 成片出片时 Agent 起草（published=false）→ 用户一键确认入库（published=true）
// → 新建项目可选用 → draft_style_samples 直接复用该包样张。
// 冷启动零预置：所有条目都来自真实成片。
package methodlib

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// StylePack 从成片提炼的风格包（v1 = 方向名 + 样张 + 说明，token 级深挖后续）。
type StylePack struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Direction     string    `json:"direction"` // 与 style_samples.direction 同源
	Desc          string    `json:"desc"`
	SampleHTML    string    `json:"sample_html"` // 首张样张（所见即所得）
	SampleTag     string    `json:"sample_tag"`
	OriginProject string    `json:"origin_project"`
	Published     bool      `json:"published"`
	CreatedAt     time.Time `json:"created_at"`
}

type Library struct {
	dir string // data/library
}

func New(dataDir string) *Library {
	return &Library{dir: filepath.Join(dataDir, "library")}
}

func (l *Library) path() string { return filepath.Join(l.dir, "stylepacks.json") }

// List 全部条目，新在前。
func (l *Library) List() ([]StylePack, error) {
	var packs []StylePack
	b, err := os.ReadFile(l.path())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &packs); err != nil {
		return nil, fmt.Errorf("stylepacks.json 损坏: %w", err)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].CreatedAt.After(packs[j].CreatedAt) })
	return packs, nil
}

// DraftFromProject 从已出片项目起草风格包（Agent 自动，published=false 等用户确认）。
// 同一项目重复起草 = 幂等返回已有条目。
func (l *Library) DraftFromProject(projectDir, projectID, projectName string) (*StylePack, error) {
	var ss struct {
		Direction string `json:"direction"`
		Samples   []struct {
			Tag  string `json:"tag"`
			HTML string `json:"html"`
		} `json:"samples"`
	}
	b, err := os.ReadFile(filepath.Join(projectDir, "style", "style_samples.json"))
	if err != nil {
		return nil, fmt.Errorf("项目无风格样张: %w", err)
	}
	if err := json.Unmarshal(b, &ss); err != nil {
		return nil, err
	}
	if ss.Direction == "" || len(ss.Samples) == 0 {
		return nil, fmt.Errorf("风格样张不完整（direction/样本缺失）")
	}

	packs, _ := l.List()
	for i := range packs {
		if packs[i].OriginProject == projectID {
			return &packs[i], nil // 已起草过
		}
	}

	pack := StylePack{
		ID:            "sp" + time.Now().Format("20060102-150405"),
		Name:          ss.Direction,
		Direction:     ss.Direction,
		Desc:          fmt.Sprintf("从「%s」成片提炼；色板/组件与该成片同一套 token", projectName),
		SampleHTML:    ss.Samples[0].HTML,
		SampleTag:     ss.Samples[0].Tag,
		OriginProject: projectID,
		Published:     false,
		CreatedAt:     time.Now(),
	}
	packs = append(packs, pack)
	if err := l.save(packs); err != nil {
		return nil, err
	}
	return &pack, nil
}

// Publish 用户确认入库。
func (l *Library) Publish(id string) (*StylePack, error) {
	packs, err := l.List()
	if err != nil {
		return nil, err
	}
	for i := range packs {
		if packs[i].ID == id {
			packs[i].Published = true
			return &packs[i], l.save(packs)
		}
	}
	return nil, fmt.Errorf("方法库没有 %s", id)
}

// Get 取单个条目（新建项目选用后查详情）。
func (l *Library) Get(id string) (*StylePack, bool) {
	packs, _ := l.List()
	for i := range packs {
		if packs[i].ID == id {
			return &packs[i], true
		}
	}
	return nil, false
}

// StyleSamplesJSON 把风格包变成本项目可用的 style_samples.json 内容（confirmed=false 等用户点头）。
func (p *StylePack) StyleSamplesJSON() []byte {
	type sample struct {
		Tag  string `json:"tag"`
		Desc string `json:"desc"`
		HTML string `json:"html"`
	}
	out := struct {
		Direction string   `json:"direction"`
		Desc      string   `json:"desc"`
		Samples   []sample `json:"samples"`
		Confirmed bool     `json:"confirmed"`
	}{
		Direction: p.Direction,
		Desc:      p.Desc,
		Samples:   []sample{{Tag: p.SampleTag, Desc: "方法库复用样张", HTML: p.SampleHTML}},
		Confirmed: false,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return b
}

func (l *Library) save(packs []StylePack) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(packs, "", "  ")
	return os.WriteFile(l.path(), b, 0o644)
}
