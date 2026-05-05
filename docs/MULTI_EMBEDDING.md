# 多 Embedding 与多路检索

本文描述当前默认的数据导入和检索链路。旧版本曾主要依赖 “VLM 生成图片描述 -> text embedding -> Qdrant 检索”；当前默认链路已经改为 Qwen3-VL 多模态 embedding：导入时直接为图片生成 image 向量，搜索时把用户文本嵌入到同一语义空间，并直接与图片向量做相似度匹配。

VLM 描述和 OCR 仍然存在，但它们是 caption/keyword 辅助信号和展示元数据，不再是唯一或主检索路径。

protobuf message schema 维护在 `backend/proto/emomo/v1/`：`types.proto` 定义 `ImageFormat`、`VectorType`、`TextPresence` 等跨边界封闭枚举，以及 allowlisted DB JSON 结构 `ImageInfo`、`MemeAnnotationLabels`；`meme.proto` / `api.proto` 定义 API entity DTO 与 HTTP request/response 消息。数据库里只保留三张核心表：`memes`、`meme_annotations`、`meme_vectors`；表结构、索引和迁移由 GORM model + `backend/internal/repository/db.go` 管，不由 protobuf 管。

## 默认架构

`backend/configs/config.yaml` 中默认配置了两个 embedding 路由和一个 search profile：

```yaml
embeddings:
  - name: qwen3vl_image
    provider: siliconflow
    model: Qwen/Qwen3-VL-Embedding-8B
    document_mode: image
    dimensions: 1024
    collection: meme_image_qwen3vl_1024

  - name: qwen3vl_caption
    provider: siliconflow
    model: Qwen/Qwen3-VL-Embedding-8B
    document_mode: text
    dimensions: 1024
    collection: meme_caption_qwen3vl_1024
    is_default: true

search:
  default_profile: qwen3vl
  profiles:
    - name: qwen3vl
      image_embedding: qwen3vl_image
      caption_embedding: qwen3vl_caption
```

默认检索 profile 会融合三路结果：

| 路由 | Collection | 写入内容 | 查询方式 | 默认权重 |
|------|------------|----------|----------|----------|
| image | `meme_image_qwen3vl_1024` | 图片直接生成的多模态向量 | 文本 query 生成向量后直接搜图片向量 | 0.60 |
| caption | `meme_caption_qwen3vl_1024` | OCR、VLM 描述、分类、tags、情绪词拼出的 caption 文本向量 | 文本 query 生成向量后搜 caption 向量 | 0.30 |
| keyword | `meme_caption_qwen3vl_1024` | OCR、描述、tags 生成的 BM25 sparse vector | 原始 query 做 sparse/BM25 检索 | 0.10 |

## 导入流程

默认 profile 导入的核心步骤：

```text
localdir 扫描静态图片
  -> 校验真实图片格式，拒绝 GIF/非静态格式
  -> WebP 转 JPEG（用于存储和模型兼容）
  -> 计算处理后图片 content_hash
  -> 按 meme_id + collection + vector_type 检查缺失向量
  -> 新图上传到 S3/R2，写 memes 元数据和 image_info
  -> 获取或生成 VLM description + OCR text，写 meme_annotations
  -> image route: 图片 URL/data -> 多模态 embedding -> Qdrant image collection
  -> caption route: OCR/描述/category/tags/情绪词 -> text embedding + BM25 -> Qdrant caption collection
  -> 写 meme_vectors，记录 collection、vector_type、model、point_id、annotation_id
```

已有图片重跑新 collection 或新 vector type 时，会复用 `memes.storage_key` 和已有 `meme_annotations`，只补缺失向量。

注意：`meme_annotations.description` 不是主检索语料，`meme_vectors.vector_type` 也不是字符串；它是 protobuf `VectorType` enum number。

## 数据模型

### `memes`

保存图片本体元数据：`storage_key`、`content_hash`、`image_info`、`category`、自由 `tags`。`image_info` 是 protobuf `ImageInfo` 的 JSON 表示，集中存放宽、高和图片格式。

### `meme_annotations`

保存 VLM/OCR 分析结果和结构化标签：`meme_id`、`analyzer_model`、`description`、`ocr_text`、`labels`。`labels` 是 protobuf `MemeAnnotationLabels` 的 JSON 表示；“有没有文字”存为 `labels.has_text`（扁平 bool），不是一级列。

### `meme_vectors`

