# Emomo 免费资源部署指南

> **monorepo 提示**：本文档原本针对纯后端仓库编写。后端代码已下沉到 `backend/`，所有形如 `./scripts/...` 或 `go run ./cmd/...` 的命令请先 `cd backend` 再执行。Render / Railway / Hugging Face 部署元数据已在仓库根更新（见根 [README.md](../README.md) 的"部署"章节）。

本文档介绍如何使用免费资源完整部署 Emomo 项目。

## 架构概览

项目包含以下组件：
- **前端**：React + Vite（可选，Vercel/自部署）
- **后端 API**：Go + Gin（需要部署）
- **向量数据库**：Qdrant（需要部署）
- **对象存储**：S3 兼容存储（Cloudflare R2、AWS S3 等）
- **元数据数据库**：SQLite / PostgreSQL
- **外部 API**：SiliconFlow/Qwen3-VL 多模态 Embedding、OpenAI Compatible VLM/OCR/查询扩展

> 说明：`deployments/docker-compose*.yml` 仅包含 API 与 Grafana Alloy，Qdrant 与对象存储需自行提供（云服务或本地容器）。
> 当前检索主链路是 Qwen3-VL 多模态 image embedding：导入时直接把图片写成向量，搜索时文本 query 直接匹配图片向量；VLM/OCR 只作为 annotation、caption/BM25 和展示元数据。
> 关系库核心表是 `memes`、`meme_annotations`、`meme_vectors`，以及只记录来源、不进检索的 `meme_metadata`；protobuf 消息 schema 定义在 `backend/proto/emomo/v1/{types,meme,api}.proto`，生成代码集中在 `backend/gen/` 与 `frontend/gen/`。
> 使用 Supabase/PostgreSQL 时，核心表不启用 Row Level Security（`disableCoreTableRLS` 在每次 `InitDB` 强制关闭）；浏览器端不直连这些表，访问控制在服务端连接层完成，前端应继续通过 Go API 访问数据。

## CORS 配置（前端部署在 Vercel）

当前端部署在 Vercel 时，需要配置后端 CORS 以允许来自 Vercel 域名的跨域请求。

### 配置方式

在 `configs/config.yaml` 中配置 CORS：

```yaml
server:
  port: 8080
  mode: release
  cors:
    # 生产环境建议设置为 false，使用 allowed_origins 列表
    allow_all_origins: false
    # 添加你的 Vercel 域名（支持多个域名）
    allowed_origins:
      - "https://your-app.vercel.app"
      - "https://your-custom-domain.com"
      # 如果需要支持本地开发，可以添加：
      - "http://localhost:5173"
```

### 配置说明

1. **开发环境**：可以设置 `allow_all_origins: true` 以允许所有来源（不推荐用于生产）
2. **生产环境**：设置 `allow_all_origins: false`，并在 `allowed_origins` 中明确列出允许的域名
3. **Vercel 域名格式**：
   - 默认域名：`https://your-app-name.vercel.app`
   - 自定义域名：`https://your-domain.com`
   - 预览部署：`https://your-app-name-git-branch.vercel.app`

### 环境变量方式配置

也可以通过环境变量配置（优先级更高）：

```bash
# 设置为 false 以使用 allowed_origins 列表
SERVER_CORS_ALLOW_ALL_ORIGINS=false

# 使用逗号分隔的域名列表
SERVER_CORS_ALLOWED_ORIGINS=https://your-app.vercel.app,https://your-domain.com
```

### 验证配置

部署后，可以通过浏览器开发者工具检查：
1. 打开前端页面
2. 打开 Network 标签
3. 发起一个 API 请求
4. 检查响应头中的 `Access-Control-Allow-Origin` 是否包含你的前端域名

## 方案一：Oracle Cloud 免费 VPS（推荐）

### 优势
- ✅ 永久免费（2核 ARM CPU，4GB RAM，200GB 存储）
- ✅ 性能稳定，不会休眠
- ✅ 可运行 API + 日志采集，并可选自建 Qdrant/MinIO

### 步骤

