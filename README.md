# Emomo

> AI 表情包语义搜索系统 — Go 后端 + React 前端 monorepo

Emomo 让你用自然语言搜表情包。系统由 Go 后端（搜索 + 本地静态图片目录摄入）和 React 前端（用户界面）组成。当前默认检索链路使用 Qwen3-VL 多模态 embedding：导入时直接为图片生成 image 向量，搜索时将用户文本嵌入到同一语义空间并与图片向量匹配；VLM 描述和 OCR 作为 caption/keyword 辅助信号与展示元数据，不再是唯一或主检索路径。

资源约束：表情包资源只支持静态图片；GIF 文件不再支持，也不会被摄入。

当前关系库只保留三张核心表：`memes`、`meme_annotations`、`meme_vectors`。protobuf message schema 定义在 [backend/proto/emomo/v1/](backend/proto/emomo/v1/)，拆为 `types.proto` / `meme.proto` / `api.proto`。在本项目里，protobuf 的边界是 API DTO、前后端生成类型、跨边界封闭枚举，以及少量结构化 DB JSON 值（当前仅 `memes.image_info`、`meme_annotations.labels`）；关系表结构、迁移、运行时配置和 UI 状态不归 protobuf 管。生成代码集中在 [backend/gen/](backend/gen/)（Go）与 [frontend/gen/](frontend/gen/)（TS），均以 `linguist-generated=true` 标记。数据库结构详见 [docs/DATABASE_SCHEMA.md](docs/DATABASE_SCHEMA.md)。

Supabase/PostgreSQL 部署中这三张核心表不启用 Row Level Security；前端不直接访问 Supabase 表，而是通过 Go API 访问数据，访问控制在服务端数据库连接层完成。

## 仓库结构

```
emomo/
├── backend/      # Go + Gin + Qdrant + GORM，REST API + 摄入流水线
├── frontend/     # React 19 + Vite + Framer Motion，单页应用
├── deployments/  # 跨服务的 Docker Compose 编排（API + Grafana Alloy）
├── docs/         # 跨服务设计与使用文档
├── scripts/
│   └── start.sh  # 本机一键起后端 + 前端
├── render.yaml   # Render 部署配置（rootDir: backend）
└── railway.json  # Railway 部署配置（dockerfilePath: backend/Dockerfile）
```

每个子项目都有自己的 `README.md` / `AGENTS.md` / `CLAUDE.md` / `GEMINI.md`，说明该子项目的本地开发与约定：

- 后端：[backend/README.md](backend/README.md)
- 前端：[frontend/README.md](frontend/README.md)

## 快速上手

### 一键起前后端

```bash
./scripts/start.sh
```

脚本会先启 backend（`go run ./cmd/api`，端口 8080），再启 frontend（`npm run dev`，端口 5173）。需要先在 [backend/.env](backend/.env.example) 填好 API keys（Qdrant、对象存储、VLM、embedding 等）。

### 单独运行某一块

```bash
# 后端
cd backend
cp .env.example .env   # 首次：填好 API keys
go run ./cmd/api

# 前端
cd frontend
cp .env.example .env   # 首次：默认指向 http://localhost:8080/api/v1
npm install
npm run dev
```

### Qwen3-VL 多模态向量摄入

默认配置会使用 `qwen3vl` profile 同时写入 image 与 caption 两路向量，其中 image 路直接对图片生成向量：

```bash
cd backend
./scripts/import-data.sh -p ./data/memes -l 50
# 或显式指定 profile:
./scripts/import-data.sh -p ./data/memes --profile qwen3vl -l 50
```

详见 [docs/MULTI_EMBEDDING.md](docs/MULTI_EMBEDDING.md) 与 [backend/configs/config.yaml](backend/configs/config.yaml)。

### 更新 protobuf 消息 schema

修改 `backend/proto/emomo/v1/` 下任意 `.proto` 后，需要同时重新生成后端 Go 与前端 TS：

```bash
# Go → backend/gen/
cd backend && GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate

# TS → frontend/gen/
cd frontend && npm run gen
```

## 技术栈速览

| 子项目 | 关键技术 |
|--------|---------|
| backend | Go 1.26.2, Gin, GORM, Qdrant (gRPC), S3/R2, Qwen3-VL 多模态 embeddings, OpenAI-compatible VLM/OCR 辅助分析, BM25 hybrid 检索, Grafana Alloy + Loki |
| frontend | React 19, TypeScript, Vite 7, Framer Motion, Playwright e2e |

## 部署

- **Docker Compose（本机）**：`docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d`，会起 API 容器 + Grafana Alloy 日志采集（Qdrant 与对象存储需自备）。
- **Render**：根的 [render.yaml](render.yaml) 把后端服务的 rootDir 设为 `backend/`。
- **Railway**：根的 [railway.json](railway.json) 指向 `backend/Dockerfile`。
- **Hugging Face Space**：[`.github/workflows/sync_to_hf.yml`](.github/workflows/sync_to_hf.yml) 在每次 push 到 main 时把 `backend/` 子树拆出来 force-push 到 Space 的 `main` 分支，所以 Space 看到的根就是 `backend/`。

## 贡献约定

- 提交信息使用 Conventional Commits（`feat:`、`fix:`、`chore:` 等）；跨子项目的改动在正文里按目录分点说明。
- AI agents 协作约定见各子项目的 [AGENTS.md](backend/AGENTS.md) / [frontend/AGENTS.md](frontend/AGENTS.md) 与本仓库根的 [AGENTS.md](AGENTS.md)。
- 不提交 secrets，使用各子项目下的 `.env` 与 `.env.example`。

## 更多文档

- [docs/QUICK_START.md](docs/QUICK_START.md)
- [docs/INGEST.md](docs/INGEST.md)
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)
- [docs/MULTI_EMBEDDING.md](docs/MULTI_EMBEDDING.md)
- [docs/DATABASE_SCHEMA.md](docs/DATABASE_SCHEMA.md)

## License

[MIT](LICENSE)
