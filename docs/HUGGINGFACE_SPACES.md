# Hugging Face Spaces 部署指南

> **monorepo 提示**：HF Space 通过 GitHub Actions 同步的是 `backend/` 子树（见仓库根 `.github/workflows/sync_to_hf.yml`），所以 Space 文件系统的根 = 当前仓库的 `backend/`。本文中提到的 `Dockerfile`、`configs/`、`scripts/` 等路径在 Space 内仍然在根目录有效。

## 问题说明

Hugging Face Spaces 只运行单个 Docker 容器，**不包含 Qdrant 和对象存储服务**。因此需要配置外部服务。

当前默认检索链路使用 Qwen3-VL 多模态 image embedding：导入时直接生成图片向量，搜索时文本 query 直接匹配图片向量。VLM/OCR 只作为 `meme_annotations` 的辅助分析数据。关系库核心表为 `memes`、`meme_annotations`、`meme_vectors`，protobuf 消息 schema 定义在 `proto/emomo/v1/`（`types.proto` / `meme.proto` / `api.proto`），生成 Go 代码在 `gen/`。

## 解决方案

### 1. 使用 Qdrant Cloud（推荐）

1. 注册 [Qdrant Cloud](https://cloud.qdrant.io/) 账户（免费套餐可用）
2. 创建集群并获取连接信息：
   - Host: `xxx.qdrant.io`
   - Port: `6334` (gRPC + TLS)
   - API Key: `your-api-key`

### 2. 使用兼容 S3 的对象存储

选项 A：使用 [Backblaze B2](https://www.backblaze.com/b2/cloud-storage.html)（免费 10GB）
- 兼容 S3 API
- 免费额度充足

选项 B：使用 [Cloudflare R2](https://www.cloudflare.com/products/r2/)（免费 10GB/月）⭐ **推荐**
- 兼容 S3 API
- 无出站流量费用
- 详细配置指南：参见 [CLOUDFLARE_R2_SETUP.md](./CLOUDFLARE_R2_SETUP.md)

选项 C：使用其他云存储（AWS S3、阿里云 OSS 等）

## 环境变量配置

生产环境启用配置中心后，Hugging Face Spaces 只需要保留配置中心的
bootstrap 变量和读 token。Qdrant API key、数据库 URL、对象存储密钥、
模型 API key、Loki 密码等高敏感值必须放在 Cloudflare Secrets Store，
并通过配置中心 payload 的 `*_secret` 字段引用。

如果暂时没有启用配置中心，才使用下面各服务的 legacy 环境变量。

### Qdrant 配置

```bash
QDRANT_HOST=your-cluster.qdrant.io
QDRANT_PORT=6334
QDRANT_USE_TLS=true
# Legacy only when CONFIG_CENTER_ENABLED=false:
QDRANT_API_KEY=your-qdrant-api-key
```

### 存储配置（使用新的统一配置）

#### Cloudflare R2（推荐）

```bash
STORAGE_TYPE=r2
STORAGE_ENDPOINT=https://<YOUR_ACCOUNT_ID>.r2.cloudflarestorage.com
STORAGE_USE_SSL=true
STORAGE_BUCKET=<YOUR_BUCKET_NAME>
STORAGE_PUBLIC_URL=https://pub-<random-id>.r2.dev  # 可选
# Legacy only when CONFIG_CENTER_ENABLED=false:
STORAGE_ACCESS_KEY=<YOUR_ACCESS_KEY_ID>
STORAGE_SECRET_KEY=<YOUR_SECRET_ACCESS_KEY>
```

**如何获取访问密钥**：参见 [CLOUDFLARE_R2_SETUP.md](./CLOUDFLARE_R2_SETUP.md)

#### Backblaze B2（使用统一配置格式）

```bash
STORAGE_TYPE=s3
STORAGE_ENDPOINT=s3.us-west-000.backblazeb2.com
STORAGE_USE_SSL=true
STORAGE_BUCKET=your-bucket-name
# Legacy only when CONFIG_CENTER_ENABLED=false:
STORAGE_ACCESS_KEY=your-access-key
STORAGE_SECRET_KEY=your-secret-key
```

**注意**：使用 `STORAGE_*` 环境变量配置 S3 兼容存储。

### API Keys

```bash
OPENAI_BASE_URL=https://openrouter.ai/api/v1
VLM_MODEL=qwen/qwen-2.5-vl-7b-instruct:free
SILICONFLOW_BASE_URL=https://api.siliconflow.cn/v1
# Legacy only when CONFIG_CENTER_ENABLED=false:
OPENAI_API_KEY=your-openai-key
SILICONFLOW_API_KEY=your-siliconflow-key
```

### 配置中心（推荐）

Hugging Face Space 里手动维护环境变量比较麻烦。生产环境建议只在 Space
里保留一次性的配置中心地址和读 token。完整后端配置通过 Cloudflare
Worker + Workers KV 更新，高敏感密钥放在 Cloudflare Secrets Store，由
Worker 在返回配置时解析。

```bash
CONFIG_CENTER_ENABLED=true
CONFIG_CENTER_REQUIRED=true
CONFIG_CENTER_URL=https://your-worker.example.workers.dev/v1/config/emomo/production/emomo-api
CONFIG_CENTER_TOKEN=your-read-token
CONFIG_CENTER_POLL_INTERVAL=60s
CONFIG_CENTER_TIMEOUT=5s
```

本地更新后发布：

```bash
cd backend
CONFIG_CENTER_URL=https://your-worker.example.workers.dev/v1/config/emomo/production/emomo-api \
CONFIG_CENTER_ADMIN_TOKEN=your-admin-token \
./scripts/publish-config-center.sh
```

完整部署和安全说明见 [CONFIG_CENTER.md](./CONFIG_CENTER.md)。

## 临时解决方案：禁用 Qdrant 和对象存储

如果暂时无法配置外部服务，可以修改代码使应用在服务不可用时仍能启动（但搜索功能将不可用）。

### 修改 `cmd/api/main.go`

将 Qdrant 和对象存储初始化改为可选：

```go
// Initialize Qdrant (optional)
qdrantRepo, err := repository.NewQdrantRepository(...)
if err != nil {
    logger.Warn("Qdrant unavailable, search features disabled", zap.Error(err))
    qdrantRepo = nil
}

// Initialize storage (optional)
objectStorage, err := storage.NewStorage(&storage.S3Config{...})
if err != nil {
    logger.Warn("Storage unavailable, upload features disabled", zap.Error(err))
    objectStorage = nil
}
```

然后修改 `SearchService` 使其在 `qdrantRepo` 为 nil 时返回空结果而不是错误。

## 推荐架构

对于生产环境，建议：

1. **后端 API**: Hugging Face Spaces（免费）
2. **向量数据库**: Qdrant Cloud（免费套餐）
3. **对象存储**: Backblaze B2 或 Cloudflare R2（免费）
4. **前端**: Vercel（免费）

这样所有组件都可以使用免费资源。

## 注意事项

1. **端口配置**: Hugging Face Spaces **要求应用监听端口 7860**。Dockerfile 已配置默认端口为 7860，无需额外设置
2. **Qdrant API Key**: 当前代码版本已支持 Qdrant Cloud 的 API Key 认证
3. **HTTPS**: Qdrant Cloud 使用 HTTPS，设置 `QDRANT_API_KEY` 后会自动启用 TLS
4. **数据持久化**: Hugging Face Spaces 的存储是临时的，重启后会丢失 SQLite 数据，建议使用外部数据库（如 Supabase PostgreSQL）