#### 1. 创建 Oracle Cloud 账户
1. 访问 [Oracle Cloud](https://www.oracle.com/cloud/free/)
2. 注册账户（需要信用卡验证，但不会扣费）
3. 创建免费 ARM 实例（Ampere A1）

#### 2. 配置服务器
```bash
# SSH 连接到服务器
ssh opc@<your-server-ip>

# 更新系统
sudo yum update -y

# 安装 Docker 和 Docker Compose
sudo yum install -y docker docker-compose
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker opc

# 安装 Go（如果需要从源码构建）
sudo yum install -y golang git

# 重新登录以应用组权限
exit
```

#### 3. 准备外部服务（Qdrant + 对象存储）

推荐使用云服务（Qdrant Cloud + Cloudflare R2）。如果需要本地部署，可使用 Docker 启动 Qdrant 与 MinIO：

```bash
# 克隆项目
git clone <your-repo-url> emomo
cd emomo

# 本地 Qdrant（gRPC 端口 6334）
docker run -d --name qdrant -p 6333:6333 -p 6334:6334 qdrant/qdrant:latest

# 本地 MinIO（S3 兼容）
docker run -d --name minio -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=accesskey -e MINIO_ROOT_PASSWORD=secretkey \
  quay.io/minio/minio server /data --console-address ":9001"
```

#### 4. 配置防火墙
在 Oracle Cloud 控制台配置安全规则，开放端口：
- `8080`：后端 API
- `6334`：Qdrant gRPC（仅在自建 Qdrant 且需要外部访问时）
- `9000`：MinIO S3 API（仅在自建存储且需要外部访问时）

#### 5. 配置环境变量
```bash
# 创建后端 .env 文件
cd /home/opc/emomo
cat > backend/.env << EOF
# 对象存储配置（推荐使用 Cloudflare R2）
STORAGE_TYPE=r2
STORAGE_ENDPOINT=<account-id>.r2.cloudflarestorage.com
STORAGE_ACCESS_KEY=your-access-key
STORAGE_SECRET_KEY=your-secret-key
STORAGE_USE_SSL=true
STORAGE_BUCKET=memes
STORAGE_PUBLIC_URL=https://pub-xxx.r2.dev

# 或使用本地 S3 兼容存储（需要部署 MinIO 等服务）
# STORAGE_TYPE=s3compatible
# STORAGE_ENDPOINT=localhost:9000
# STORAGE_ACCESS_KEY=accesskey
# STORAGE_SECRET_KEY=secretkey
# STORAGE_USE_SSL=false
# STORAGE_BUCKET=memes

# Qdrant Cloud（gRPC）
QDRANT_HOST=your-cluster.qdrant.io
QDRANT_PORT=6334
QDRANT_API_KEY=your-qdrant-api-key
QDRANT_USE_TLS=true

# 或本地 Qdrant（gRPC）
# QDRANT_HOST=localhost
# QDRANT_PORT=6334
# QDRANT_USE_TLS=false

# OpenAI-compatible API (VLM/OCR/query expansion)
OPENAI_API_KEY=your-openai-key

# Qwen3-VL multimodal embedding
SILICONFLOW_API_KEY=your-siliconflow-key
SILICONFLOW_BASE_URL=https://api.siliconflow.cn/v1

# 可选配置
VLM_MODEL=gpt-4o-mini
QUERY_EXPANSION_MODEL=gpt-4o-mini
SEARCH_SCORE_THRESHOLD=0.0
EOF
```

#### 6. 构建并运行后端 API
```bash
cd /home/opc/emomo/backend

# 构建
go build -o api ./cmd/api

# 创建 systemd 服务（推荐）
sudo tee /etc/systemd/system/emomo-api.service > /dev/null << EOF
[Unit]
Description=Emomo API Service
After=network.target

[Service]
Type=simple
User=opc
WorkingDirectory=/home/opc/emomo/backend
EnvironmentFile=/home/opc/emomo/backend/.env
ExecStart=/home/opc/emomo/backend/api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable emomo-api
sudo systemctl start emomo-api
sudo systemctl status emomo-api
```

#### 7. 配置 Nginx 反向代理（可选，推荐）
```bash
# 安装 Nginx
sudo yum install -y nginx

# 配置 Nginx
sudo tee /etc/nginx/conf.d/emomo.conf > /dev/null << EOF
server {
    listen 80;
    server_name <your-domain-or-ip>;

    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

# 启动 Nginx
sudo systemctl enable nginx
sudo systemctl start nginx
```

#### 8. 配置 Vercel 前端环境变量
在 Vercel 项目设置中添加：
```
VITE_API_BASE=https://<your-domain-or-ip>/api/v1
```

如果没有域名，使用 IP：
```
VITE_API_BASE=http://<your-server-ip>:8080/api/v1
```

## 方案二：Railway（简单但需要信用卡）

### 优势
- ✅ 部署简单，支持 Docker
- ✅ 自动 HTTPS
- ✅ 可以部署多个服务

### 限制
- ⚠️ 需要信用卡（免费额度 $5/月）
- ⚠️ 超出免费额度会收费

### 步骤

#### 1. 准备 Qdrant
- 推荐使用 [Qdrant Cloud](https://cloud.qdrant.io/)（gRPC 端口 6334）
- 如需在 Railway 自建：使用 `qdrant/qdrant:latest` 镜像并确保 gRPC 端口 `6334` 可用

#### 2. 配置对象存储（推荐使用 Cloudflare R2）
1. 注册 Cloudflare 账户并创建 R2 bucket
2. 获取访问密钥和端点信息
3. 配置环境变量（见下方）

#### 3. 部署后端 API
1. 添加新服务，连接到 GitHub 仓库
2. 配置环境变量（参考方案一的步骤 5，设置 QDRANT_* 与 STORAGE_*）
3. Railway 会自动检测 Go 项目并构建

#### 4. 配置前端
在 Vercel 中设置：
```
VITE_API_BASE=https://your-api.railway.app/api/v1
```

## 方案三：混合方案（最经济）

### 架构
- **Qdrant**：使用 [Qdrant Cloud 免费 tier](https://cloud.qdrant.io/)（1GB 免费）
- **对象存储**：使用 [Cloudflare R2](https://www.cloudflare.com/products/r2/)（10GB 免费）
- **后端 API**：Railway 或 Render 免费 tier
- **前端**：Vercel（已部署）

### 步骤

#### 1. 设置 Qdrant Cloud
1. 注册 [Qdrant Cloud](https://cloud.qdrant.io/)
2. 创建免费集群
3. 获取 API 密钥和集群 URL

#### 2. 设置 Cloudflare R2
1. 注册 Cloudflare 账户
2. 创建 R2 bucket（名为 `memes`）
3. 获取 Access Key ID 和 Secret Access Key
4. 配置 CORS 和公开访问策略

#### 3. 配置对象存储
代码已支持 S3 兼容存储（包括 R2），无需修改代码。只需配置环境变量即可。

#### 4. 部署后端 API
使用 Railway 或 Render：
- Railway：连接 GitHub，自动构建
- Render：创建 Web Service，连接 GitHub

环境变量配置：
```bash
# Qdrant Cloud
QDRANT_HOST=your-cluster.qdrant.io
QDRANT_PORT=6334
QDRANT_COLLECTION=memes
QDRANT_API_KEY=your-api-key
QDRANT_USE_TLS=true

# Cloudflare R2（推荐使用新的统一配置格式）
STORAGE_TYPE=r2
STORAGE_ENDPOINT=your-account-id.r2.cloudflarestorage.com
STORAGE_ACCESS_KEY=your-r2-access-key
STORAGE_SECRET_KEY=your-r2-secret-key
STORAGE_USE_SSL=true
STORAGE_BUCKET=memes
STORAGE_PUBLIC_URL=https://pub-xxx.r2.dev

# VLM/OCR/query expansion + Qwen3-VL multimodal embedding
OPENAI_API_KEY=your-key
SILICONFLOW_API_KEY=your-key
SILICONFLOW_BASE_URL=https://api.siliconflow.cn/v1
```

## 方案四：Render（免费但会休眠）

### 优势
- ✅ 完全免费（免费 tier）
- ✅ 支持 Docker

### 限制
- ⚠️ 免费服务会在 15 分钟无活动后休眠
- ⚠️ 唤醒需要 30-60 秒

### 步骤
1. 访问 [Render](https://render.com/)
2. 创建 Web Service，连接 GitHub
3. 配置环境变量
4. 使用 Dockerfile 或直接构建 Go 应用

## Docker 部署说明

### 使用 Docker Compose 部署

`deployments/docker-compose.yml` 启动 API + Grafana Alloy，Qdrant 与对象存储需外部提供（云服务或本地容器）。
如仅需日志采集，可运行 `docker compose --env-file backend/.env -f deployments/docker-compose.yml up -d alloy`。

详细说明请参考 [`deployments/README.md`](../deployments/README.md)。

#### 重要：本地静态图片目录挂载

在使用 Docker 部署时，**必须确保本地静态图片目录被正确挂载**，否则 ingestion 会处理 0 个项目。默认目录为 `/root/data/memes`。

#### 云服务配置（推荐）

如果使用云服务，需要配置以下环境变量：

**Qdrant Cloud**：
```bash
export QDRANT_HOST=your-cluster.qdrant.io
export QDRANT_PORT=6334
export QDRANT_API_KEY=your-qdrant-cloud-key
export QDRANT_USE_TLS=true
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

或者使用配置文件 `configs/config.cloud.yaml.example` 作为参考，更新 `configs/config.yaml`。

#### 步骤

1. **准备数据目录**
   ```bash
   cd /path/to/emomo
   mkdir -p ./backend/data/memes
   # 把 .jpg/.jpeg/.png/.webp 静态图片放入 ./backend/data/memes
   
   # 确保目录结构正确
   ls -la ./backend/data/memes  # 应该看到图片文件
   ```

2. **检查 docker-compose.yml 挂载配置**
   
   确保 `docker-compose.yml` 中有以下挂载配置：
   ```yaml
   volumes:
     - ../backend/data:/root/data        # 挂载整个 data 目录
     - ../backend/configs:/root/configs  # 挂载配置文件
   ```
   
   这会将主机的 `./backend/data/memes` 挂载到容器的 `/root/data/memes`。

3. **启动服务**
   ```bash
   cd deployments
   docker compose --env-file ../backend/.env -f docker-compose.yml up -d
   ```

4. **验证目录挂载**
   ```bash
   # 进入容器检查
   docker exec -it emomo-api sh
   
   # 在容器内检查
   ls -la /root/data/memes
   # 应该能看到图片文件
   
   # 检查文件数量
   find /root/data/memes -type f | wc -l
   ```

5. **查看启动日志**
   ```bash
   docker logs emomo-api
   ```
   
   启动时会自动运行 `check-data-dir.sh` 脚本，检查目录是否存在。如果看到错误信息，说明目录未正确挂载。

#### 常见问题

**问题：Ingestion 处理了 0 个项目**

**原因**：本地静态图片目录未正确挂载到容器中。

**解决方案**：
1. 检查主机上的目录是否存在：`ls -la /path/to/emomo/backend/data/memes`
2. 检查 docker-compose.yml 中的挂载路径是否正确（在 `deployments/` 下使用相对路径 `../backend/data`，或使用绝对路径）
3. 确保目录包含静态图片文件（.jpg, .jpeg, .png, .webp）；GIF 不会被摄入
4. 重启容器：`docker compose --env-file ../backend/.env -f docker-compose.yml restart api`
5. 查看容器日志：`docker logs emomo-api`

**问题：使用绝对路径挂载**

如果项目不在 `deployments` 目录的上一级，可以使用绝对路径：

```yaml
volumes:
  - /absolute/path/to/emomo/backend/data:/root/data
  - /absolute/path/to/emomo/backend/configs:/root/configs
```

**问题：目录权限问题**

如果容器无法访问挂载的目录，检查权限：

```bash
# 确保目录可读
chmod -R 755 /path/to/emomo/backend/data/memes

# 如果使用非 root 用户运行容器，可能需要调整权限
```

## 数据摄入

部署完成后，需要摄入数据。`backend/scripts/import-data.sh` 是唯一支持的数据导入入口；API 不再提供导入端点，底层 Go worker 也不作为直接入口使用。

```bash
# 在服务器上或本地
cd emomo

# 确保数据目录存在，并放入 .jpg/.jpeg/.png/.webp 静态图片
mkdir -p ./backend/data/memes

# 使用唯一导入脚本（无需预先编译，默认导入目录中的全部图片）
cd backend
./scripts/import-data.sh -p ./data/memes
```

### 在 Docker 容器内摄入

```bash
# 进入容器
docker exec -it emomo-api sh

# 在容器内仍然使用 backend/scripts/import-data.sh
```

## 成本对比

| 方案 | 月成本 | 优点 | 缺点 |
|------|--------|------|------|
| Oracle Cloud VPS | $0 | 永久免费，性能好 | 需要信用卡验证 |
| Railway | $0-5 | 部署简单 | 需要信用卡，有额度限制 |
| Qdrant Cloud + R2 + Railway | $0-5 | 服务分离，易扩展 | 需要信用卡 |
| Render | $0 | 完全免费 | 会休眠，响应慢 |

## 推荐配置

对于个人项目，推荐**方案一（Oracle Cloud VPS）**：
- 成本最低（$0）
- 性能稳定
- 可运行 API + 日志采集，并可选自建 Qdrant/MinIO
- 不会休眠

如果不想管理服务器，推荐**方案三（混合方案）**：
- Qdrant Cloud（免费 1GB）
- Cloudflare R2（免费 10GB）
- Railway/Render（免费 tier）

## 故障排查

### 后端无法连接 Qdrant
- 检查 Qdrant 是否运行：`docker ps`
- 确认 gRPC 端口是否正确（默认 `6334`）
- 如有 REST 端口，可用 `curl http://localhost:6333/health` 验证
- 检查防火墙规则

### 图片无法加载
- 检查对象存储 bucket 是否设置为公开读取
- 检查 CORS 配置
- 检查图片 URL 是否正确
- 如果使用 R2，确保配置了 `STORAGE_PUBLIC_URL`

### API 请求失败
- 检查 Vercel 环境变量 `VITE_API_BASE` 是否正确
- 检查后端日志：`sudo journalctl -u emomo-api -f`（systemd）或 `docker logs emomo-api`（Docker）
- 检查 CORS 配置

### Ingestion 处理 0 个项目（Docker 部署）
- **检查本地静态图片目录是否挂载**：
  ```bash
  # 检查容器内目录
  docker exec emomo-api ls -la /root/data/memes
  
  # 检查文件数量
  docker exec emomo-api find /root/data/memes -type f | wc -l
  ```
- **检查 docker-compose.yml 挂载配置**：
  - 确保 `../backend/data:/root/data` 挂载正确
  - 如果使用绝对路径，确保路径正确
- **检查主机上的目录**：
  ```bash
  ls -la /path/to/emomo/backend/data/memes
  ```
- **查看启动日志**：
  ```bash
  docker logs emomo-api | grep -i "local static\|data\|directory"
  ```
- **重启容器**：
  ```bash
  docker compose --env-file ../backend/.env -f docker-compose.yml restart api
  ```

## 安全建议

1. **使用 HTTPS**：配置域名和 SSL 证书（Let's Encrypt 免费）
2. **保护 API Keys**：不要提交到 Git，使用环境变量
3. **限制访问**：配置防火墙，只开放必要端口
4. **定期备份**：备份 SQLite 数据库和 Qdrant 数据

## 下一步

- [ ] 配置自定义域名
- [ ] 设置 SSL 证书（Let's Encrypt）
- [ ] 配置监控和日志
- [ ] 设置自动备份
- [ ] 优化性能（CDN、缓存等）
