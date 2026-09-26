package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"webpage-video-agent/server/internal/contract"
	"webpage-video-agent/server/internal/llm"
)

// ── 错误语义 ──────────────────────────────────────────────────────

// ErrAwaitLLM 会话模式（LLM_MODE=manual）：请求文件已写好，
// 等待产物文件出现后重跑继续（每步落盘、从任意步重放）。
type ErrAwaitLLM struct {
	RequestPath string
	OutputPath  string
	Note        string
}

func (e *ErrAwaitLLM) Error() string {
	note := e.Note
	if note == "" {
		note = "填写产物后重跑 m0 run 继续"
	}
	return fmt.Sprintf("等待 LLM 产物 %s（请求见 %s；%s）", e.OutputPath, e.RequestPath, note)
}

// ErrTodo 阶段属后续工作（TTS 引擎接入、hyperframes 渲染环境等），骨架先占位。
var ErrTodo = errors.New("阶段待实现（后续里程碑）")

// ── 项目 ──────────────────────────────────────────────────────────

type Project struct {
	ID  string
	Dir string
	// Ctx 任务级取消信号（pipeline.Run 注入）；runCLI 等长作业用其杀子进程。
	// 为 nil 时视为 context.Background()（CLI 直跑 m0 run 场景）。
	Ctx context.Context

	manifestMu sync.Mutex // 并发作业（spec 并行生成）下 manifest.jsonl 追加互斥
}

func NewProject(dataDir, id string) *Project {
	return &Project{ID: id, Dir: filepath.Join(dataDir, "projects", id)}
}

func (p *Project) Artifact(rel string) string     { return filepath.Join(p.Dir, rel) }
func (p *Project) LLMRequest(name string) string  { return filepath.Join(p.Dir, "llm", name+".request.md") }
func (p *Project) LLMOutput(name string) string   { return filepath.Join(p.Dir, "llm", name+".json") }
func (p *Project) LoadStoryboard() (*contract.Storyboard, error) {
	b, err := os.ReadFile(p.Artifact("storyboards/storyboard.json"))
	if err != nil {
		return nil, err
	}
	var sb contract.Storyboard
	if err := json.Unmarshal(b, &sb); err != nil {
		return nil, fmt.Errorf("storyboard.json 解析失败: %w", err)
	}
	return &sb, nil
}

