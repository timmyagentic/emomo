# Docker Compose 部署指南

本目录包含 Docker Compose 配置文件，用于启动 API 服务与 Grafana Alloy（日志采集）。
Qdrant 与对象存储需外部提供（云服务或本地容器）。

API 使用当前后端 schema：关系库核心表为 `memes`、`meme_annotations`、`meme_vectors`；默认检索链路直接使用图片向量，VLM/OCR 只作为辅助 annotation。Protobuf 消息 schema 定义在 `backend/proto/emomo/v1/` 下 `types.proto` / `meme.proto` / `api.proto`，生成代码集中在 `backend/gen/`（Go）与 `frontend/gen/`（TS）。

## 文件说明

- `docker-compose.yml` - 启动 API + Grafana Alloy（日志采集）

## 快速开始

1. 准备外部服务（Qdrant + S3 兼容存储）
2. 配置环境变量（推荐使用 `backend/.env`，Compose 已通过 `env_file` 读取；也可使用系统环境变量覆盖）
3. 启动服务：
   ```bash
   docker compose --env-file ../backend/.env -f docker-compose.yml up -d
   ```

## 环境变量配置

### Qdrant 配置（gRPC）

**本地 Qdrant**：
```bash
export QDRANT_HOST=localhost
export QDRANT_PORT=6334
export QDRANT_USE_TLS=false
```

**Qdrant Cloud**：
```bash
export QDRANT_HOST=your-cluster.qdrant.io
export QDRANT_PORT=6334
export QDRANT_API_KEY=your-api-key
export QDRANT_USE_TLS=true
```

### 存储配置

**本地 S3 兼容存储（MinIO）**：
```bash
export STORAGE_TYPE=s3compatible
export STORAGE_ENDPOINT=localhost:9000
export STORAGE_ACCESS_KEY=accesskey
export STORAGE_SECRET_KEY=secretkey
export STORAGE_BUCKET=memes
export STORAGE_USE_SSL=false
```

**Cloudflare R2**：
```bash
export STORAGE_TYPE=r2
export STORAGE_ENDPOINT=your-account-id.r2.cloudflarestorage.com
export STORAGE_ACCESS_KEY=your-r2-access-key
export STORAGE_SECRET_KEY=your-r2-secret-key
export STORAGE_BUCKET=memes
export STORAGE_REGION=auto
export STORAGE_USE_SSL=true
export STORAGE_PUBLIC_URL=https://pub-xxx.r2.dev  # 可选
```

**AWS S3**：
```bash
export STORAGE_TYPE=s3
export STORAGE_ENDPOINT=s3.amazonaws.com
export STORAGE_ACCESS_KEY=your-aws-access-key
export STORAGE_SECRET_KEY=your-aws-secret-key
export STORAGE_BUCKET=your-bucket-name
export STORAGE_REGION=us-east-1
export STORAGE_USE_SSL=true
```

## 常用命令

```bash
# 启动服务
docker compose --env-file ../backend/.env -f docker-compose.yml up -d

# 查看日志
docker compose --env-file ../backend/.env -f docker-compose.yml logs -f api

# 停止服务
docker compose --env-file ../backend/.env -f docker-compose.yml down

# 停止并删除数据卷（谨慎使用）
docker compose --env-file ../backend/.env -f docker-compose.yml down -v

# 重启服务
docker compose --env-file ../backend/.env -f docker-compose.yml restart api

# 查看运行状态
docker compose --env-file ../backend/.env -f docker-compose.yml ps
```

## 日志配置

API 默认持续输出 JSON 日志到 stdout，适合通过运行平台原生日志或 Docker logs 排查。若后端部署在 Hugging Face Space 这类单容器环境，推荐开启 API 直写 Loki，而不是依赖 Alloy sidecar：

```bash
LOKI_ENABLED=true
LOKI_URL=https://logs-prod-xxx.grafana.net/loki/api/v1/push
LOKI_USERNAME=your-grafana-cloud-instance-id
LOKI_PASSWORD=your-grafana-cloud-api-key
APP_ENV=production
CLUSTER_NAME=production
```

开启后，API 会保留 stdout，同时异步批量推送到 Loki。`project`、`service`、`environment`、`cluster`、`level`、`component` 会作为 Loki labels；`request_id`、`search_id`、`path`、`status`、`duration_ms` 等高基数字段会作为 structured metadata 和 JSON 日志内容发送。

Compose 部署仍可使用本目录的 Alloy 采集 Docker stdout；这条链路不需要 `LOKI_ENABLED=true`。

## 故障排查

### API 无法连接 Qdrant

1. 检查环境变量：
   ```bash
   docker exec emomo-api env | grep QDRANT
   ```
2. 确认 gRPC 端口与 TLS 配置是否正确（默认 `6334`）
3. 如为自建 Qdrant，检查容器是否运行：
   ```bash
   docker ps | grep qdrant
   ```

### API 无法连接存储

1. 检查环境变量：
   ```bash
   docker exec emomo-api env | grep STORAGE
   ```
2. 如为自建存储，检查容器是否运行：
   ```bash
   docker ps | grep minio
   ```
3. 验证存储配置：
   - 本地 S3 兼容存储：访问 http://localhost:9001 检查 bucket
   - Cloudflare R2：检查 R2 dashboard
   - AWS S3：检查 AWS Console

## 安全建议

1. **不要提交敏感信息**：使用环境变量或 secrets 管理
2. **使用 HTTPS**：生产环境必须使用 TLS
3. **限制访问**：配置防火墙规则
4. **定期备份**：备份数据库和重要数据
