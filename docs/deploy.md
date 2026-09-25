# 部署（47.94.247.12）

一键部署（开发机 Git Bash）：

```bash
bash deploy/deploy.sh --env   # 首次（上传 .env）
bash deploy/deploy.sh         # 日常发版
```

前置（一次性）：

1. **SSH 免密**：开发机 `ssh-copy-id root@47.94.247.12`（服务器只收 publickey）。
2. **服务器 .env**（`--env` 上传即自动改写）：`PG_DSN`/`MINIO_ENDPOINT` 等本服务器地址
   自动替换为 `127.0.0.1`（容器 host 网络，本机回环直连宿主机 PG/MinIO，不经安全组）；
   需有 `LLM_MODE=api`、`LLM_API_KEY`、`SILICONFLOW_API_KEY`（TTS 走 API 免本地模型）。
3. **服务器安全组**：放行 8080（试用访问入口）。

形态与取舍：

- **单容器**（`deploy/compose.yml`）：app 进程内直接 exec node/python/ffmpeg（代码现实如此，
  tech-stack §5 两容器是愿景形态）。host 网络 + `restart: unless-stopped`。
- **镜像内预装**：node22 + hyperframes@0.8.55（npx --offline 走本地）、ffmpeg、chromium
  （hyperframes 无头渲染）、fonts-noto-cjk（中文必需）、faster-whisper（字级对齐）。
- **对齐引擎取舍**：服务器不装 funasr+torch（约 2GB），对齐走 faster-whisper 降级路径
  （词级→字级均分）。模型首次运行时经 hf-mirror 下载（medium，约 1.5GB）。
- **数据**：`/opt/zhenshu/data/projects` 宿主卷，项目产物跨发版保留；migrations 幂等，
  每次部署重跑（psql 容器内执行）。
- **发版物**：开发机交叉编译 linux 静态二进制 + web/dist + ai/（引擎+素材+注册表同在此包）→ tar 流传输。