记录每一路向量写入结果。当前关键字段包括：

```text
meme_id
collection
vector_type        protobuf enum: 1=image, 2=caption
embedding_model
input_hash
annotation_id
qdrant_point_id
```

唯一约束按 `meme_id + collection + vector_type` 去重，因此同一张图可以同时拥有 image 向量和 caption 向量。

## Ingest Script

`backend/scripts/import-data.sh` is the only supported data ingest entrypoint. It accepts a local image directory via `-p` / `--path` and invokes the internal ingest worker for the selected profile.

默认导入当前 search profile：

```bash
cd backend
./scripts/import-data.sh -p ./data/memes
```

显式使用默认 profile：

```bash
./scripts/import-data.sh -p ./data/memes --profile qwen3vl
```

只导入单一路 embedding：

```bash
./scripts/import-data.sh -p ./data/memes -e qwen3vl_image
./scripts/import-data.sh -p ./data/memes -e qwen3vl_caption
```

补齐已有数据的新向量可使用脚本的 retry 模式：

```bash
./scripts/import-data.sh -r -l 100
```

`cmd/reembed` 是已有数据的向量维护工具，不是数据导入入口。调整模型、collection 或 caption 构造逻辑后，可用它重建受影响的向量：

```bash
go run ./cmd/reembed --profile qwen3vl --vector-type all
go run ./cmd/reembed --profile qwen3vl --vector-type image
go run ./cmd/reembed --profile qwen3vl --vector-type caption
```

如果数据库还没有更新到 `meme_annotations` / `meme_vectors.annotation_id` schema，首次回填时加上 `--auto-migrate`。

## Search API

默认搜索会使用 `qwen3vl` profile：

```json
{
  "query": "无语",
  "top_k": 20
}
```

也可以显式指定 profile：

```json
{
  "query": "无语",
  "top_k": 20,
  "profile": "qwen3vl",
  "category": "学生党表情包",
  "text_presence": 2
}
```

`text_presence` 使用 protobuf enum number：`1=unknown`、`2=with_text`、`3=without_text`；`0` 或省略表示不筛选。

服务内部流程：

```text
query
  -> 可选 query expansion
  -> EmbedQuery(query) for image route
  -> Qdrant Search(image collection)
  -> EmbedQuery(query) for caption route
  -> Qdrant Search(caption collection)
  -> Qdrant SparseSearch(caption collection, original query)
  -> weighted rank fusion
  -> 用 memes 表补充 image_info/category/tags
```

## 配置字段说明

| 字段 | 说明 |
|------|------|
| `name` | embedding 配置名称，也是 API 中可用的单路 collection key |
| `provider` | `jina`、`modelscope`、`openai-compatible`、`siliconflow` |
| `model` | embedding 模型名 |
| `document_mode` | `image` 表示输入图片，`text` 表示输入 caption/query 文本 |
| `dimensions` | 向量维度，必须和 Qdrant collection 一致 |
| `collection` | 实际 Qdrant collection 名称 |
| `is_default` | 单路 fallback 默认 embedding |

这些配置字段属于运行时配置，不会逐项复制到 `meme_vectors`。关系库只记录当前必要的 `collection`、`vector_type`、`embedding_model`、`input_hash` 和 `qdrant_point_id`。

## 环境变量

当前默认配置需要：

```bash
SILICONFLOW_API_KEY=...
SILICONFLOW_BASE_URL=...
QDRANT_HOST=...
QDRANT_PORT=6334
QDRANT_API_KEY=...
QDRANT_USE_TLS=true
STORAGE_PUBLIC_URL=...
```

`VLM_MODEL` / `OPENAI_API_KEY` 仍用于 VLM 描述、OCR 和查询扩展，但默认 image route 的向量检索不依赖 “先把图片转成文字描述”。

## 最佳实践

- 默认优先使用 `--profile qwen3vl`，保证 image/caption/keyword 三路信号完整。
- 只验证图片向量质量时，可以单独导入或回填 `qwen3vl_image`。
- 调整模型、prompt 或 caption 构造逻辑后，应使用 `cmd/reembed` 回填受影响的 vector type。
- 不要把 `meme_annotations.description` 当成唯一检索语料；它只是辅助 caption/keyword 的一部分。
- “有没有文字”在关系库中来自 `meme_annotations.labels.has_text`。Qdrant payload 可派生 `text_presence=with_text/without_text/unknown` 作为过滤字段。
