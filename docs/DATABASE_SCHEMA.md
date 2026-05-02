# Emomo 数据库结构

当前项目把关系型数据库收敛为 3 张核心表：

1. `memes`：表情包图片本体
2. `meme_annotations`：VLM/OCR 生成的语义补充和结构化标签
3. `meme_vectors`：关系库中的 Qdrant point 索引记录

检索主链路不是“图片 -> VLM 描述 -> 文本向量”。当前默认链路是：导入时直接生成图片向量并写入 Qdrant；搜索时把用户文本编码到同一个多模态向量空间，直接和图片向量做相似度匹配。`meme_annotations` 只用于展示、OCR、标签筛选和 caption/BM25 辅助信号。

生产环境使用 Supabase/PostgreSQL 时，三张核心表位于 `public` schema，但浏览器端不直接访问这些表。前端只调用 Go API；Go 后端使用服务端数据库连接访问 Postgres。因此三张核心表启用 Row Level Security，不为 `anon` / `authenticated` 角色创建默认读写 policy，避免 Supabase Data API 暴露表数据。

## IDL

类型契约定义在 `backend/proto/emomo/v1/schema.proto`，Go 生成代码在 `backend/internal/idl/emomo/v1/schema.pb.go`。

**IDL 范围（重要）**：本项目的 protobuf 仅作为"列级结构化值"和"封闭枚举"的 schema 来源——并不是 wire / RPC 协议。HTTP API 直接使用 Go struct + `encoding/json`；schema.proto 里**不包含** `Meme` / `MemeAnnotation` / `MemeVector` 这种顶层表行 message，那些行模型由 GORM 结构体（`backend/internal/domain/`）定义。

重新生成 IDL 代码：

```bash
cd backend
go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate
```

有限集合用 protobuf enum 定义，数据库里按整数存储；开放集合仍用文本：

| 类型 | IDL | 数据库存储 |
|---|---|---|
| 图片格式 | `ImageFormat` | `image_info.format` 内的 enum number |
| 向量类型 | `VectorType` | `meme_vectors.vector_type INTEGER` |
| 图片信息 | `ImageInfo` | `memes.image_info TEXT(JSON)` |
| 分析标签 | `MemeAnnotationLabels` | `meme_annotations.labels TEXT(JSON)` |

`category`、`tags`、`collection`、`embedding_model` 不做枚举，因为它们是业务/配置开放值。

## 迁移策略

数据库迁移**完全由 Go 代码托管**，single source of truth 是 `backend/internal/repository/db.go`：

- GORM `AutoMigrate(&domain.Meme{}, &domain.MemeAnnotation{}, &domain.MemeVector{})` 负责"按当前模型创建/扩展"的部分；
- `prepareLegacyMemesForAutoMigrate` / `prepareLegacyMemeVectorsForAutoMigrate` 处理老 schema 在 SQLite/Postgres 上的预处理（含 SQLite 整表重建）；
- `migrateMemes` / `migrateMemeAnnotations` / `migrateMemeVectorIndexes` 做数据回填与索引重建；
- `dropLegacyArtifacts`（含 `dropLegacyMemesColumns` / `dropLegacyMemeVectorsColumns` / `finalizeMemesConstraints` / `dropLegacyIndexes` / `dropLegacyTables`）清理废弃列、表、索引并补上 NOT NULL/DEFAULT 约束（仅 Postgres 需要）；
- `migrateCoreTableSecurity` 在 Postgres 上启用三张核心表的 RLS。

项目**不使用**独立的 SQL 迁移工具（goose / golang-migrate / atlas 等），也**不存在** `backend/migrations/` 目录。新增 schema 演进时：

1. 修改 `backend/internal/domain/` 的 GORM 模型或 `backend/proto/emomo/v1/schema.proto` 的结构化值/枚举（IDL 改动跑 `buf generate`）；
2. 在 `db.go` 里增改对应迁移函数；
3. 在 `internal/repository/db_test.go` 中加一条 SQLite 集成测试覆盖"老 schema → 新 schema"路径，必要时也在 `db_postgres_integration_test.go` 加 Postgres 集成测试。

## memes

```sql
CREATE TABLE memes (
    id TEXT PRIMARY KEY,
    storage_key TEXT NOT NULL,
    content_hash TEXT NOT NULL UNIQUE,
    image_info TEXT NOT NULL DEFAULT '{}',
    tags TEXT,
    category TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

ALTER TABLE memes ENABLE ROW LEVEL SECURITY;
```

