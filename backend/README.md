---
title: Emomo
emoji: 🔥
colorFrom: green
colorTo: indigo
sdk: docker
pinned: true
license: mit
---

---

# Emomo Backend - AI 表情包语义搜索

> 本目录是 emomo monorepo 的后端子项目。仓库总览见 [../README.md](../README.md)。
> Hugging Face Space 通过 GitHub Actions 仅同步本目录（见仓库 `.github/workflows/sync_to_hf.yml`），所以 Space 看到的根就是这里。

Emomo 是一个基于 Go + Qdrant + 多模态 Embedding 的表情包语义搜索系统，支持本地静态图片目录摄入、图片向量检索、caption/keyword 辅助检索和元数据管理。当前默认链路使用 Qwen3-VL：导入时直接为图片生成 image 向量，搜索时将用户文本嵌入到同一语义空间并与图片向量匹配；VLM 描述和 OCR 仍会生成，但主要作为 caption/BM25 辅助信号和展示元数据。表情包资源只支持静态图片；GIF 文件不再支持，也不会被摄入。

关系库当前收敛为三张核心表：`memes`、`meme_annotations`、`meme_vectors`。protobuf message schema 定义在 `proto/emomo/v1/{types,meme,api}.proto`，生成代码在 `gen/emomo/v1/`（导入为 `pb`）。本项目把 protobuf 限定在 API DTO、前后端生成类型、跨边界封闭枚举，以及少量结构化 DB JSON 值（当前仅 `memes.image_info` / `meme_annotations.labels`，通过 `protojson` + `UseEnumNumbers=true` 序列化）。关系表结构、索引、迁移、运行时配置、开放业务集合和 UI 状态不由 protobuf 管。它**不是** RPC 协议——HTTP API 仍是 RESTful（POST `/api/v1/search` 等），handler 直接对 protobuf message 做 `protojson.Marshal/Unmarshal`。

数据库迁移完全由代码托管：`internal/repository/db.go` 中的 GORM AutoMigrate 加上 `prepareLegacy*` / `migrate*` / `dropLegacy*` 这一组迁移函数是唯一的事实来源（single source of truth）；项目**不使用** goose / golang-migrate / atlas 等独立 SQL 迁移工具，也不存在 `backend/migrations/` 目录。新增 schema 演进需要修改 `db.go`，并在 `db_test.go` 里补一条 SQLite 集成测试。

三张核心表**不启用 Row Level Security**——访问控制完全在连接层（service-role DSN）做。Supabase 部署的前提是不通过 Data API（PostgREST）暴露这些表给 anon/authenticated 角色；前端只调用 Go API，Go 后端持服务端连接读写 Postgres。`disableCoreTableRLS` 会在每次 `InitDB` 时强制把 RLS 关闭，老库即使之前 ENABLE 过也会被自动关回。

## 功能概览

- 多模态语义搜索：输入文字即可直接检索图片向量，默认融合 image、caption 和 keyword 三路结果。
- 数据摄入：支持本地静态图片目录分批摄入，仅接收静态图片并跳过 GIF 文件。
- 向量管理：支持多 Embedding 模型、多集合和 image/caption 多路向量管理。
- 存储抽象：兼容 Cloudflare R2、AWS S3 与其他 S3 兼容服务。
- 可扩展：查询扩展、VLM/OCR 辅助分析与多模型配置均可开关。

## 技术栈

- **后端**: Go + Gin + GORM
- **Protobuf message schema**: protobuf-go + protobuf-es + buf
- **向量数据库**: Qdrant (gRPC)
- **元数据存储**: SQLite (本地) / PostgreSQL (生产)
- **对象存储**: S3 兼容存储（Cloudflare R2、AWS S3 等）
- **Embedding**: Qwen3-VL 多模态 Embedding / Jina / OpenAI-compatible Embeddings
- **VLM/OCR**: OpenAI-compatible API (caption、OCR、keyword 辅助分析)

## 环境要求

- Go 1.26.2（见 `go.mod`）
- Docker（可选，用于本地 Qdrant/MinIO 或日志采集）

## 快速开始（本地开发）

### 1) 准备环境变量

```bash
cp .env.example .env
# 编辑 .env 填入 API Keys 和服务地址
```

### 2) 准备依赖服务

本项目不会自动启动 Qdrant 或对象存储，你可以选择云服务或本地服务。

**推荐：云服务（Qdrant Cloud + Cloudflare R2）**

```bash
# Qdrant Cloud (gRPC)
QDRANT_HOST=your-cluster.qdrant.io
QDRANT_PORT=6334
QDRANT_API_KEY=your-qdrant-api-key
QDRANT_USE_TLS=true

# Cloudflare R2
STORAGE_TYPE=r2
STORAGE_ENDPOINT=your-account-id.r2.cloudflarestorage.com
STORAGE_ACCESS_KEY=your-r2-access-key
STORAGE_SECRET_KEY=your-r2-secret-key
STORAGE_BUCKET=memes
STORAGE_REGION=auto
STORAGE_USE_SSL=true
STORAGE_PUBLIC_URL=https://pub-xxx.r2.dev
```

**本地体验：Qdrant + MinIO（S3 兼容）**

