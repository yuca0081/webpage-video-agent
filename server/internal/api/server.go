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
)

type Server struct {
	DataDir  string
	RootDir  string
	Store    store.Store
	Hub      *events.Hub
	Agent    *agent.Agent
	Producer *produce.Runner
	Lib      *methodlib.Library
}

func (s *Server) Router() *gin.Engine {
	r := gin.Default()
	r.MaxMultipartMemory = 8 << 20

	r.POST("/api/projects", s.createProject)
	r.GET("/api/projects", s.listProjects)
	r.GET("/api/projects/:id", s.projectView)
	r.GET("/api/projects/:id/manuscript", s.manuscript)
	r.GET("/api/projects/:id/storyboard", s.storyboard)
	r.GET("/api/projects/:id/style", s.style)
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
		Manuscript  string `json:"manuscript"`
		Aspect      string `json:"aspect"`      // 9:16（默认，plan §7 M1）| 16:9
		StylePackID string `json:"stylepack_id"` // 方法库复用（可空 = 冷启动自拟）
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	in.Manuscript = strings.TrimSpace(in.Manuscript)
	if in.Manuscript == "" {
		c.JSON(400, gin.H{"error": "文稿不能为空（硬门 1/3）"})
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
		in.Name = strings.SplitN(in.Manuscript, "\n", 2)[0]
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
	if err := os.WriteFile(p.Artifact("input/article.txt"), []byte(in.Manuscript), 0o644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	meta, _ := json.MarshalIndent(map[string]any{
		"id": id, "name": in.Name, "status": "created", "aspect": in.Aspect,
		"stylepack_id": in.StylePackID,
		"created_at":   time.Now().Format(time.RFC3339),
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
	if b, err := os.ReadFile(p.Artifact("project.json")); err == nil {
		var meta struct {
			Aspect string `json:"aspect"`
		}
		if json.Unmarshal(b, &meta) == nil && meta.Aspect != "" {
			aspect = meta.Aspect
		}
	}
	c.JSON(200, gin.H{
		"id": id, "name": st.Name, "aspect": aspect,
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
		Content string       `json:"content"`
		Refs    []store.Ref  `json:"refs"` // 引用卡片（段/元素，M2 起结构化）
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