// Manifest 每步事件落盘（审计与重放依据）。
func (p *Project) Manifest(event, detail string) {
	f := p.Artifact("manifest.jsonl")
	line, _ := json.Marshal(map[string]any{
		"ts": time.Now().Format(time.RFC3339), "event": event, "detail": detail,
	})
	p.manifestMu.Lock()
	defer p.manifestMu.Unlock()
	fh, err := os.OpenFile(f, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	_, _ = fh.Write(append(line, '\n'))
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func writeFile(dir string, path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s 解析失败: %w", filepath.Base(path), err)
	}
	return nil
}

// ── 阶段定义 ──────────────────────────────────────────────────────

type Stage struct {
	Name string
	Run  func(p *Project) error
}

// Stages M0 管线：固定顺序 = 依赖方向（音频先于画面、基准先于适配）。
func Stages() []Stage {
	return []Stage{
		{Name: "manuscript", Run: stageManuscript},
		{Name: "storyboard", Run: stageStoryboard},
		{Name: "style_samples", Run: stageStyleSamples},
		{Name: "tts", Run: stageTTS},
		{Name: "compositions", Run: stageCompositions},
		{Name: "check", Run: stageCheck},
		{Name: "render", Run: stageRender},
		{Name: "stitch", Run: stageStitch},
	}
}

// Run 顺序执行阶段；遇 等待LLM/待实现/错误 即停（产物已落盘，重跑续上）。
// until 非空时执行到该阶段为止（含）。ctx 取消时在阶段边界退出（渲染中的
// 子进程由 runCLI 经 p.Ctx 连带杀掉）。
func Run(ctx context.Context, p *Project, until string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	p.Ctx = ctx
	for _, st := range Stages() {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Printf("▶ %-14s ", st.Name)
		err := st.Run(p)
		var await *ErrAwaitLLM
		switch {
		case err == nil:
			fmt.Println("✓")
			p.Manifest("stage.done", st.Name)
		case errors.As(err, &await):
			fmt.Println("⏸")
			fmt.Println("   ", err.Error())
			p.Manifest("stage.await", st.Name+": "+err.Error())
			return err
		case errors.Is(err, ErrTodo):
			fmt.Println("∘ 待实现")
			p.Manifest("stage.todo", st.Name)
			return err
		default:
			fmt.Println("✗ " + err.Error())
			p.Manifest("stage.error", st.Name+": "+err.Error())
			return err
		}
		if until != "" && st.Name == until {
			return nil
		}
	}
	fmt.Println("✅ 全部阶段完成")
	return nil
}

// ── 阶段实现 ──────────────────────────────────────────────────────

// stageManuscript 文稿：粘贴的文章正文即文稿（M0 输入），落成 manuscripts/manuscript.json。
func stageManuscript(p *Project) error {
	out := p.Artifact("manuscripts/manuscript.json")
	if exists(out) {
		return nil // 已有产物，跳过（重放语义）
	}
	raw, err := os.ReadFile(p.Artifact("input/article.txt"))
	if err != nil {
		return fmt.Errorf("缺少 input/article.txt: %w", err)
	}
	content := strings.TrimSpace(string(raw))
	if content == "" {
		return fmt.Errorf("文稿为空")
	}
	m := contract.NewManuscript(content, "paste")
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := writeFile("", out, string(b)); err != nil {
		return err
	}
	p.Manifest("manuscript.saved", fmt.Sprintf("%d 字", m.WordCount))
	return nil
}

// stageStoryboard 分镜（LLM 作业 #1）：api 模式直接调 DeepSeek；manual 模式走文件契约。
// api 失败（无 key/网络）自动降级为 ⏸ 等待人工产物，不阻塞管线。
func stageStoryboard(p *Project) error {
	final := p.Artifact("storyboards/storyboard.json")
	if exists(final) {
		var sb contract.Storyboard
		if err := readJSON(final, &sb); err != nil {
			return err
		}
		return contract.ValidateStoryboard(&sb)
	}
	accept := func(sb *contract.Storyboard) error {
		if err := contract.ValidateStoryboard(sb); err != nil {
			return fmt.Errorf("分镜未过自检: %w", err)
		}
		if err := writeFile("", final, mustJSON(sb)); err != nil {
			return err
		}
		p.Manifest("storyboard.saved", fmt.Sprintf("%d 段 / %.0fs", len(sb.Segments), sb.TotalHint()))
		return nil
	}

	out := p.LLMOutput("storyboard")
	if exists(out) { // 会话模式：人工/上轮已填的产物
		var sb contract.Storyboard
		if err := readJSON(out, &sb); err != nil {
			return err
		}
		return accept(&sb)
	}

	// API 模式
	prov, perr := llm.FromEnv(llm.RolePlan)
	if perr == nil {
		article, _ := os.ReadFile(p.Artifact("input/article.txt"))
		var sb contract.Storyboard
		usage, gerr := prov.GenerateJSON(context.Background(),
			"你是科普/解说视频的分镜规划器。只输出 JSON。",
			storyboardUserPrompt(string(article)), &sb)
		if gerr == nil {
			p.Manifest("llm.usage", fmt.Sprintf("storyboard prompt=%d completion=%d tokens",
				usage.PromptTokens, usage.CompletionTokens))
			if aerr := accept(&sb); aerr == nil {
				p.Manifest("storyboard.api", prov.Name+"/"+prov.Model)
				return nil
			} else {
				p.Manifest("storyboard.api.rejected", aerr.Error())
				// 校验拒绝：落一份请求文件供人工修正，不继续重试（bake-off 要记录失败）
				_ = writeFile("", p.LLMRequest("storyboard"),
					storyboardUserPrompt(string(article))+fmt.Sprintf("\n\n<!-- 上次模型输出被拒绝: %s\n    产物写入 %s 后重跑 -->\n", aerr, out))
				return fmt.Errorf("API 分镜被自检拒绝: %w", aerr)
			}
		}
		p.Manifest("storyboard.api.error", gerr.Error())
	}

	// 降级：会话模式（写请求文件，等待产物）
	req := p.LLMRequest("storyboard")
	article, _ := os.ReadFile(p.Artifact("input/article.txt"))
	if err := writeFile("", req, storyboardRequest(p, string(article), out)); err != nil {
		return err
	}
	return &ErrAwaitLLM{RequestPath: req, OutputPath: out}
}

// storyboardUserPrompt API 模式的 user prompt（与文件契约同一份要求，输出 JSON 严格按 schema）。
// input/storyboard_notes.txt 存在时追加用户修改要求（update_storyboard 工具写入）。
func storyboardUserPrompt(article string) string { return storyboardUserPromptNotes(article, "") }

func storyboardUserPromptNotes(article, notes string) string {
	noteBlock := ""
	if notes != "" {
		noteBlock = "\n\n用户对分镜的修改要求（优先满足）：\n" + notes
	}
	return fmt.Sprintf(`把下面的文稿改写成视频分镜表 JSON。

要求：
- timing_basis 固定 "narration"；plan 写一句话分段思路
- 每段 = {id:"seg01"起连续编号, idx, key:段落短标题, narration:旁白全文, visual_brief:画面提示一句话, duration_hint:秒数}
- 忠实原文，不编造事实；旁白只能来自文稿（口语化改写可以）
- 每段 8–16 秒；总时长 ≤ 900 秒（超限需压缩取舍）
- 只输出 JSON，无 markdown 围栏、无注释

文稿（%d 字）：
---
%s
---%s`, utf8.RuneCountInString(article), article, noteBlock)
}

// stageStyleSamples 风格样张（LLM 作业 #2）：必须 confirmed=true 才放行（硬门）。
// input/style_notes.txt 存在时按用户修改要求重生成（draft_style_samples 带 instruction 写入）。
func stageStyleSamples(p *Project) error {
	final := p.Artifact("style/style_samples.json")
	if exists(final) {
		var ss contract.StyleSamples
		if err := readJSON(final, &ss); err != nil {
			return err
		}
		return contract.ValidateStyleSamples(&ss)
	}
	out := p.LLMOutput("style_samples")
	if exists(out) {
		var ss contract.StyleSamples
		if err := readJSON(out, &ss); err != nil {
			return err
		}
		if err := contract.ValidateStyleSamples(&ss); err != nil {
			return err // 包括 confirmed=false：等待用户确认
		}
		if err := writeFile("", final, mustJSON(ss)); err != nil {
			return err
		}
		p.Manifest("style.confirmed", ss.Direction)
		return nil
	}

	// API 模式：直出样张（confirmed 置 false，等用户在舞台确认——硬门由 final 落定校验把守）。
	// NoThink：样张是纯 CSS 作业，开思考会烧上万 reasoning token、拖到分钟级。
	prov, perr := llm.FromEnv(llm.RolePlan)
	if perr == nil {
		var ss contract.StyleSamples
		usage, gerr := prov.GenerateJSONNoThink(context.Background(),
			"你是视频视觉设计师。只输出 JSON。",
			styleUserPrompt(p), &ss)
		if gerr == nil {
			if verr := contract.ValidateStyleSamplesDraft(&ss); verr != nil {
				p.Manifest("style_samples.api.rejected", verr.Error())
				return fmt.Errorf("API 样张被自检拒绝: %w", verr)
			}
			ss.Confirmed = false
			if err := writeFile("", final, mustJSON(ss)); err != nil {
				return err
			}
			p.Manifest("llm.usage", fmt.Sprintf("style_samples prompt=%d completion=%d tokens",
				usage.PromptTokens, usage.CompletionTokens))
			p.Manifest("style.drafted", ss.Direction)
			p.Manifest("style_samples.api", prov.Name+"/"+prov.Model)
			return nil
		}
		p.Manifest("style_samples.api.error", gerr.Error())
	}

	req := p.LLMRequest("style_samples")
	if err := writeFile("", req, styleRequest(p, out)); err != nil {
		return err
	}
	return &ErrAwaitLLM{RequestPath: req, OutputPath: out, Note: "样张 JSON 中 confirmed 置 true 代表用户已确认（硬门）"}
}

// stageTTS 配音：每段音频 + 逐词时间戳。
// 生成命令（引擎 Kokoro 离线，验证通过的调用形态）：
//
//	npx hyperframes@0.8.55 tts --text-file audio/segNN.txt -v zf_xiaobei -l zh -o audio/segNN.wav --json
//	faster-whisper medium + initial_prompt=旁白原文 → 字级时间戳 → audio_meta.json
//
// ⚠️ --text-file 文件必须存在，否则路径字符串会被当文本朗读（已踩坑）。
// 本阶段校验产物齐全；批量生成器在 ai/ 侧（ai/tts_align.py，下一里程碑并入）。
func stageTTS(p *Project) error {
	sb, err := p.LoadStoryboard()
	if err != nil {
		return fmt.Errorf("先完成 storyboard: %w", err)
	}
	missing := 0
	for _, seg := range sb.Segments {
		if !exists(p.Artifact("audio/" + seg.ID + ".wav")) {
			missing++
		}
	}
	if missing > 0 {
		return fmt.Errorf("缺 %d/%d 段音频：运行 ai/tts_align.py 或 hyperframes tts（见上注释）: %w",
			missing, len(sb.Segments), ErrTodo)
	}
	if !exists(p.Artifact("audio_meta.json")) {
		return fmt.Errorf("缺 audio_meta.json（字级时间戳）: %w", ErrTodo)
	}
	p.Manifest("tts.ready", fmt.Sprintf("%d 段", len(sb.Segments)))
	return nil
}

// stageCompositions 合成物（LLM 作业 #3）：每段一个 HTML（含元素命名规范），可并行填写。
func stageCompositions(p *Project) error {
	sb, err := p.LoadStoryboard()
	if err != nil {
		return fmt.Errorf("先完成 storyboard: %w", err)
	}
	// 样式方向注入 packet
	direction := ""
	var ss contract.StyleSamples
	if readJSON(p.Artifact("style/style_samples.json"), &ss) == nil {
		direction = ss.Direction
	}
	awaiting := ""
	for _, seg := range sb.Segments {
		comp := p.Artifact("compositions/frames/" + seg.ID + ".html")
		if exists(comp) {
			continue
		}
		name := "comp-" + seg.ID
		out := p.LLMOutput(name)
		if exists(out) {
			b, rerr := os.ReadFile(out)
			if rerr != nil {
				return rerr
			}
			// 产物格式：{"html": "...", "elements": [...]}
			var payload struct {
				HTML     string                  `json:"html"`
				Elements []contract.NamedElement `json:"elements"`
			}
			if err := json.Unmarshal(b, &payload); err != nil {
				return fmt.Errorf("%s 解析失败: %w", out, err)
			}
			if payload.HTML == "" || len(payload.Elements) == 0 {
				return fmt.Errorf("%s 缺少 html 或 elements（元素命名规范是硬约束）", out)
			}
			if err := writeFile("", comp, payload.HTML); err != nil {
				return err
			}
			reg, _ := json.MarshalIndent(contract.ElementRegistry{Elements: payload.Elements}, "", "  ")
			_ = writeFile("", p.Artifact("compositions/registry/"+seg.ID+".json"), string(reg))
			p.Manifest("composition.saved", seg.ID)
			continue
		}
		req := p.LLMRequest(name)
		if err := writeFile("", req, compRequest(p, seg, direction, out)); err != nil {
			return err
		}
		awaiting = seg.ID
	}
	if awaiting != "" {
		return &ErrAwaitLLM{
			RequestPath: p.LLMRequest("comp-" + awaiting),
			OutputPath:  p.LLMOutput("comp-" + awaiting),
			Note:        "每段一个产物文件，可任意顺序/并行填写",
		}
	}
	return nil
}

// stageCheck 质量门禁：调 hyperframes check（lint+runtime+layout+motion+contrast）。
func stageCheck(p *Project) error {
	out, err := runCLI(p, "npx", "--yes", "hyperframes@0.8.55", "check")
	if err != nil {
		return fmt.Errorf("check 未过（诊断如下，修复后重跑）:\n%s", tailLines(out, 30))
	}
	_ = writeFile("", p.Artifact(".hyperframes-ok"), "check passed "+time.Now().Format(time.RFC3339)+"\n")
	p.Manifest("check.passed", "")
	return nil
}

// stageRender 按段渲染（增量）+ 无损拼接。
// 每段一个单段工程 .hf-seg/segNN.html（assemble 生成，含该段画面与配音），
// 渲出 renders/segs/segNN.mp4；段片缺失或旧于 段帧/音频/段工程 任一来源则重渲，
// 其余段跳过——rework 只重渲改动段，耗时与段长成正比而非片长。
// main.mp4 = 人声母版 concat -c copy 后按项目配乐混音（见 stageStitch）。
func stageRender(p *Project) error {
	sb, err := p.LoadStoryboard()
	if err != nil {
		return err
	}
	segsDir := p.Artifact("renders/segs")
	if err := os.MkdirAll(segsDir, 0o755); err != nil {
		return err
	}
	live := map[string]bool{}
	for _, seg := range sb.Segments {
		live[seg.ID] = true
		out := filepath.Join(segsDir, seg.ID+".mp4")
		if !segStale(p, seg.ID, out) {
			p.Manifest("render.seg.cached", seg.ID)
			continue
		}
		fmt.Printf("  · %s 渲染中\n", seg.ID)
		outLog, err := runCLI(p, "npx", "hyperframes@0.8.55", "render", ".",
			"-c", ".hf-seg/"+seg.ID+".html", "-o", "renders/segs/"+seg.ID+".mp4")
		if err != nil {
			return fmt.Errorf("%s 渲染失败:\n%s", seg.ID, tailLines(outLog, 30))
		}
		p.Manifest("render.seg", seg.ID)
	}
	entries, _ := os.ReadDir(segsDir)
	for _, e := range entries { // 清理清单外残留段片（段增删后旧文件不进 concat，也不留垃圾）
		if strings.HasSuffix(e.Name(), ".mp4") && !live[strings.TrimSuffix(e.Name(), ".mp4")] {
			_ = os.Remove(filepath.Join(segsDir, e.Name()))
		}
	}
	if err := concatSegs(p, sb, segsDir); err != nil {
		return err
	}
	p.Manifest("render.done", "renders/main_voice.mp4")
	return nil
}

// segStale 段片是否需要重渲：缺失，或旧于该段的 帧html/音频/单段工程 任一来源。
func segStale(p *Project, segID, out string) bool {
	fi, err := os.Stat(out)
	if err != nil {
		return true
	}
	for _, src := range []string{
		p.Artifact("compositions/frames/" + segID + ".html"),
		p.Artifact("audio/" + segID + ".wav"),
		p.Artifact(".hf-seg/" + segID + ".html"),
	} {
		if sfi, err := os.Stat(src); err == nil && sfi.ModTime().After(fi.ModTime()) {
			return true
		}
	}
	return false
}

// concatSegs 全部段片 concat -c copy 拼成人声母版 main_voice.mp4（同源输出参数
// 一致，免重编码）。BGM 在 stitch 阶段从母版混入 main.mp4——换配乐不用重渲画面。
func concatSegs(p *Project, sb *contract.Storyboard, segsDir string) error {
	ff, err := ffmpegPath()
	if err != nil {
		return err
	}
	list := filepath.Join(segsDir, "list.txt")
	var b strings.Builder
	for _, seg := range sb.Segments {
		b.WriteString("file '" + seg.ID + ".mp4'\n")
	}
	if err := os.WriteFile(list, []byte(b.String()), 0o644); err != nil {
		return err
	}
	ctx := p.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, ff, "-y", "-v", "error",
		"-f", "concat", "-safe", "0", "-i", "list.txt", "-c", "copy", "../main_voice.mp4")
	cmd.Dir = segsDir
	cmd.Env = cmdEnv()
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("段片拼接失败: %w\n%s", err, tailLines(buf.String(), 20))
	}
	return nil
}

