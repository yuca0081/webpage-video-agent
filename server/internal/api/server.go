// Package api：HTTP 层（Gin）。REST + SSE；磁盘是产物事实源，PG 存案卷。
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"webpage-video-agent/server/internal/agent"
	"webpage-video-agent/server/internal/contract"
	"webpage-video-agent/server/internal/events"
	"webpage-video-agent/server/internal/llm"
	"webpage-video-agent/server/internal/methodlib"
	"webpage-video-agent/server/internal/pipeline"
	"webpage-video-agent/server/internal/produce"
	"webpage-video-agent/server/internal/store"
	"webpage-video-agent/server/internal/styleref"
)

type Server struct {
	DataDir  string
	RootDir  string
	Store    store.Store
	Hub      *events.Hub
	Agent    *agent.Agent
	Producer *produce.Runner
	Lib      *methodlib.Library
	Refs     *styleref.Store
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	r.MaxMultipartMemory = 8 << 20

	r.POST("/api/projects", s.createProject)
	r.GET("/api/projects", s.listProjects)
	r.GET("/api/projects/:id", s.projectView)
	r.GET("/api/projects/:id/manuscript", s.manuscript)
	r.PUT("/api/projects/:id/manuscript", s.saveManuscript)
	r.GET("/api/projects/:id/storyboard", s.storyboard)
	r.GET("/api/projects/:id/style", s.style)
	r.POST("/api/projects/:id/style/apply", s.applyStyle)
	r.POST("/api/projects/:id/style/confirm", s.confirmStyle)
	r.POST("/api/projects/:id/chat", s.chat)
	r.POST("/api/draft/manuscript", s.draftManuscript)
	r.GET("/api/projects/:id/messages", s.messages)
	r.GET("/api/projects/:id/events", s.sse)
	r.POST("/api/projects/:id/produce", s.startProduce)
	r.POST("/api/projects/:id/cancel", s.cancelProduce)
	r.GET("/api/projects/:id/video/main.mp4", s.video)
	r.GET("/api/projects/:id/audio_meta", s.audioMeta)
	r.GET("/api/projects/:id/frames/:seg", s.frame)
	r.GET("/api/projects/:id/assets/*filepath", s.asset)
	r.GET("/api/library", s.listLibrary)
	r.POST("/api/library/:id/publish", s.publishPack)
	r.GET("/api/registry", s.registry)
	r.GET("/api/gallery", s.gallery)
	r.GET("/api/gallery/:style/:file", s.galleryFile)
	r.POST("/api/references", s.uploadReference)
	r.GET("/api/references", s.listReferences)
	r.POST("/api/references/:id/analyze", s.analyzeReference)
	r.POST("/api/references/:id/confirm", s.confirmReference)

	// 前端产物（web/dist）存在则托管
	dist := filepath.Join(s.RootDir, "web", "dist")
	if st, err := os.Stat(dist); err == nil && st.IsDir() {
		r.Static("/assets", filepath.Join(dist, "assets"))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			// SPA 入口不缓存：发版后浏览器立刻拿到新壳（带 hash 的 assets 才长期缓存）
			c.Header("Cache-Control", "no-store")
			c.File(filepath.Join(dist, "index.html"))
		})
	}
	return r
}

// ── 项目 ─────────────────────────────────────────────────────

