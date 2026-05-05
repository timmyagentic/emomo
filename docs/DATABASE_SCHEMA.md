# Emomo 数据库结构

当前项目把关系型数据库收敛为 4 张核心表：

1. `memes`：表情包图片本体
2. `meme_annotations`：VLM/OCR 生成的语义补充和结构化标签
3. `meme_vectors`：关系库中的 Qdrant point 索引记录
4. `meme_metadata`：来源/出处元数据（爬虫平台、原标题、note id 等），仅用于追溯，不进检索索引

检索主链路不是“图片 -> VLM 描述 -> 文本向量”。当前默认链路是：导入时直接生成图片向量并写入 Qdrant；搜索时把用户文本编码到同一个多模态向量空间，直接和图片向量做相似度匹配。`meme_annotations` 只用于展示、OCR、标签筛选和 caption/BM25 辅助信号。`meme_metadata` 仅用于"这张图从哪来"的追溯，永远不参与 caption embedding、BM25、Qdrant payload 或前端 facet。

生产环境使用 Supabase/PostgreSQL 时，三张核心表位于 `public` schema，但浏览器端不直接访问这些表。前端只调用 Go API；Go 后端使用服务端（service-role）数据库连接访问 Postgres。

四张核心表**不启用 Row Level Security**：访问控制完全在连接层做（service-role DSN + 不通过 Supabase Data API 把这些表暴露给 anon/authenticated）。早期版本曾把 RLS 在这些表上 ENABLE 但没建任何 policy，那是个"隐式拒绝 + 靠 BYPASSRLS 偷渡"的半成品；当前 `InitDB` 会通过 `disableCoreTableRLS` 主动 DISABLE，新老库行为统一。

## Protobuf 设计边界

类型契约定义在 `backend/proto/emomo/v1/{types,meme,api}.proto`，Go 生成代码在 `backend/gen/emomo/v1/`，前端 TS 生成代码在 `frontend/gen/emomo/v1/`。

本项目使用 protobuf 的目标是减少前后端 API 类型漂移，并为少数结构化 JSON 值提供可生成、可演进的 schema。它不是“所有数据模型的总源头”。关系表结构、索引、迁移和查询语义仍由 GORM domain model 与 `backend/internal/repository/db.go` 管理。

**应该使用 protobuf 的地方：**

- HTTP request / response / SSE event 的消息体，例如 `SearchRequest`、`SearchResponse`、`SearchProgressEvent`。后端 handler 仍是手写 Gin REST endpoint，只是在 body 层用 `protojson` 编/解码。
- 前后端共享 DTO：Go 代码从 `backend/gen/emomo/v1/` 导入 `pb`，前端 API 层从 `frontend/gen/emomo/v1/` 导入 schema，再投影为 UI 类型。
- 跨 API 或持久化边界的封闭枚举，例如 `ImageFormat`、`VectorType`、`TextPresence`。已有 enum number 不得重排；新增值只能追加。
- 明确允许的 DB 列内 JSON 值。目前 allowlist 只有 `memes.image_info` (`ImageInfo`) 和 `meme_annotations.labels` (`MemeAnnotationLabels`)。这些列由 `backend/internal/persistence` 的 `protojson` GORM serializer 持久化，DB JSON 使用 `UseEnumNumbers=true` + `UseProtoNames=true`。

**不应该使用 protobuf 的地方：**

- 关系型表结构本身。`memes` / `meme_annotations` / `meme_vectors` 的列、索引、约束和迁移以 GORM model + `db.go` 为准，`.proto` 顶层 entity message 是 API 投影，不是数据库 DDL。
- 运行时配置和开放业务集合。`category`、`tags`、`collection`、`embedding_model`、`analyzer_model`、source ID 等是开放字符串，不做 protobuf enum。
- 需要频繁过滤、JOIN、排序或建索引的数据。如果某个 JSON 子字段成为核心查询条件，应优先提升为关系列，而不是继续塞进 protobuf JSON。
- repository / service 内部私有结构、临时计算结果、任务状态机内部类型。除非这些值跨 HTTP 边界或进入 allowlisted JSON 列，否则用普通 Go 类型。
- React 组件状态、组件 props、本地 fallback 数据。前端只在 `src/api/` 的网络边界使用生成类型，解码后应投影到 `src/types/` 中的 UI-owned shape。
- RPC 层。当前没有 protobuf `service`、gRPC、Connect 或 `google.api.http` annotation；引入这些需要单独设计，不应作为普通 schema 变更顺手加入。

新增 protobuf message / enum 前先回答两个问题：它是否跨 HTTP/API 边界，或是否属于 allowlisted DB JSON 列？如果答案都是否，就不应该放进 `.proto`。新增 DB JSON protobuf 列时，还必须同步更新本节 allowlist，并在 `internal/repository/db_test.go` 覆盖旧数据迁移和 `protojson` 读写行为。

## Schema-Level Types

重新生成 protobuf 代码：

```bash
cd backend
GOTOOLCHAIN=go1.26.2 go run github.com/bufbuild/buf/cmd/buf@v1.69.0 generate
```

有限集合用 protobuf enum 定义，数据库里按整数存储；开放集合仍用文本：

| 类型 | Schema-level type | 数据库存储 |
|---|---|---|
| 图片格式 | `ImageFormat` | `image_info.format` 内的 enum number |
| 向量类型 | `VectorType` | `meme_vectors.vector_type INTEGER` |
| 图片信息 | `ImageInfo` | `memes.image_info TEXT(JSON)` |
| 分析标签 | `MemeAnnotationLabels` | `meme_annotations.labels TEXT(JSON)` |

`category`、`tags`、`collection`、`embedding_model` 不做枚举，因为它们是业务/配置开放值。