// ffmpegPath PATH 优先，回退 extraPATH（与 runCLI 的 Env 注入同一来源）。
func ffmpegPath() (string, error) {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	if extraPATH != "" {
		for _, cand := range []string{"ffmpeg.exe", "ffmpeg"} {
			p := filepath.Join(extraPATH, cand)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p, nil
			}
		}
	}
	return "", errors.New("ffmpeg 不在 PATH（ensureFFmpeg 未生效？）")
}

// FFmpegPath 供外部包（produce 抽帧审查）定位 ffmpeg。
func FFmpegPath() (string, error) { return ffmpegPath() }

// cmdEnv runCLI 与 ffmpeg 共用的进程环境（extraPATH 注入）。
func cmdEnv() []string {
	if extraPATH == "" {
		return nil
	}
	return append(os.Environ(), "PATH="+extraPATH+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// stageStitch 成片：人声母版 main_voice.mp4 → 按项目配乐（project.json bgm 字段）
// 混入 BGM（sidechain 闪避：人声压音乐）写 renders/main.mp4；无配乐则直接转正。
// 母版始终保留，换 BGM 只需重跑本阶段（StitchNow），不动画面渲染。
func stageStitch(p *Project) error {
	voice := p.Artifact("renders/main_voice.mp4")
	if !exists(voice) {
		return fmt.Errorf("无 renders/main_voice.mp4（render 阶段未完成？）")
	}
	out := p.Artifact("renders/main.mp4")
	name, track := bgmFor(p)
	if track == "" {
		if err := copyFile(voice, out); err != nil {
			return err
		}
		p.Manifest("film.ready", "renders/main.mp4")
		return nil
	}
	if err := mixBGM(p, voice, track, out); err != nil {
		return fmt.Errorf("BGM 混音失败: %w", err)
	}
	p.Manifest("film.ready", "renders/main.mp4+bgm="+name)
	return nil
}

// StitchNow 只跑拼接/混音（成片就绪后换 BGM 用，画面不动）。
func StitchNow(p *Project) error { return stageStitch(p) }

// ── BGM 混音 ──────────────────────────────────────────────────────

// bgmTrack 曲库清单（ai/assets/music/tracks.json）。
type bgmTrack struct {
	Name     string  `json:"name"`
	File     string  `json:"file"`
	Duration float64 `json:"duration"`
	Mood     string  `json:"mood"`
}

func musicDir() string { return filepath.Join(rootDir, "ai", "assets", "music") }

func loadTracks() []bgmTrack {
	if rootDir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(musicDir(), "tracks.json"))
	if err != nil {
		return nil
	}
	var ts []bgmTrack
	if json.Unmarshal(b, &ts) != nil {
		return nil
	}
	return ts
}

// bgmFor 项目配乐解析：project.json "bgm" 字段 → 曲库文件路径（空 = 无配乐）。
func bgmFor(p *Project) (name, file string) {
	b, err := os.ReadFile(p.Artifact("project.json"))
	if err != nil {
		return "", ""
	}
	var meta struct {
		BGM string `json:"bgm"`
	}
	if json.Unmarshal(b, &meta) != nil {
		return "", ""
	}
	want := strings.TrimSpace(meta.BGM)
	if want == "" || strings.EqualFold(want, "off") {
		return "", ""
	}
	for _, tr := range loadTracks() {
		if strings.EqualFold(tr.Name, want) {
			f := filepath.Join(musicDir(), tr.File)
			if exists(f) {
				return tr.Name, f
			}
			return "", ""
		}
	}
	return "", ""
}

// mixBGM 人声母版 + 循环 BGM（-stream_loop 无限铺底）→ sidechain 闪避
//（音乐听人声的话自动压低）→ amix（不归一化，人声原样）。视频流直拷。
// 母版永不改动，产物写临时文件后原子转正——重复执行无二次混音风险。
func mixBGM(p *Project, voice, track, out string) error {
	ff, err := ffmpegPath()
	if err != nil {
		return err
	}
	fade := ""
	if dur := probeDuration(voice); dur > 3.5 {
		fade = fmt.Sprintf(",afade=t=out:st=%.2f:d=1.5", dur-1.7)
	}
	fc := fmt.Sprintf("[1:a]volume=0.34%s[m];"+
		"[m][0:a]sidechaincompress=threshold=0.035:ratio=7:attack=120:release=1100[duck];"+
		"[0:a][duck]amix=inputs=2:duration=first:dropout_transition=0:normalize=0[aout]", fade)
	tmp := out + ".bgm.mp4.tmp"
	cmd := exec.Command(ff, "-y", "-v", "error",
		"-i", voice,
		"-stream_loop", "-1", "-i", track,
		"-filter_complex", fc,
		"-map", "0:v", "-map", "[aout]",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "192k", tmp)
	cmd.Dir = p.Dir
	cmd.Env = cmdEnv()
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ffmpeg 混音退出: %w\n%s", err, tailLines(buf.String(), 20))
	}
	_ = os.Remove(out)
	return os.Rename(tmp, out)
}