func (s *Server) createProject(c *gin.Context) {
	var in struct {
		Name        string `json:"name"`
		Topic       string `json:"topic"`        // 一句话主题（文稿弹窗 AI 起草的种子）
		Manuscript  string `json:"manuscript"`   // 可选：直接带稿创建（不走弹窗补）
		Aspect      string `json:"aspect"`       // 9:16（默认，plan §7 M1）| 16:9
		StylePackID string `json:"stylepack_id"` // 方法库复用（可空 = 冷启动自拟）
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	in.Topic = strings.TrimSpace(in.Topic)
	in.Manuscript = strings.TrimSpace(in.Manuscript)
	// 新建只要求名称+主题（文稿/风格是三前置，进工作区后在对话栏标签里补）；
	// 带稿直建是老路径，兼容保留。
	if in.Topic == "" && in.Manuscript == "" {
		c.JSON(400, gin.H{"error": "主题必填（一句话说清要讲什么，例：蚊子为什么嗡嗡叫）"})
		return
	}
	if in.Aspect != "16:9" && in.Aspect != "9:16" {
		in.Aspect = "9:16"
	}
	if in.StylePackID != "" {
		if pack, ok := s.Lib.Get(in.StylePackID); !ok || !pack.Published {
			c.JSON(400, gin.H{"error": "风格包不存在或未发布"})
			return
		}
	}
	if in.Name == "" {
		src := in.Topic
		if src == "" {
			src = strings.SplitN(in.Manuscript, "\n", 2)[0]
		}
		in.Name = src
		if r := []rune(in.Name); len(r) > 24 {
			in.Name = string(r[:24])
		}
	}
	id := "p" + time.Now().Format("20060102-150405")
	p := pipeline.NewProject(s.DataDir, id)
	for _, d := range []string{
		"input", "llm", "manuscripts", "storyboards", "style",
		"audio", "compositions/frames", "compositions/registry", "renders",
	} {
		_ = os.MkdirAll(filepath.Join(p.Dir, d), 0o755)
	}
	if in.Manuscript != "" {
		if err := os.WriteFile(p.Artifact("input/article.txt"), []byte(in.Manuscript), 0o644); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	meta, _ := json.MarshalIndent(map[string]any{
		"id": id, "name": in.Name, "status": "created", "aspect": in.Aspect,
		"topic": in.Topic, "stylepack_id": in.StylePackID,
		"created_at": time.Now().Format(time.RFC3339),
	}, "", "  ")
	_ = os.WriteFile(p.Artifact("project.json"), meta, 0o644)
	p.Manifest("project.created", fmt.Sprintf("%s（%s）", in.Name, in.Aspect))
	_ = s.Store.CreateProject(id, in.Name)
	s.Hub.Emit(id, "msg", "", "项目已创建："+in.Name)
	c.JSON(200, gin.H{"id": id, "name": in.Name, "aspect": in.Aspect})
}

func (s *Server) listProjects(c *gin.Context) {
	// 扫磁盘（事实源），确保 DB 有行；合并状态
	entries, _ := os.ReadDir(filepath.Join(s.DataDir, "projects"))
	type row struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
		HasVideo  bool   `json:"has_video"`
	}
	var out []row
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		pj := filepath.Join(s.DataDir, "projects", e.Name(), "project.json")
		b, err := os.ReadFile(pj)
		if err != nil {
			continue
		}
		var meta struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
		}
		if json.Unmarshal(b, &meta) != nil || meta.Name == "" {
			continue
		}
		_ = s.Store.EnsureProject(e.Name(), meta.Name)
		_, hasVideo := os.Stat(filepath.Join(s.DataDir, "projects", e.Name(), "renders", "main.mp4"))
		if s.Producer.IsRunning(e.Name()) {
			meta.Status = "producing"
		} else if hasVideo == nil {
			meta.Status = "video"
		}
		out = append(out, row{e.Name(), meta.Name, meta.Status, meta.CreatedAt, hasVideo == nil})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	c.JSON(200, out)
}

func (s *Server) projectView(c *gin.Context) {
	id := c.Param("id")
	p := pipeline.NewProject(s.DataDir, id)
	if _, err := os.Stat(p.Dir); err != nil {
		c.JSON(404, gin.H{"error": "项目不存在"})
		return
	}
	st := s.Agent.State(id)
	aspect := "16:9" // 旧项目无字段，与已渲成片一致
	topic := ""
	if b, err := os.ReadFile(p.Artifact("project.json")); err == nil {
		var meta struct {
			Aspect string `json:"aspect"`
			Topic  string `json:"topic"`
		}
		if json.Unmarshal(b, &meta) == nil {
			if meta.Aspect != "" {
				aspect = meta.Aspect
			}
			topic = meta.Topic
		}
	}
	c.JSON(200, gin.H{
		"id": id, "name": st.Name, "aspect": aspect, "topic": topic,
		"gates": gin.H{
			"manuscript": st.HasManuscript, "storyboard": st.HasStoryboard, "style_confirmed": st.StyleConfirmed,
			"style_draft": st.HasStyle, "style_direction": st.StyleDirection,
		},
		"seg_count": st.SegCount, "total_hint": st.TotalHint,
		"has_video": st.HasVideo, "producing": st.Producing,
	})
}