字段说明：

| 字段 | 说明 |
|---|---|
| `id` | 表情包 ID |
| `storage_key` | 对象存储里的图片 key |
| `content_hash` | 处理后图片内容 hash，用于去重 |
| `image_info` | 图片固有信息，protobuf `ImageInfo` 的 JSON 表示 |
| `tags` | 导入时得到的普通标签数组 JSON |
| `category` | 导入时得到的大类 |
| `created_at` / `updated_at` | 记录时间 |

`image_info` 示例：

```json
{
  "width": 512,
  "height": 512,
  "format": 1
}
```

其中 `format` 来自 protobuf `ImageFormat`：`1=JPEG`、`2=PNG`、`3=WEBP`。

不再保留 `source_type`、`source_id`、`local_path`、`is_animated`、`file_size`、`md5_hash`、`perceptual_hash`、`status`。当前导入以内容 hash 去重，静态图片策略下也不需要为动画状态和本机路径建一级字段。

## meme_annotations

```sql
CREATE TABLE meme_annotations (
    id TEXT PRIMARY KEY,
    meme_id TEXT NOT NULL,
    analyzer_model TEXT NOT NULL,
    description TEXT,
    ocr_text TEXT,
    labels TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE UNIQUE INDEX idx_meme_annotations_meme_model
    ON meme_annotations(meme_id, analyzer_model);

ALTER TABLE meme_annotations ENABLE ROW LEVEL SECURITY;
```

字段说明：

| 字段 | 说明 |
|---|---|
| `id` | annotation ID |
| `meme_id` | 关联 `memes.id` |
| `analyzer_model` | 生成该分析结果的模型名 |
| `description` | 图片语义描述，用于展示和辅助索引 |
| `ocr_text` | OCR 识别出的图片文字 |
| `labels` | protobuf `MemeAnnotationLabels` 的 JSON 表示 |
| `created_at` / `updated_at` | 记录时间 |

`labels` 示例：

```json
{
  "text": {
    "present": true
  }
}
```

“图片有没有文字”不作为一级字段，而是 `labels.text.present`。Qdrant payload 里的 `text_presence` 可以由这个结构化标签派生，用于检索过滤。

不再保留 `prompt_version`、`text_presence`、`text_char_count`、`text_language`、`facets`、`caption_text`、`bm25_text`、`status`、`error_message`。这些字段要么可推导，要么是索引构建过程数据，要么属于未落地的任务管理需求。

## meme_vectors

```sql
CREATE TABLE meme_vectors (
    id TEXT PRIMARY KEY,
    meme_id TEXT NOT NULL,
    collection TEXT NOT NULL,
    vector_type INTEGER NOT NULL DEFAULT 1,
    embedding_model TEXT NOT NULL,
    input_hash TEXT,
    annotation_id TEXT,
    qdrant_point_id TEXT NOT NULL,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE UNIQUE INDEX idx_meme_vectors_meme_collection_type
    ON meme_vectors(meme_id, collection, vector_type);

ALTER TABLE meme_vectors ENABLE ROW LEVEL SECURITY;
```

字段说明：

| 字段 | 说明 |
|---|---|
| `id` | 向量记录 ID |
| `meme_id` | 关联 `memes.id` |
| `collection` | Qdrant collection 名称 |
| `vector_type` | protobuf `VectorType`：`1=image`、`2=caption` |
| `embedding_model` | 生成该向量的模型名 |
| `input_hash` | 生成向量的输入 hash，用于判断是否需要重建 |
| `annotation_id` | 可选，caption 向量可关联 annotation |
| `qdrant_point_id` | Qdrant point UUID |
| `created_at` / `updated_at` | 记录时间 |

不再保留 `md5_hash`、`embedding_provider`、`embedding_mode`、`dimension`、`status`。这些信息要么已经由 `memes.content_hash`、配置或 Qdrant collection 表达，要么当前没有可执行的生命周期管理逻辑。

## 数据流

```text
本地图片
  -> 读取并标准化静态图片
  -> 计算 content_hash
  -> 上传对象存储
  -> 写 memes
  -> 直接生成 image embedding
  -> 写 Qdrant
  -> 写 meme_vectors
  -> 可选生成 meme_annotations(description / OCR / labels)
```

搜索：

```text
用户文本
  -> query embedding
  -> Qdrant image 向量相似度检索
  -> 可选按 category / labels.text.present 派生的 text_presence 过滤
  -> 用 memes 补充 storage/image_info/category/tags
```