// probeDuration ffprobe 取媒体时长（秒；失败返回 0，调用方跳过淡出）。
func probeDuration(path string) float64 {
	if p, err := exec.LookPath("ffprobe"); err == nil {
		out, err := exec.Command(p, "-v", "error", "-show_entries", "format=duration",
			"-of", "csv=p=0", path).Output()
		if err == nil {
			if f, perr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); perr == nil {
				return f
			}
		}
	}
	if rootDir != "" && extraPATH != "" { // 回退 winget bin 目录
		for _, cand := range []string{"ffprobe.exe", "ffprobe"} {
			out, err := exec.Command(filepath.Join(extraPATH, cand), "-v", "error",
				"-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
			if err == nil {
				if f, perr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64); perr == nil {
					return f
				}
			}
		}
	}
	return 0
}

// copyFile 成片转正（母版保留副本，换 BGM 时可重新混音）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".copy.tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}

// ListBGM 曲库可读清单（agent 工具用）："calm——温暖平静…"。
func ListBGM() string {
	ts := loadTracks()
	if len(ts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(ts))
	for _, tr := range ts {
		lines = append(lines, fmt.Sprintf("%s（%s）", tr.Name, tr.Mood))
	}
	return strings.Join(lines, "；")
}

// SetBGM 写 project.json 的 bgm 字段（track="off"/"" 清除；曲名做曲库校验）。
func SetBGM(p *Project, track string) error {
	want := strings.TrimSpace(track)
	if want != "" && !strings.EqualFold(want, "off") {
		ok := false
		for _, tr := range loadTracks() {
			if strings.EqualFold(tr.Name, want) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("曲名 %q 不在曲库（可用：%s）", want, ListBGM())
		}
	}
	b, err := os.ReadFile(p.Artifact("project.json"))
	if err != nil {
		return err
	}
	var meta map[string]any
	if err := json.Unmarshal(b, &meta); err != nil {
		return fmt.Errorf("project.json 解析失败: %w", err)
	}
	if want == "" || strings.EqualFold(want, "off") {
		delete(meta, "bgm")
	} else {
		meta["bgm"] = strings.ToLower(want)
	}
	nb, _ := json.MarshalIndent(meta, "", "  ")
	return os.WriteFile(p.Artifact("project.json"), nb, 0o644)
}