func (s *Server) manuscript(c *gin.Context) {
	p := pipeline.NewProject(s.DataDir, c.Param("id"))
	var m struct {
		Content   string `json:"content"`
		WordCount int    `json:"word_count"`
	}
	if b, err := os.ReadFile(p.Artifact("manuscripts/manuscript.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	} else if b, err := os.ReadFile(p.Artifact("input/article.txt")); err == nil {
		m.Content = strings.TrimSpace(string(b))
		m.WordCount = len([]rune(m.Content))
	} else {
		c.JSON(404, gin.H{"error": "文稿不存在"})
		return
	}
	c.JSON(200, m)
}

// saveManuscript 文稿弹窗保存。article.txt 是唯一真源；manuscript.json 是管线产物
// （重放语义：存在即跳过）——改稿必须删掉它，否则重新制作时 TTS 拿的还是旧稿。
// 已有分镜不硬删：响应带 storyboard_stale，由前端提示"分镜基于旧稿，需重新生成"。
func (s *Server) saveManuscript(c *gin.Context) {
	var in struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		c.JSON(400, gin.H{"error": "文稿不能为空"})
		return
	}
	id := c.Param("id")
	p := pipeline.NewProject(s.DataDir, id)
	if _, err := os.Stat(p.Dir); err != nil {
		c.JSON(404, gin.H{"error": "项目不存在"})
		return
	}
	if err := os.WriteFile(p.Artifact("input/article.txt"), []byte(content), 0o644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_ = os.Remove(p.Artifact("manuscripts/manuscript.json"))
	_, hasSb := os.Stat(p.Artifact("storyboards/storyboard.json"))
	s.updateStatus(id, "manuscript")
	words := len([]rune(content))
	p.Manifest("manuscript.saved", fmt.Sprintf("%d 字", words))
	s.Hub.Emit(id, "msg", "", fmt.Sprintf("文稿已更新（%d 字）", words))
	c.JSON(200, gin.H{"ok": true, "word_count": words, "storyboard_stale": hasSb == nil})
}

// applyStyle 风格弹窗选定库内风格包：写 style_samples.json 并直接标记已确认
// （弹窗里预览样张后显式点「确认使用」= 硬门通过，无需再回舞台点一遍）。
// 同时把选定包记入 project.json，后续 extract_stylepack / 复用链路读同一字段。
func (s *Server) applyStyle(c *gin.Context) {
	var in struct {
		StylePackID string `json:"stylepack_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.StylePackID) == "" {
		c.JSON(400, gin.H{"error": "缺少 stylepack_id"})
		return
	}
	pack, ok := s.Lib.Get(strings.TrimSpace(in.StylePackID))
	if !ok || !pack.Published {
		c.JSON(400, gin.H{"error": "风格包不存在或未发布"})
		return
	}
	id := c.Param("id")
	p := pipeline.NewProject(s.DataDir, id)
	if _, err := os.Stat(p.Dir); err != nil {
		c.JSON(404, gin.H{"error": "项目不存在"})
		return
	}
	if err := os.MkdirAll(filepath.Dir(p.Artifact("style/style_samples.json")), 0o755); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// 弹窗预览 + 显式确认一步完成：confirmed 翻 true（StyleSamplesJSON 默认 false 等舞台点头）
	var ss map[string]any
	if err := json.Unmarshal(pack.StyleSamplesJSON(), &ss); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	ss["confirmed"] = true
	if err := os.WriteFile(p.Artifact("style/style_samples.json"), mustJSON(ss), 0o644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if b, err := os.ReadFile(p.Artifact("project.json")); err == nil {
		var meta map[string]any
		if json.Unmarshal(b, &meta) == nil {
			meta["stylepack_id"] = pack.ID
			_ = os.WriteFile(p.Artifact("project.json"), mustJSON(meta), 0o644)
		}
	}
	s.updateStatus(id, "style")
	p.Manifest("style.applied", fmt.Sprintf("库内风格包「%s」", pack.Name))
	s.Hub.Emit(id, "style_confirmed", "", pack.Direction)
	c.JSON(200, gin.H{"ok": true, "direction": pack.Direction})
}

func (s *Server) storyboard(c *gin.Context) {
	p := pipeline.NewProject(s.DataDir, c.Param("id"))
	sb, err := p.LoadStoryboard()
	if err != nil {
		c.JSON(404, gin.H{"error": "分镜未生成"})
		return
	}
	c.Data(200, "application/json", mustJSON(sb))
}

func (s *Server) style(c *gin.Context) {
	p := pipeline.NewProject(s.DataDir, c.Param("id"))
	b, err := os.ReadFile(p.Artifact("style/style_samples.json"))
	if err != nil {
		c.JSON(404, gin.H{"error": "风格样张未出"})
		return
	}
	c.Data(200, "application/json", b)
}

// confirmStyle 用户在舞台点头确认风格——三前置硬门的唯一用户门。
func (s *Server) confirmStyle(c *gin.Context) {
	id := c.Param("id")
	p := pipeline.NewProject(s.DataDir, id)
	path := p.Artifact("style/style_samples.json")
	b, err := os.ReadFile(path)
	if err != nil {
		c.JSON(400, gin.H{"error": "样张未出，先让 Agent 出样张"})
		return
	}
	var ss contract.StyleSamples
	if err := json.Unmarshal(b, &ss); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	ss.Confirmed = true
	if err := os.WriteFile(path, mustJSON(ss), 0o644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	p.Manifest("style.confirmed", ss.Direction)
	s.updateStatus(id, "style")
	s.Hub.Emit(id, "style_confirmed", "", ss.Direction)
	c.JSON(200, gin.H{"ok": true, "direction": ss.Direction})
}

// ── 聊天 ─────────────────────────────────────────────────────

// draftManuscript 主题 → 口播文稿草稿（创建前 AI 辅助起草）。
// 文稿硬门不变：草稿仍进文稿框，用户可改可删，创建时照常校验非空。
func (s *Server) draftManuscript(c *gin.Context) {
	var in struct {
		Topic   string  `json:"topic"`
		Minutes float64 `json:"minutes"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Topic) == "" {
		c.JSON(400, gin.H{"error": "主题不能为空"})
		return
	}
	if in.Minutes <= 0 {
		in.Minutes = 1
	}
	if in.Minutes > 5 {
		in.Minutes = 5
	}
	p, err := llm.FromEnv(llm.RoleDialogue)
	if err != nil {
		c.JSON(503, gin.H{"error": "LLM 未配置（LLM_API_KEY），请直接粘贴文稿"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	words := int(in.Minutes * 60 * 4.2) // 口播语速 ≈ 4.2 字/秒
	var out struct {
		Content string `json:"content"`
	}
	system := "你是「帧述」的口播科普撰稿人，为短视频写口播文稿。硬性要求：" +
		"1) 第一句就是钩子（提问/反常识/画面感），不写标题和开场白；" +
		"2) 2~4 个递进的知识点，口语化短句，像讲给朋友听，少形容词堆砌；" +
		"3) 结尾一句收束或抛一个问题；" +
		"4) 只输出正文：不带标题、小节序号、markdown、emoji、舞台指示；" +
		"5) 数字和事实宁缺毋滥，不确定的不编。输出 JSON：{\"content\": \"文稿全文\"}。"
	user := fmt.Sprintf("主题：%s\n目标时长：约 %.0f 分钟（正文约 %d 字，±20%%）。", strings.TrimSpace(in.Topic), in.Minutes, words)
	if _, err := p.GenerateJSON(ctx, system, user, &out); err != nil {
		c.JSON(500, gin.H{"error": "起草失败：" + err.Error()})
		return
	}
	if strings.TrimSpace(out.Content) == "" {
		c.JSON(500, gin.H{"error": "起草结果为空，请重试或直接粘贴文稿"})
		return
	}
	c.JSON(200, gin.H{"content": strings.TrimSpace(out.Content)})
}

func (s *Server) chat(c *gin.Context) {
	id := c.Param("id")
	var in struct {
		Content string      `json:"content"`
		Refs    []store.Ref `json:"refs"` // 引用卡片（段/元素，M2 起结构化）
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Content) == "" {
		c.JSON(400, gin.H{"error": "消息不能为空"})
		return
	}
	if _, err := os.Stat(pipeline.NewProject(s.DataDir, id).Dir); err != nil {
		c.JSON(404, gin.H{"error": "项目不存在"})
		return
	}
	in.Refs = sanitizeRefs(in.Refs)
	m := &store.Msg{ProjectID: id, Role: "user", Type: "text", Content: in.Content, Refs: in.Refs}
	_ = s.Store.SaveMsg(m)
	s.Hub.Emit(id, "msg", "", in.Content)
	go s.Agent.Run(id, in.Content, in.Refs)
	c.JSON(202, gin.H{"ok": true})
}

// sanitizeRefs 引用卡片是模型上下文也是落盘数据：限量、限长、剔无效。
func sanitizeRefs(refs []store.Ref) []store.Ref {
	if len(refs) > 8 {
		refs = refs[:8]
	}
	out := refs[:0]
	for _, r := range refs {
		if r.Idx < 1 {
			continue
		}
		r.Key = clip(r.Key, 64)
		r.ElementID = clip(r.ElementID, 64)
		r.ElementName = clip(r.ElementName, 64)
		if r.T < 0 {
			r.T = 0
		}
		out = append(out, r)
	}
	return out
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (s *Server) messages(c *gin.Context) {
	id := c.Param("id")
	var after int64
	fmt.Sscanf(c.Query("after"), "%d", &after)
	msgs, err := s.Store.ListMsgs(id, after, 200)
	if err != nil {
		msgs = nil
	}
	c.JSON(200, msgs)
}

// ── 制作 & 视频 ─────────────────────────────────────────────

func (s *Server) startProduce(c *gin.Context) {
	id := c.Param("id")
	st := s.Agent.State(id)
	if !st.HasManuscript || !st.HasStoryboard || !st.StyleConfirmed {
		c.JSON(400, gin.H{"error": "三前置未齐（文稿/分镜/风格确认），硬门不放行"})
		return
	}
	ok, err := s.Producer.Start(id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	s.updateStatus(id, "producing")
	c.JSON(202, gin.H{"ok": true, "started": ok})
}

// cancelProduce 取消进行中的制作/重做（M1 遗留 cancel_production）。
// 已在收尾的任务可能来不及响应，返回 ok=true 表示取消信号已发出。
func (s *Server) cancelProduce(c *gin.Context) {
	if !s.Producer.Cancel(c.Param("id")) {
		c.JSON(409, gin.H{"error": "没有正在进行的制作任务"})
		return
	}
	c.JSON(202, gin.H{"ok": true})
}

// audioMeta 时间轴数据源：每段配音时长 + 字级时间戳（TTS 对齐产物），供前端画字幕/音频轨。
func (s *Server) audioMeta(c *gin.Context) {
	p := pipeline.NewProject(s.DataDir, c.Param("id"))
	b, err := os.ReadFile(p.Artifact("audio_meta.json"))
	if err != nil {
		c.JSON(404, gin.H{"error": "音频对齐数据不存在（未出配音）"})
		return
	}
	c.Data(200, "application/json", b)
}

func (s *Server) video(c *gin.Context) {
	path := pipeline.NewProject(s.DataDir, c.Param("id")).Artifact("renders/main.mp4")
	if _, err := os.Stat(path); err != nil {
		c.JSON(404, gin.H{"error": "成片未就绪"})
		return
	}
	c.Header("Cache-Control", "no-store")
	http.ServeContent(c.Writer, c.Request, "main.mp4", time.Now(), mustOpen(path))
}

// frame 修改态活合成物：单段帧 HTML（<template> 片段，plan §4.3 预览双模）。
// 前端 LiveFrame 以 srcdoc 壳消费：壳内引项目 assets/gsap.min.js 并自行激活时间轴。
func (s *Server) frame(c *gin.Context) {
	seg := c.Param("seg")
	if !isSegID(seg) {
		c.JSON(400, gin.H{"error": "非法段标识"})
		return
	}
	f, err := os.Open(pipeline.NewProject(s.DataDir, c.Param("id")).Artifact("compositions/frames/" + seg + ".html"))
	if err != nil {
		c.JSON(404, gin.H{"error": "合成物不存在"})
		return
	}
	defer f.Close()
	c.Header("Cache-Control", "no-store")
	http.ServeContent(c.Writer, c.Request, seg+".html", time.Now(), f)
}

// asset 项目资产（gsap.min.js 等）——活合成物壳的确定性依赖，渲染引擎同款本地 gsap，无 CDN。
func (s *Server) asset(c *gin.Context) {
	// path.Clean 以 "/" 锚定后 ".." 全部被解析掉，再校验只剩相对路径
	rel := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(c.Param("filepath"), "/")), "/")
	if rel == "" {
		c.JSON(400, gin.H{"error": "非法路径"})
		return
	}
	f, err := os.Open(filepath.Join(pipeline.NewProject(s.DataDir, c.Param("id")).Dir, "assets", filepath.FromSlash(rel)))
	if err != nil {
		c.JSON(404, gin.H{"error": "资产不存在"})
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	if st == nil || st.IsDir() {
		c.JSON(404, gin.H{"error": "资产不存在"})
		return
	}
	http.ServeContent(c.Writer, c.Request, st.Name(), time.Now(), f)
}

func isSegID(s string) bool {
	if !strings.HasPrefix(s, "seg") || len(s) == 3 {
		return false
	}
	for _, r := range s[3:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ── 方法库（plan.md §4.6：起草自动 + 入库确认）───────────────

func (s *Server) listLibrary(c *gin.Context) {
	packs, err := s.Lib.List()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, packs)
}

func (s *Server) publishPack(c *gin.Context) {
	pack, err := s.Lib.Publish(c.Param("id"))
	if err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, pack)
}

// ── 素材中心（元素注册表 / 风格注册表 / 样张矩阵）────────────

// registry 元素 + 风格注册表（styles 剥掉 tokens 只给元信息；elements 原样透传）。
func (s *Server) registry(c *gin.Context) {
	eb, err := os.ReadFile(filepath.Join(s.RootDir, "ai", "registry", "elements.json"))
	if err != nil {
		c.JSON(500, gin.H{"error": "元素注册表缺失：" + err.Error()})
		return
	}
	sb, err := os.ReadFile(filepath.Join(s.RootDir, "ai", "registry", "styles.json"))
	if err != nil {
		c.JSON(500, gin.H{"error": "风格注册表缺失：" + err.Error()})
		return
	}
	var styles []struct {
		ID        string   `json:"id"`
		Direction string   `json:"direction"`
		Genre     string   `json:"genre"`
		Keywords  []string `json:"keywords"`
		Photo     string   `json:"photo"`
	}
	if err := json.Unmarshal(sb, &styles); err != nil {
		c.JSON(500, gin.H{"error": "styles.json 损坏：" + err.Error()})
		return
	}
	c.JSON(200, gin.H{"elements": json.RawMessage(eb), "styles": styles})
}

// gallery 样张矩阵清单：{styleId: [kind...]}（_overview 也计入，前端作风格封面）。
func (s *Server) gallery(c *gin.Context) {
	dir := filepath.Join(s.DataDir, "gallery")
	out := map[string][]string{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var kinds []string
		files, _ := os.ReadDir(filepath.Join(dir, e.Name()))
		for _, f := range files {
			if name := strings.TrimSuffix(f.Name(), ".png"); name != f.Name() && name != "" {
				kinds = append(kinds, name)
			}
		}
		if len(kinds) > 0 {
			out[e.Name()] = kinds
		}
	}
	c.JSON(200, out)
}

// galleryFile 样张 PNG（重生成即换内容，短缓存即可）。
func (s *Server) galleryFile(c *gin.Context) {
	style, file := c.Param("style"), c.Param("file")
	if !isSlug(style) || !isPngName(file) {
		c.JSON(400, gin.H{"error": "非法路径"})
		return
	}
	f, err := os.Open(filepath.Join(s.DataDir, "gallery", style, file))
	if err != nil {
		c.JSON(404, gin.H{"error": "样张不存在"})
		return
	}
	defer f.Close()
	c.Header("Cache-Control", "max-age=60")
	http.ServeContent(c.Writer, c.Request, file, time.Now(), f)
}

func isSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func isPngName(s string) bool {
	if !strings.HasSuffix(s, ".png") || len(s) <= 4 {
		return false
	}
	return isSlug(strings.TrimSuffix(s, ".png"))
}

// ── SSE ──────────────────────────────────────────────────────

func (s *Server) sse(c *gin.Context) {
	id := c.Param("id")
	ch, replay, cancel := s.Hub.Subscribe(id)
	defer cancel()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	fl, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(500, gin.H{"error": "streaming unsupported"})
		return
	}
	io.WriteString(c.Writer, "retry: 3000\n\n")
	for _, ev := range replay { // 断线重连补最近事件
		io.WriteString(c.Writer, "event: "+ev.Type+"\ndata: "+ev.JSON()+"\n\n")
	}
	fl.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case ev, alive := <-ch:
			if !alive {
				return
			}
			io.WriteString(c.Writer, "event: "+ev.Type+"\ndata: "+ev.JSON()+"\n\n")
			fl.Flush()
		case <-heartbeat.C:
			io.WriteString(c.Writer, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) updateStatus(id, status string) {
	p := pipeline.NewProject(s.DataDir, id)
	if b, err := os.ReadFile(p.Artifact("project.json")); err == nil {
		var meta map[string]any
		if json.Unmarshal(b, &meta) == nil {
			meta["status"] = status
			_ = os.WriteFile(p.Artifact("project.json"), mustJSON(meta), 0o644)
		}
	}
	_ = s.Store.UpdateStatus(id, status)
}

func mustJSON(v any) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return b
}

func mustOpen(path string) *os.File {
	f, _ := os.Open(path)
	return f
}

// ── 参考视频解析（素材中心「视频解析」）────────────────────────

// uploadReference 上传视频 → 落盘登记 → 立即异步解析。
func (s *Server) uploadReference(c *gin.Context) {
	fh, err := c.FormFile("video")
	if err != nil {
		c.JSON(400, gin.H{"error": "缺少 video 文件字段"})
		return
	}
	name := filepath.Base(fh.Filename)
	if !strings.ContainsAny(name, ".") {
		c.JSON(400, gin.H{"error": "文件名要有扩展名（mp4/mov/webm 等）"})
		return
	}
	ref, err := s.Refs.Save(name, func(dst string) error {
		return c.SaveUploadedFile(fh, dst)
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_, _ = s.Refs.Analyze(ref.ID) // 上传即解析；状态轮询 GET /api/references
	c.JSON(200, ref)
}

func (s *Server) listReferences(c *gin.Context) {
	refs, err := s.Refs.List()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if refs == nil {
		refs = []styleref.Ref{}
	}
	c.JSON(200, refs)
}

// analyzeReference 解析（重试入口；上传时已自动起过一次）。
func (s *Server) analyzeReference(c *gin.Context) {
	started, err := s.Refs.Analyze(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if !started {
		c.JSON(409, gin.H{"error": "已在解析中"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// confirmReference 解析结果入库：styles.json 注册（引擎吃 tokens）+ 方法库 pack 草稿。
func (s *Server) confirmReference(c *gin.Context) {
	ref, err := s.Refs.Get(c.Param("id"))
	if err != nil {
		c.JSON(404, gin.H{"error": "引用不存在"})
		return
	}
	if ref.Status != "done" || ref.Style == nil {
		c.JSON(400, gin.H{"error": "还没解析完成，不能入库"})
		return
	}
	regID := styleref.RegistryID(ref.ID)
	if err := styleref.AppendRegistry(s.RootDir, regID, ref.Style); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	desc := ref.Style.Genre
	if desc != "" {
		desc = "从参考视频「" + ref.Name + "」解析提炼；" + desc
	} else {
		desc = "从参考视频「" + ref.Name + "」解析提炼"
	}
	pack, err := s.Lib.AddVideoPack(ref.ID, ref.Style.Direction, ref.Style.Direction, desc,
		ref.Style.Samples[0].HTML, ref.Style.Samples[0].Tag)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := s.Refs.MarkConfirmed(ref.ID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "registry_id": regID, "pack": pack})
}
