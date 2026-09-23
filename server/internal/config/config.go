package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config 汇总运行时配置。M0 只依赖 DataDir 与 LLMMode；
// PG/MinIO/Redis 的值先就位（原服务器 47.94.247.12），M1 接入。
type Config struct {
	DataDir string
	LLMMode string // manual（会话模式·文件契约，M0 默认）| api（DeepSeek 等，管线稳定后切换）

	DeepSeekKey string

	PGDSN          string
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	RedisAddr      string
}

func Load() (*Config, error) {
	// .env 可选存在：当前目录逐级向上找（在 server/ 里跑也能读到仓库根的 .env）
	for _, dir := range []string{".", "..", "../.."} {
		p := filepath.Join(dir, ".env")
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			break
		}
	}

	get := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}

	c := &Config{
		DataDir:        get("DATA_DIR", "data"),
		LLMMode:        get("LLM_MODE", "manual"),
		DeepSeekKey:    os.Getenv("DEEPSEEK_API_KEY"),
		PGDSN:          os.Getenv("PG_DSN"),
		MinIOEndpoint:  get("MINIO_ENDPOINT", "47.94.247.12:9000"),
		MinIOAccessKey: os.Getenv("MINIO_ACCESS_KEY"),
		MinIOSecretKey: os.Getenv("MINIO_SECRET_KEY"),
		MinIOBucket:    get("MINIO_BUCKET", "zhenshu"),
		RedisAddr:      get("REDIS_ADDR", "47.94.247.12:6379"),
	}

	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
	c.DataDir = abs
	return c, nil
}