```bash
# Qdrant
docker run -d --name qdrant -p 6333:6333 -p 6334:6334 qdrant/qdrant:latest

# 本地 Qdrant 配置
QDRANT_HOST=localhost
QDRANT_PORT=6334
QDRANT_USE_TLS=false

# MinIO
docker run -d --name minio -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=accesskey -e MINIO_ROOT_PASSWORD=secretkey \
  quay.io/minio/minio server /data --console-address ":9001"

# 本地存储配置
STORAGE_TYPE=s3compatible
STORAGE_ENDPOINT=localhost:9000
STORAGE_ACCESS_KEY=accesskey
STORAGE_SECRET_KEY=secretkey
STORAGE_BUCKET=memes
STORAGE_USE_SSL=false
```

**可选：使用 Docker Compose 启动 API + 日志采集（Grafana Alloy）**

```bash
# 在仓库根执行（compose 文件在 ../deployments）
cd ..
docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d
```

### 3) 准备数据源

把静态图片放到本地目录，例如：

```text
data/memes/
├── 猫猫/
│   └── 无语.jpg
└── 狗狗/
    └── 柴犬.webp
```

### 4) 摄入数据

```bash
# 使用导入脚本（推荐，无需预先编译）
./scripts/import-data.sh -p ./data/memes -l 100

# 或使用 go run 直接运行
go run ./cmd/ingest --source=localdir --path=./data/memes --limit=100
```

### 5) 启动 API 服务

```bash
# 直接运行
go run ./cmd/api

# 或构建二进制
go build -o api ./cmd/api
./api
```

服务默认运行在 `http://localhost:8080`，健康检查 `http://localhost:8080/health`。

## API 示例

### 文本搜索

```bash
curl -X POST http://localhost:8080/api/v1/search \
  -H "Content-Type: application/json" \
  -d '{"query": "无语", "top_k": 20}'
```

### 获取分类列表

```bash
curl http://localhost:8080/api/v1/categories
```

### 获取表情包列表

```bash
curl "http://localhost:8080/api/v1/memes?category=猫猫表情&limit=20"
```

### 获取单个表情包

```bash
curl http://localhost:8080/api/v1/memes/{id}
```

### 获取统计信息

```bash
curl http://localhost:8080/api/v1/stats
```

## 配置说明

- 默认配置文件：`configs/config.yaml`
- 可通过 `CONFIG_PATH` 指定配置文件路径（默认 `configs/config.yaml`）
- `.env` 用于注入 API keys 与运行时环境变量

常用环境变量：

| 配置项 | 环境变量 | 说明 |
|--------|----------|------|
| vlm.api_key | OPENAI_API_KEY | OpenAI-compatible API Key（VLM/OCR/查询扩展） |
| vlm.base_url | OPENAI_BASE_URL | OpenAI-compatible Base URL |
| embeddings[].api_key_env | SILICONFLOW_API_KEY | 默认 Qwen3-VL 多模态 Embedding API Key |
| embeddings[].base_url_env | SILICONFLOW_BASE_URL | 默认 Qwen3-VL Embedding API Base URL |
| storage.type | STORAGE_TYPE | 存储类型：r2, s3, s3compatible |
| storage.endpoint | STORAGE_ENDPOINT | 存储端点（不含 bucket） |
| storage.bucket | STORAGE_BUCKET | 存储桶名称 |
| storage.region | STORAGE_REGION | 存储区域（R2 使用 `auto`） |
| storage.use_ssl | STORAGE_USE_SSL | 是否使用 HTTPS |
| storage.public_url | STORAGE_PUBLIC_URL | 公开访问 URL（R2 推荐） |
| qdrant.host | QDRANT_HOST | Qdrant 地址 |
| qdrant.port | QDRANT_PORT | Qdrant gRPC 端口（默认 6334） |
| qdrant.api_key | QDRANT_API_KEY | Qdrant Cloud API Key |
| qdrant.use_tls | QDRANT_USE_TLS | Qdrant TLS（Cloud 建议 true） |

## 开发与测试

```bash
# 运行 Go 测试
go test ./...

# 启动 API（热更新自行使用 air/其他工具）
go run ./cmd/api
```

### 更新 protobuf 消息 schema

```bash
GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate
```

修改 `proto/emomo/v1/` 下的任意 `.proto` 后需要重新生成 `gen/emomo/v1/*.pb.go`，并同步更新前端 `frontend/gen/`，再运行 `go test ./...`。

## 项目结构（backend/）

```
backend/
├── cmd/                 # Go 入口（api/ingest）
├── internal/            # Go 应用核心逻辑
│   ├── api/             # API 层
│   ├── config/          # 配置管理
│   ├── domain/          # 领域模型（GORM 结构体）
│   ├── repository/      # 数据访问层；db.go 统管全部迁移
│   ├── service/         # 业务逻辑层
│   ├── source/          # 数据源适配器
│   └── storage/         # 对象存储
├── configs/             # 配置文件
├── proto/               # 手写 protobuf message schema（types/meme/api 三个 .proto）
├── gen/                 # buf generate 输出的 Go 代码（不要手改）
├── data/                # 本地数据目录（被 gitignore，仅保留 .gitkeep）
├── scripts/             # 后端脚本（import-data / setup / check-data-dir）
└── Dockerfile           # 后端镜像
```

仓库内其他兄弟目录（顶层）：

- `../frontend/`：React + Vite 前端
- `../deployments/`：跨服务的 Docker Compose 编排
- `../docs/`：跨服务设计与使用文档
- `../scripts/start.sh`：本机一键起前后端

## 更多文档

- `../docs/QUICK_START.md`
- `../docs/INGEST.md`
- `../docs/DEPLOYMENT.md`
- `../docs/MULTI_EMBEDDING.md`
- `../docs/DATABASE_SCHEMA.md`

## License

MIT
