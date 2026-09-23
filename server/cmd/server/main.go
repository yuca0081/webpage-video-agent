// server — 帧述 M1 HTTP 服务（API + SSE + Agent + 制作编排）。
//
//	go run ./cmd/server          （默认 :8080；前端 web/ 另起 Vite dev 或用 dist 托管）
package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"webpage-video-agent/server/internal/agent"
	"webpage-video-agent/server/internal/api"
	"webpage-video-agent/server/internal/config"
	"webpage-video-agent/server/internal/events"
	"webpage-video-agent/server/internal/methodlib"
	"webpage-video-agent/server/internal/pipeline"
	"webpage-video-agent/server/internal/produce"
	"webpage-video-agent/server/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	root, _ := filepath.Abs(filepath.Join(cfg.DataDir, ".."))
	if _, err := os.Stat(filepath.Join(root, "ai")); err != nil {
		log.Fatalf("仓库根推断失败（%s 下无 ai/），检查 DATA_DIR", root)
	}

	ensureFFmpeg(root)

	st := store.Open(cfg.PGDSN)
	hub := events.NewHub()
	lib := methodlib.New(cfg.DataDir)
	producer := &produce.Runner{
		DataDir: cfg.DataDir, RootDir: root, Hub: hub,
		OnDone: func(id, status string) {
			hub.Emit(id, "stage", "pipeline", status)
		},
	}
	ag := &agent.Agent{
		DataDir: cfg.DataDir, RootDir: root, Store: st, Lib: lib, Producer: producer,
		Emit:    hub.Emit,
		OnEvent: func(id, event, detail string) { hub.Emit(id, event, "", detail) },
	}
	srv := &api.Server{
		DataDir: cfg.DataDir, RootDir: root, Store: st, Hub: hub, Agent: ag, Producer: producer, Lib: lib,
	}

	gin.SetMode(gin.ReleaseMode)
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("帧述 M1 服务启动 %s（store=%s llm_mode=%s data=%s）", addr, st.Kind(), cfg.LLMMode, cfg.DataDir)
	log.Fatal(srv.Router().Run(addr))
}

// ensureFFmpeg hyperframes check/render 需要 ffmpeg。PATH 里没有就找 winget 安装目录并入 PATH。
func ensureFFmpeg(root string) {
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		return
	}
	const winget = `C:\Users\86151\AppData\Local\Microsoft\WinGet\Packages`
	matches, _ := filepath.Glob(filepath.Join(winget, "Gyan.FFmpeg*", "ffmpeg-*", "bin"))
	if len(matches) > 0 {
		os.Setenv("PATH", matches[0]+string(os.PathListSeparator)+os.Getenv("PATH"))
		pipeline.SetExtraPATH(matches[0])
		log.Printf("[ffmpeg] 注入 PATH: %s", matches[0])
		return
	}
	log.Println("[ffmpeg] 警告：PATH 中找不到 ffmpeg，check/render 将失败（winget install Gyan.FFmpeg）")
}