// extraPATH 额外 PATH（如 winget 安装的 FFmpeg bin），由宿主进程注入。
var extraPATH string

// rootDir 仓库根（SetRootDir 注入）：BGM 曲库定位 ai/assets/music/。
var rootDir string

// SetRootDir 注入仓库根（server/m0 启动时调用；不注入 = BGM 功能关闭）。
func SetRootDir(dir string) { rootDir = dir }

// SetExtraPATH 注入额外可执行搜索路径（check/render 需要 ffmpeg）。
func SetExtraPATH(dir string) { extraPATH = dir }

// EnsureFFmpeg hyperframes check/render 与段片拼接需要 ffmpeg。
// PATH 里没有就找 winget 安装目录并入 PATH（开发机场景；容器内 PATH 自带）。
func EnsureFFmpeg() {
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		return
	}
	const winget = `C:\Users\86151\AppData\Local\Microsoft\WinGet\Packages`
	matches, _ := filepath.Glob(filepath.Join(winget, "Gyan.FFmpeg*", "ffmpeg-*", "bin"))
	if len(matches) > 0 {
		os.Setenv("PATH", matches[0]+string(os.PathListSeparator)+os.Getenv("PATH"))
		SetExtraPATH(matches[0])
		fmt.Printf("[ffmpeg] 注入 PATH: %s\n", matches[0])
		return
	}
	fmt.Println("[ffmpeg] 警告：PATH 中找不到 ffmpeg，check/render 将失败（winget install Gyan.FFmpeg）")
}