## 迁移策略

数据库迁移**完全由 Go 代码托管**，single source of truth 是 `backend/internal/repository/db.go`：

- GORM `AutoMigrate(&domain.Meme{}, &domain.MemeAnnotation{}, &domain.MemeMetadata{}, &domain.MemeVector{})` 负责"按当前模型创建/扩展"的部分；
- `prepareLegacyMemesForAutoMigrate` / `prepareLegacyMemeVectorsForAutoMigrate` 处理老 schema 在 SQLite/Postgres 上的预处理（含 SQLite 整表重建）；
- `migrateMemes` / `migrateMemeAnnotations` / `migrateMemeVectorIndexes` 做数据回填与索引重建；
- `dropLegacyArtifacts`（含 `dropLegacyMemesColumns` / `dropLegacyMemeVectorsColumns` / `finalizeMemesConstraints` / `dropLegacyIndexes` / `dropLegacyTables`）清理废弃列、表、索引并补上 NOT NULL/DEFAULT 约束（仅 Postgres 需要）；
- `disableCoreTableRLS` 在 Postgres 上确保三张核心表的 RLS 处于关闭状态（旧库若已 ENABLE 也会被关回）。

项目**不使用**独立的 SQL 迁移工具（goose / golang-migrate / atlas 等），也**不存在** `backend/migrations/` 目录。新增 schema 演进时：

1. 修改 `backend/internal/domain/` 的 GORM 模型或 `backend/proto/emomo/v1/` 下任意 `.proto`（消息/枚举改动跑 `buf generate`，并同步前端 `npm run gen`）；
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
```

字段说明：

| 字段 | 说明 |
|---|---|
| `id` | 表情包 ID |
| `storage_key` | 对象存储里的图片 key |
| `content_hash` | 处理后图片内容 hash，用于去重 |
| `image_info` | 图片固有信息，protobuf `ImageInfo` 的 JSON 表示 |
| `tags` | 语义标签数组（JSON）。当前导入流水线**保持为空** —— 来自 `localdir` 等数据源的搜索词、note id、目录名都属于"出处元数据"，统一存到 `meme_metadata`，不再塞进这里。未来若 VLM 输出离散标签或人工打标，会重新启用本字段。 |
| `category` | 类别字符串。同样**当前保持为空**；爬虫的搜索关键词不是语义类别，故不在此处沉淀。后续若有真正的人工/VLM 分类，会重新启用。 |
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

“图片有没有文字”不作为一级字段，而是 `labels.has_text`（扁平 bool）。Qdrant payload 里的 `text_presence` 由这个结构化标签派生，用于检索过滤。

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

## meme_metadata

```sql
CREATE TABLE meme_metadata (
    id TEXT PRIMARY KEY,
    meme_id TEXT NOT NULL,
    source TEXT NOT NULL,
    source_item_id TEXT,
    source_url TEXT,
    title TEXT,
    author TEXT,
    published_at TEXT,
    search_keywords TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE UNIQUE INDEX idx_meme_metadata_source_item_meme
    ON meme_metadata(source, source_item_id, meme_id);
CREATE INDEX idx_meme_metadata_meme_id ON meme_metadata(meme_id);
CREATE INDEX idx_meme_metadata_source  ON meme_metadata(source);
```

字段说明：

| 字段 | 说明 |
|---|---|
| `id` | metadata row ID（surrogate） |
| `meme_id` | 关联 `memes.id`。同一张图（按 content hash 去重，meme_id 唯一）可以由不同 source 各贡献一行 metadata |
| `source` | 来源平台标识，例如 `xiaohongshu`、`chinesebqb`、`fabiaoqing`、`localdir`（手工目录）。adapter 自己定义命名。 |
| `source_item_id` | 在 `source` 命名空间下的资源 ID（小红书 = `note_id`；手工目录 = 相对路径）。可为空 |
| `source_url` | 可选，回链原始资源页 |
| `title` | 来源平台的标题。小红书 = 笔记 title；手工目录 = 文件名（不含扩展） |
| `author` | 上传者/作者，可为空，不解析 |
| `published_at` | 原始发布时间字符串（来源格式各异，不做规范化） |
| `search_keywords` | JSON 数组，记录爬虫所有命中此条的搜索词；用于追溯"为什么这张图进了语料"，不进检索索引 |
| `created_at` / `updated_at` | 记录时间 |

唯一键 `(source, source_item_id, meme_id)` 的语义：

- 同图被多个平台收录 → 多条 metadata 共享 `meme_id`。
- 同源同 item 重复导入 → ON CONFLICT UPDATE，不会产生重复行。
- ingest 服务通过 `MemeMetadataRepository.Upsert` 写入；surrogate `id` 在冲突时保留旧值。

**为什么单独一张表，而不是塞进 memes**：搜索链路只关心 `memes` / `meme_annotations` / `meme_vectors`。爬虫元数据如果塞进 `memes.tags` 或 `memes.category`，就会被 caption embedding / BM25 sparse / Qdrant payload 间接吃进去，污染语义检索。把它隔离到独立表里，并在 ingest pipeline 中**只写不读**（`upsertMetadata` 调用一次后就再不被任何检索代码触碰），可以保证检索质量与导入历史完全解耦。

## 数据流

```text
本地图片
  -> 读取并标准化静态图片
  -> 计算 content_hash
  -> 上传对象存储
  -> 写 memes（category/tags 当前留空）
  -> 写 meme_metadata（来源/标题/note_id/搜索词等出处信息）
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
  -> 可选按 category / labels.has_text 派生的 text_presence 过滤
  -> 用 memes 补充 storage/image_info/category/tags
```
