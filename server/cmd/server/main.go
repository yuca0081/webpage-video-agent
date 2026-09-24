// server — 帧述 M1 HTTP 服务（API + SSE + Agent + 制作编排）。
//
//	cd server && go run ./cmd/server   （默认 :8080；DATA_DIR 相对 .env 解析，启动目录无关）
package main

import (
	"log"
	"os"
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

	pipeline.EnsureFFmpeg()

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