// runCLI 在项目目录执行外部命令（hyperframes CLI 等），返回合并输出。
// npx 一律 --offline：hyperframes 锁版本已在本地缓存，渲染管线不依赖 registry 网络。
func runCLI(p *Project, name string, args ...string) (string, error) {
	full := append([]string{"--offline", "--yes"}, args...)
	ctx := p.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, full...)
	cmd.Dir = p.Dir
	cmd.Env = cmdEnv()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err != nil && ctx.Err() != nil { // 被取消杀掉：报可识别的 ctx 错误而非 killed
		return buf.String(), ctx.Err()
	}
	return buf.String(), err
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func mustJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// ── LLM 请求模板（prompt 库 v0 · 会话模式即产物契约）─────────────

func storyboardRequest(p *Project, article, out string) string {
	return fmt.Sprintf(`# 作业：分镜规划（帧述 M0 · LLM 作业 #1）

## 角色与任务
你是科普/解说视频的分镜规划器。把下面的文稿改写成一份分镜表：
- 忠实原文（时长服从内容，不强制压缩；总时长上限 900 秒，超限需给出压缩取舍建议）
- 每段 = 旁白（从文稿改写，口语化可以，不得编造事实）+ 画面提示（一句话，人话）+ 时长估计
- 每段控制在 8–16 秒；段落数量随内容自然决定
- timing_basis 固定 "narration"（旁白驱动：先配音，画面按实际音频时长生成）

## 文稿（input/article.txt，共 %d 字）
---
%s
---

## 输出要求
把 JSON 写到: %s
严格按此结构（UTF-8，无注释，无多余字段）:

{
  "timing_basis": "narration",
  "style_id": "",
  "plan": "一句话说明分段思路与时长分配",
  "segments": [
    {
      "id": "seg01",
      "idx": 1,
      "key": "段落短标题",
      "narration": "该段旁白全文",
      "visual_brief": "画面提示（一句话）",
      "duration_hint": 10
    }
  ]
}

## 硬约束（schema 校验会拒收）
- segments 非空；每段 narration、duration_hint > 0；id 从 seg01 连续编号
- 总时长 ≤ 900；旁白只能来自文稿内容
`, utf8.RuneCountInString(article), article, out)
}

