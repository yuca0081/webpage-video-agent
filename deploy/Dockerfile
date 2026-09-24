# 帧述 app+renderer 一体镜像（单容器形态：app 进程内直接 exec node/python/ffmpeg，
# tech-stack §5 的两容器是愿景形态，代码现实是进程内编排，按现实部署）。
# 构建位置：服务器（开发机无 docker；构建产物已由 deploy.sh 在开发机交叉编译好）。
FROM node:22-bookworm-slim

# ffmpeg：渲染合成；chromium：hyperframes 无头渲染；Noto CJK：中文字形（缺了全豆腐块）；
# postgresql-client：部署时跑 migrations（deploy.sh 经 compose run 调用）
RUN apt-get update && apt-get install -y --no-install-recommends \
      ffmpeg python3 python3-pip chromium fonts-noto-cjk ca-certificates tini \
      postgresql-client \
    && rm -rf /var/lib/apt/lists/*
ENV PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium

# 锁版渲染/合成 CLI 预装入镜像（runCLI 用 npx --offline，必须本地已缓存）
RUN npm i -g hyperframes@0.8.55

# 字级对齐降级引擎（funasr+torch 约 2GB 不装；whisper medium 模型首次运行经 hf-mirror 下载）
RUN pip3 install --break-system-packages faster-whisper
ENV HF_ENDPOINT=https://hf-mirror.com

WORKDIR /app
# 开发机交叉编译/构建产物（deploy.sh 生成）
COPY app-linux /app/app
COPY web/dist /app/web/dist
COPY ai /app/ai
COPY data/projects/_shared /app/data/projects/_shared
COPY server/migrations /app/server/migrations

# root = DATA_DIR 上一级（main.go 推断逻辑要求 root 下有 ai/），CWD 必须是 /app
ENV DATA_DIR=/app/data ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/app/app"]