// styleBrief 样张作业正文（API 直调与会话请求共用一份，防两处漂移）。
// input/style_notes.txt 存在时追加用户修改要求（draft_style_samples 带 instruction 写入）。
// 画布硬定 960×540：舞台 SampleFrame 缩放与 ai/snap_samples.py 截图都按此假设，违反必被裁切。
func styleBrief(p *Project) string {
	notes := ""
	if b, err := os.ReadFile(p.Artifact("input/style_notes.txt")); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		notes = "\n\n## 用户修改要求（优先满足；未提及的方面保持连贯，不要推倒重来）\n" + string(b) + "\n"
	}
	return fmt.Sprintf(`你是视频视觉设计师。按文稿气质提议一个风格方向，产出 1–3 张 HTML 样张：
- 样张 = 用拟采用风格参数（色板/字体/组件/动效偏好）渲出的静态小画面
- 样张与成片出自同一套 token，所见即所得；不要用图片/网络资源，纯 CSS
- 中文用系统楷体 KaiTi（演示环境），正式渲染字体后续本地化

## 画布硬约束（违反必被裁切）
- 每张样张的 html 是完整自包含文档，画布固定 960×540：html,body{width:960px;height:540px;overflow:hidden}
- 内容全部落在画布内，不依赖滚动；禁止 vw/vh；大字标题留边距，不得溢出画布
- CSS 紧凑：内联一个 <style> 块，每张 html 控制在 2000 字符内%s`, notes)
}

func styleJSONShape() string {
	return `{
  "direction": "风格方向名（如：手绘叙事（纸面·马克笔））",
  "samples": [
    { "tag": "样张 A · 标题帧", "desc": "说明这张展示什么", "html": "<!doctype html>…整文档…" },
    { "tag": "样张 B · 数据帧", "desc": "…", "html": "…" }
  ],
  "confirmed": false
}`
}

func styleUserPrompt(p *Project) string {
	return styleBrief(p) + "\n\n## 输出要求\n只输出 JSON，无 markdown 围栏：\n" + styleJSONShape()
}

func styleRequest(p *Project, out string) string {
	return fmt.Sprintf(`# 作业：风格样张（帧述 M0 · LLM 作业 #2）

%s

## 输出要求
把 JSON 写到: %s（confirmed 先置 false，用户确认后由管线改为 true——这是硬门）

%s

## 校验
- direction 非空；samples 1–3 张；每张 html 非空
`, styleBrief(p), out, styleJSONShape())
}

func compRequest(p *Project, seg contract.Segment, direction, out string) string {
	return fmt.Sprintf(`# 作业：段合成物（帧述 M0 · LLM 作业 #3 · %s）

## 段信息（来自分镜表）
- 段号: %d（id=%s）
- 标题: %s
- 旁白（时长基准，约 %.0f 秒）: %s
- 画面提示: %s
- 全片风格: %s

## 任务
写这一个段的 HyperFrames 子合成物 HTML（一段 = 一个 sub-composition），
同时给出元素注册表（每个可见元素的稳定 id + 人话名）。

## HyperFrames 硬规则（违反会被 lint 拒收）
1. 整个文件就是一个 <template>…</template> 片段，无 DOCTYPE/html/head/body
2. 根元素 data-composition-id="%s"；样式作用于根用 #root，不要给根加 class
3. 恰好一个 gsap.timeline({{paused:true}}) 注册到 window.__timelines["%s"]
4. 每个可见元素给 data-start / data-duration / data-track-index 与 class="clip"
5. 确定性：无网络、无 Date.now、无 Math.random、无 CSS transition/repeat/yoyo
6. 字体只用本地资产或系统楷体；不引任何外链
7. 揭示随旁白时间点分布（不前载）；字幕带占底部 ~17%%，主内容留在上 83%%
8. 元素命名规范（平台门禁）：每个有意义的可见元素带
   data-hf-name="人话名"（如 data-hf-name="箭头：蓝光弹开"）

## 输出要求
把 JSON 写到: %s

{
  "html": "<template>…整段合成物…</template>",
  "elements": [
    { "id": "%s-main-title", "name": "标题：xxx" },
    { "id": "%s-arrow-1",   "name": "箭头：xxx" }
  ]
}
`, seg.Key, seg.Idx, seg.ID, seg.Key, seg.DurationHint, seg.Narration, seg.VisualBrief, directionLabel(direction), seg.ID, seg.ID, out, seg.ID, seg.ID)
}

func directionLabel(d string) string {
	if d == "" {
		return "（风格样张未出，先按分镜气质自拟，风格确认后可重生成）"
	}
	return d
}
