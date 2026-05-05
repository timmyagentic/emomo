# AI 表情库项目路线图

> 技术栈：Golang + Qdrant + Qwen3-VL 多模态 Embedding + VLM/OCR 辅助分析
> 数据源：本地静态图片目录（MVP）、API/上传入口（扩展）
> 目标：快速跑通 MVP，同时预留多数据源扩展能力
> 备注：下文为早期规划示意，实际实现以当前仓库为准（Docker Compose 仅包含 API + Alloy，Qdrant/存储外部）。

> 当前实现更新：默认检索链路已经不是 “VLM 生成图片描述 -> Text Embedding -> 向量检索”。默认 `qwen3vl` profile 会在导入时直接为图片生成 image 向量；搜索时用户文本直接生成 query embedding，并与图片向量匹配。VLM 描述和 OCR 仍保留，但只作为 caption/BM25 辅助信号和展示元数据。详见 [MULTI_EMBEDDING.md](MULTI_EMBEDDING.md)。

---

## 一、整体架构设计

### 1.1 系统架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                           前端 (Web/App)                             │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    ▼                               ▼
            ┌─────────────┐                 ┌─────────────┐
            │  文本搜索    │                 │  图片搜索    │
            │  (文搜图)    │                 │  (图搜图)    │
            └─────────────┘                 └─────────────┘
                    │                               │
                    └───────────────┬───────────────┘
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         API Gateway (Gin)                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │ 文本搜索    │  │ 图片搜索    │  │ 分类浏览    │  │ 管理接口    │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
┌─────────────────────┐  ┌─────────────────┐  ┌─────────────────────┐
│   Search Service    │  │  Ingest Service │  │   VLM Service       │
│   (语义搜索核心)     │  │  (数据摄入编排)  │  │   (图片描述生成)     │
└─────────────────────┘  └─────────────────┘  └─────────────────────┘
          │                       │                       │
          │              ┌────────┴────────┐              │
          │              ▼                 ▼              ▼
          │    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
          │    │ Source Adapter  │  │ Source Adapter  │  │ Embedding       │
          │    │ (LocalDir)      │  │ (Upload/API)    │  │ Service         │
          │    └─────────────────┘  └─────────────────┘  │ (多模态/文本向量) │
          │              │                 │              └─────────────────┘
          │              │    ┌────────────┘                      │
          │              ▼    ▼                                   │
          │    ┌─────────────────────────────────────┐            │
          │    │       Data Source Interface         │            │
          │    │  (统一数据源抽象层 - 可扩展)          │            │
          │    └─────────────────────────────────────┘            │
          │                                                        │
          ▼                                                        ▼
┌─────────────────────┐                     ┌─────────────────────────────┐
│       Qdrant        │◄────────────────────│ Multimodal Embedding API    │
│   (向量数据库)       │                     │   (Qwen3-VL/Jina/etc.)      │
└─────────────────────┘                     └─────────────────────────────┘
          │                                               ▲
          ▼                                               │
┌─────────────────────┐         ┌─────────────────────┐   │
│       SQLite        │         │    对象存储 (S3)     │   │
│   (元数据 + 去重)    │         │   (图片文件存储)     │   │
│ [生产环境:PostgreSQL]│         └─────────────────────┘   │
└─────────────────────┘                                   │
                                          ┌───────────────┘
                                          │
                                ┌─────────────────────┐
                                │   VLM API           │
                                │ (GPT-4o/Qwen-VL)    │
                                └─────────────────────┘
```

### 1.2 核心设计原则

**数据源抽象层**：当前主数据源是本地静态图片目录，后续上传/API 数据源也应实现统一接口，无需修改核心摄入逻辑。数据源标识属于运行时适配器配置，不作为 `memes` 一级字段持久化。

**摄入与搜索分离**：数据摄入是后台异步任务，搜索是实时低延迟服务，两者独立扩展。

**多模态 Embedding + VLM/OCR 辅助方案**：
- 主检索路径：导入阶段直接对图片生成多模态 image 向量；文本搜索时对用户查询生成 query 向量，并与 image 向量匹配
- 辅助路径：VLM 描述、OCR、category、tags 和情绪词拼成 caption 文本，生成 caption 向量和 BM25 sparse 向量
- 默认融合：image、caption、keyword 三路结果按权重融合，image 路是当前主信号
- 优势：不再依赖图片先转文字，图片语义保真度更高；VLM/OCR 仍能补充文字梗、情绪词和关键词召回

**MVP 轻量化原则**：MVP 阶段使用 SQLite 替代 PostgreSQL，降低运维复杂度；待规模扩大后再迁移。

---

## 二、核心模块设计

### 2.1 数据源抽象层（关键扩展点）

定义统一的数据源接口，所有数据源适配器必须实现：

| 方法 | 说明 |
|------|------|
| `FetchBatch()` | 批量获取表情包（支持分页/游标） |
| `GetMetadata()` | 获取单个表情包的元数据 |
| `SupportsIncremental()` | 是否支持增量更新 |
| `GetSourceID()` | 返回数据源唯一标识 |

**MVP 阶段实现**：LocalDir Adapter（递归扫描本地静态图片目录）

**预留扩展**：
- 用户上传 Adapter
- 对象存储清单 Adapter
- API/Staging Adapter

### 2.2 摄入服务（Ingest Service）

负责编排数据从各数据源到存储层的完整流程：

```
数据源 → 读取/校验图片 → 去重检查 → 存储图片 → 生成 image/caption 向量 → 写入 Qdrant → 写入元数据
```

**关键设计**：
- 使用 Go 的 Worker Pool 模式控制并发
- 每个步骤独立，支持断点续传
- 失败通过日志和运行时统计暴露；已有图片缺失向量时通过 `--retry` / `cmd/reembed` 回填，不单独维护 `ingest_jobs` 表

### 2.3 VLM/OCR 辅助分析服务

负责生成图片描述和 OCR 文本，供 caption 向量、BM25 sparse 检索、结果展示和后续结构化筛选使用。它不再是默认 image 向量检索的前置步骤。

**Prompt 设计建议**：
```
请用简洁的中文描述这张表情包的内容，包括：
1. 图片中的主体（人物、动物、卡通形象等）
2. 表情或动作
3. 图片上的文字（如果有）
4. 表达的情绪或场景
输出格式：一段 50-100 字的描述
```

**VLM 选型对比**：

| 方案 | 单图成本 | 延迟 | 质量 | 适用阶段 |
|------|----------|------|------|----------|
| GPT-4o mini | ~$0.00015 | ~2s | 优秀 | MVP 首选 |
| Qwen-VL (API) | ~¥0.001 | ~1s | 良好 | 国内部署 |
| Claude 3 Haiku | ~$0.00025 | ~1.5s | 优秀 | 高质量备选 |
| LLaVA (本地) | 服务器成本 | ~500ms | 良好 | 大规模自建 |

**MVP 方案**：使用 GPT-4o mini，5000 张图预处理成本约 $0.75

### 2.4 多模态 Embedding 服务

默认使用 Qwen3-VL 多模态 embedding，同时支持 `document_mode=image` 和 `document_mode=text`：

- image route：图片 URL/data 直接生成图片向量
- query：用户文本生成同空间 query 向量，用于直接搜索 image collection
- caption route：OCR/描述/category/tags 生成 caption 文本向量

**选型对比**：

| 方案 | 延迟 | 成本 | 向量维度 | 适用阶段 |
|------|------|------|----------|----------|
| Qwen3-VL Embedding | 中 | API 成本 | 1024 | 当前默认，多模态文搜图 |
| Jina v4 image/text | 中 | API 成本 | 2048 | 可选 provider |
| BGE-M3 / 本地多模态模型 | 低到中 | 服务器成本 | 取决于模型 | 大规模自建 |

**当前默认方案**：使用 SiliconFlow Qwen/Qwen3-VL-Embedding-8B，分别维护 image 与 caption 两个 collection。

### 2.5 搜索服务

支持两种搜索模式：文本搜索（文搜图）和图片搜索（图搜图）。

#### 文本搜索流程（文搜图）
1. 接收用户输入的自然语言查询
2. 调用多模态 Embedding 服务生成查询向量
3. 在 image collection 中直接搜索图片向量
4. 同时在 caption collection 做 caption dense 搜索和 BM25 sparse 搜索
5. 融合 image、caption、keyword 三路结果
6. 从数据库补充元数据（可选，Qdrant Payload 已含基础信息）
7. 返回结果（含图片 URL、相似度分数、标签等）

#### 图片搜索流程（图搜图）
1. 接收用户上传的图片
2. 调用多模态 Embedding 服务直接生成图片 query 向量
3. 在 image collection 中执行相似度搜索，返回 TopK 结果
4. 返回结果

**图搜图优化策略**：
- 缓存：对上传图片计算内容 hash，缓存图片 query embedding 或近似查询结果
- 前端体验：显示「正在分析图片...」进度提示，预期延迟取决于多模态 embedding API

**Qdrant 配置建议**：
- Collection 使用余弦相似度（Cosine）
- 向量维度：1024（当前 Qwen3-VL 默认配置）
- 开启 HNSW 索引，ef=128, m=16
- Payload 存储：category, tags, annotation 派生的 vlm_description/ocr_text/text_presence, storage_url

---

## 三、数据模型设计

### 3.1 数据库选型策略

| 阶段 | 数据库 | 理由 |
|------|--------|------|
| MVP (< 5万图) | SQLite | 简单、零运维、单文件部署 |
| 扩展期 (5-50万图) | PostgreSQL | 并发支持、复杂查询 |
| 规模化 (50万+) | PostgreSQL + 读写分离 | 性能扩展 |

**迁移策略**：使用 GORM 作为 ORM，支持 SQLite → PostgreSQL 无缝切换，只需修改连接字符串。

### 3.2 SQLite 表结构（MVP）

**memes 表（核心）**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID) | 主键 |
| storage_key | TEXT | 对象存储路径 |
| content_hash | TEXT | 处理后图片内容 hash，精确去重 |
| image_info | TEXT | protobuf `ImageInfo` JSON，包含宽、高、格式 enum |
| tags | TEXT | 标签（JSON 数组格式） |
| category | TEXT | 分类 |
| created_at | TEXT | 入库时间（ISO8601） |
| updated_at | TEXT | 更新时间（ISO8601） |

**索引设计**：
```sql
CREATE UNIQUE INDEX idx_memes_content_hash ON memes(content_hash);
CREATE INDEX idx_memes_category ON memes(category);
```

**meme_annotations 表（VLM/OCR 辅助分析 + 结构化筛选）**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID) | 主键 |
| meme_id | TEXT | 关联 memes.id |
| analyzer_model | TEXT | 分析模型 |
| description | TEXT | VLM 描述，作为 caption/BM25 辅助信号 |
| ocr_text | TEXT | OCR 文字 |
| labels | TEXT | protobuf `MemeAnnotationLabels` JSON，`labels.has_text` 表示是否有文字 |
| created_at | TEXT | 创建时间 |
| updated_at | TEXT | 更新时间 |

**meme_vectors 表（多路向量记录）**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID) | 主键 |
| meme_id | TEXT | 关联 memes.id |
| collection | TEXT | Qdrant collection |
| vector_type | INTEGER | protobuf `VectorType`：1=image，2=caption |
| embedding_model | TEXT | Embedding 模型 |
| input_hash | TEXT | 向量输入哈希 |
| annotation_id | TEXT | 关联 meme_annotations.id |
| qdrant_point_id | TEXT | Qdrant Point ID |

当前不单独建 `data_sources` / `ingest_jobs` 表。导入来源由配置和导入命令表达，任务状态暂时保留在运行时统计和日志中，避免提前增加维护面。

### 3.3 Qdrant Collection 结构

**默认 Collections**

| Collection | 用途 | 向量维度 | 距离 |
|------------|------|----------|------|
| `meme_image_qwen3vl_1024` | 图片直接生成的 image 向量 | 1024 | Cosine |
| `meme_caption_qwen3vl_1024` | caption 文本向量 + BM25 sparse 向量 | 1024 | Cosine |

**Payload 字段设计**：

| 字段 | 类型 | 用途 |
|------|------|------|
| meme_id | string | 关联 SQLite 主键 |
| category | string | 分类过滤 |
| tags | string[] | 标签过滤 |
| text_presence | string | 文字筛选：with_text / without_text / unknown |
| vlm_description | string | annotation 派生的描述文本（展示/调试用，不是主检索语料） |
| ocr_text | string | OCR 文本 |
| storage_url | string | 图片 URL（直接返回，减少查库） |

**Payload 用于过滤**：搜索时可以附加条件，如「只搜索某分类」或「只搜索带文字/不带文字的表情包」。文字筛选的关系库来源是 `meme_annotations.labels.has_text`，Qdrant 中的 `text_presence` 是派生 payload。

**示例向量点结构**：
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "vector": [0.1, 0.2, ...],
  "payload": {
    "meme_id": "550e8400-e29b-41d4-a716-446655440000",
    "category": "猫猫表情",
    "tags": ["猫", "无语", "可爱"],
    "vlm_description": "一只橘猫露出无语的表情，眼神呆滞",
    "ocr_text": "我不理解",
    "text_presence": "with_text",
    "storage_url": "https://cdn.example.com/memes/abc123.jpg"
  }
}
```

---

## 四、核心流程时序图

本节保留部分早期时序图作为历史规划参考。当前实现以以下流程为准：

```text
导入:
localdir 静态图片
  -> 校验/转换格式
  -> 计算 content_hash
  -> 写 memes / 复用已有 storage_key
  -> 可选生成或复用 VLM description + OCR，写 meme_annotations
  -> image route: 图片直接生成向量 -> meme_image_qwen3vl_1024
  -> caption route: OCR/描述/category/tags 生成文本向量 + BM25 -> meme_caption_qwen3vl_1024
  -> 写 meme_vectors

搜索:
用户文本
  -> query expansion（可选）
  -> 文本 query embedding 直接搜索 image collection
  -> 文本 query embedding 搜索 caption collection
  -> 原始 query 搜索 BM25 sparse vector
  -> 三路结果按权重融合
```

### 4.1 预处理（摄入）时序图（历史文本化方案）

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│  Ingest  │  │  Source  │  │  Object  │  │   VLM    │  │Embedding │  │  Qdrant  │  │  SQLite  │
│ Service  │  │ Adapter  │  │ Storage  │  │ Service  │  │ Service  │  │          │  │          │
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │             │             │             │             │             │
     │ 1. FetchBatch()           │             │             │             │             │
     │────────────>│             │             │             │             │             │
     │             │             │             │             │             │             │
     │ 2. 返回图片列表 [{url, id, category}]   │             │             │             │
     │<────────────│             │             │             │             │             │
     │             │             │             │             │             │             │
     │ 3. 下载/标准化图片 & 计算 content_hash  │             │             │             │
     │─────────────────────────────────────────────────────────────────────────────────>│
     │             │             │             │             │             │ 4. 查询 content_hash │
     │             │             │             │             │             │    是否存在 │
     │<─────────────────────────────────────────────────────────────────────────────────│
     │             │             │             │             │             │             │
     │ [如果已存在，跳过；如果不存在，继续]     │             │             │             │
     │             │             │             │             │             │             │
     │ 5. 上传图片到对象存储     │             │             │             │             │
     │────────────────────────────>│           │             │             │             │
     │             │             │             │             │             │             │
     │ 6. 返回 storage_key       │             │             │             │             │
     │<────────────────────────────│           │             │             │             │
     │             │             │             │             │             │             │
     │ 7. 调用 VLM 生成图片描述  │             │             │             │             │
     │─────────────────────────────────────────>│            │             │             │
     │             │             │             │             │             │             │
     │ 8. 返回描述文本                          │             │             │             │
     │    "一只橘猫露出无语表情，眼神呆滞"      │             │             │             │
     │<─────────────────────────────────────────│            │             │             │
     │             │             │             │             │             │             │
     │ 9. 调用 Embedding 生成向量               │             │             │             │
     │──────────────────────────────────────────────────────>│             │             │
     │             │             │             │             │             │             │
     │ 10. 返回向量 [1024 维]                   │             │             │             │
     │<──────────────────────────────────────────────────────│             │             │
     │             │             │             │             │             │             │
     │ 11. 写入向量到 Qdrant (含 Payload)       │             │             │             │
     │─────────────────────────────────────────────────────────────────────>│            │
     │             │             │             │             │             │             │
     │ 12. 返回 point_id                        │             │             │             │
     │<─────────────────────────────────────────────────────────────────────│            │
     │             │             │             │             │             │             │
     │ 13. 写入元数据到 SQLite                  │             │             │             │
     │─────────────────────────────────────────────────────────────────────────────────>│
     │             │             │             │             │             │             │
     │ 14. 确认写入成功                         │             │             │             │
     │<─────────────────────────────────────────────────────────────────────────────────│
     │             │             │             │             │             │             │

预处理耗时估算（单张图片）：
- Step 3-4: ~10ms (content_hash + 查库)
- Step 5-6: ~100ms (上传存储)
- Step 7-8: ~2000ms (VLM 推理，主要耗时)
- Step 9-10: ~50ms (Text Embedding)
- Step 11-14: ~20ms (写入)
- 总计: ~2.2s/张，5000张约 3 小时
```

### 4.2 文本搜索时序图（历史单路方案）

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│  Client  │  │   API    │  │Embedding │  │  Qdrant  │  │  SQLite  │
│          │  │ Gateway  │  │ Service  │  │          │  │ (可选)   │
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │             │             │             │
     │ 1. POST /api/v1/search                  │             │
     │    {"query": "无语", "top_k": 20}       │             │
     │────────────>│             │             │             │
     │             │             │             │             │
     │             │ 2. 生成查询文本 Embedding  │             │
     │             │────────────>│             │             │
     │             │             │             │             │
     │             │ 3. 返回向量 [1024 维]      │             │
     │             │<────────────│             │             │
     │             │             │             │             │
     │             │ 4. 向量相似搜索            │             │
     │             │    (TopK=20, 可带过滤条件) │             │
     │             │────────────────────────────>│            │
     │             │             │             │             │
     │             │ 5. 返回结果                │             │
     │             │    [{point_id, score, payload}]         │
     │             │<────────────────────────────│            │
     │             │             │             │             │
     │             │ [可选] 6. 补充查询详细元数据             │
     │             │──────────────────────────────────────────>│
     │             │             │             │             │
     │             │ [可选] 7. 返回额外信息     │             │
     │             │<──────────────────────────────────────────│
     │             │             │             │             │
     │ 8. 返回搜索结果                          │             │
     │    [{url, score, description, tags}]    │             │
     │<────────────│             │             │             │
     │             │             │             │             │

响应时间预估：
- Step 2-3: ~50ms (Text Embedding)
- Step 4-5: ~30ms (Qdrant 向量搜索)
- Step 6-7: ~10ms (SQLite 查询，可选)
- 总计: ~100ms (P99 目标 < 200ms)
```

### 4.3 图片搜索时序图（当前多模态方案）

```
┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
│  Client  │  │   API    │  │  Cache   │  │Embedding │  │  Qdrant  │
│          │  │ Gateway  │  │ (可选)   │  │ Service  │  │          │
└────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │             │             │             │
     │ 1. POST /api/v1/search/image            │             │
     │    {"image": "<base64>", "top_k": 20}   │             │
     │────────────>│             │             │             │
     │             │             │             │             │
     │             │ 2. 计算图片 content_hash   │             │
     │             │────────────>│             │             │
     │             │             │             │             │
     │             │ 3. 查询缓存 (hash → 向量/结果)          │
     │             │<────────────│             │             │
     │             │             │             │             │
     │ [缓存命中则跳到 Step 7]   │             │             │
     │             │             │             │             │
     │             │ 4. 直接生成图片 query 向量 │             │
     │             │───────────────────────────>│            │
     │             │             │             │             │
     │             │ 5. 返回 image embedding    │             │
     │             │<───────────────────────────│            │
     │             │             │             │             │
     │             │ 6. 缓存图片向量/查询结果   │             │
     │             │────────────>│             │             │
     │             │             │             │             │
     │             │ 7. image collection 相似搜索             │
     │             │────────────────────────────────────────>│
     │             │             │             │             │
     │             │ 8. 返回结果 [{point_id, score, payload}] │
     │             │<────────────────────────────────────────│
     │             │             │             │             │
     │ 9. 返回搜索结果 [{url, score, description, tags}]    │
     │<────────────│             │             │             │
     │             │             │             │             │

响应时间预估：
- Step 2-3: ~5ms (content_hash + 缓存查询)
- Step 4-5: 取决于多模态 embedding API，缓存命中则跳过
- Step 6: ~5ms (缓存写入)
- Step 7-8: ~30ms (Qdrant 搜索)
- 总计: 主要取决于 image embedding 延迟
- 缓存命中: ~100ms
```

---

## 五、技术选型清单

### 5.1 后端技术栈

| 组件 | 选型 | 理由 |
|------|------|------|
| Web 框架 | Gin | 高性能、生态成熟 |
| ORM | GORM | 功能完善、支持 SQLite/PostgreSQL 无缝切换 |
| 配置管理 | Viper | 支持多格式、环境变量 |
| 日志 | Zap | 高性能结构化日志 |
| 任务队列 | Asynq (Redis) | Go 原生、轻量级（MVP 可选用简单协程池） |
| HTTP 客户端 | Resty | 链式调用、重试支持 |
| Qdrant 客户端 | go-qdrant (官方) | 官方维护 |

### 5.2 基础设施

| 组件 | MVP 选型 | 生产选型 | 备注 |
|------|----------|----------|------|
| 数据库 | SQLite | PostgreSQL 15+ | GORM 支持无缝迁移 |
| 向量数据库 | Qdrant | Qdrant (集群) | 开源、性能优秀 |
| 对象存储 | MinIO | S3 / 阿里云 OSS | MVP 用 MinIO 本地存储 |
| 缓存 | 内存 Map | Redis | MVP 阶段可不引入 Redis |
| 容器化 | Docker Compose | Kubernetes | MVP 阶段一键启动 |

### 5.3 外部 AI 服务

| 服务 | 用途 | MVP 方案 | 备选方案 |
|------|------|----------|----------|
| Multimodal Embedding API | image/caption/query 向量生成 | Qwen3-VL Embedding | Jina v4、其他多模态模型 |
| VLM/OCR API | 辅助描述、OCR、查询扩展 | OpenAI-compatible VLM | Qwen-VL、Claude、GPT-4o mini |
| 图片 CDN | 图片分发 | 直接用 MinIO | CloudFront / 阿里云 CDN |

---

## 六、目录结构规划

```
meme-library/
├── cmd/
│   ├── api/                    # API 服务入口
│   │   └── main.go
│   ├── ingest/                 # 摄入服务入口（CLI 工具）
│   │   └── main.go
│   └── worker/                 # 异步任务 Worker（可选）
│       └── main.go
│
├── internal/
│   ├── api/                    # API 层
│   │   ├── handler/            # HTTP 处理器
│   │   │   ├── search.go       # 搜索相关接口
│   │   │   ├── meme.go         # 表情包 CRUD
│   │   │   └── health.go       # 健康检查
│   │   ├── middleware/         # 中间件
│   │   └── router.go
│   │
│   ├── domain/                 # 领域模型
│   │   ├── meme.go             # 表情包实体
│   │   ├── meme_annotation.go  # VLM/OCR 分析与结构化 labels
│   │   └── meme_vector.go      # Qdrant point 索引记录
│   │
│   ├── service/                # 业务逻辑层
│   │   ├── search.go           # 搜索服务（文搜图 + 图搜图）
│   │   ├── ingest.go           # 摄入编排
│   │   ├── vlm.go              # VLM/OCR 辅助分析服务
│   │   └── embedding.go        # 多模态/文本 Embedding 服务
│   │
│   ├── repository/             # 数据访问层
│   │   ├── meme_repo.go        # SQLite/PostgreSQL 表情包存储
│   │   ├── meme_vector_repo.go # 向量索引记录存储
│   │   ├── qdrant_repo.go      # Qdrant 向量操作
│   │   └── meme_annotation_repo.go  # VLM/OCR 分析结果与筛选字段存储
│   │
│   ├── source/                 # 数据源适配器（扩展点）
│   │   ├── interface.go        # 统一接口定义
│   │   ├── localdir/           # 本地静态图片目录适配器
│   │   ├── tenor/              # Tenor API 适配器（预留）
│   │   └── giphy/              # Giphy API 适配器（预留）
│   │
│   ├── storage/                # 存储抽象
│   │   ├── interface.go
│   │   ├── minio.go
│   │   └── s3.go
│   │
├── data/                       # 本地数据目录（.gitignore）
│   ├── memes.db                # SQLite 数据库文件
│   └── cache/                  # 可选缓存
│
├── configs/                    # 配置文件
│   ├── config.yaml             # 主配置
│   └── config.example.yaml     # 示例配置
│
├── proto/                      # 手写 protobuf API/structured-value schema (.proto 源)
├── gen/                        # buf 生成的 Go protobuf DTO 代码
│
├── deployments/                # 部署相关
│   ├── docker-compose.yml      # API + Alloy（Qdrant/存储外部）
│   └── Dockerfile
│
├── scripts/                    # 脚本
│   └── import-data.sh          # 本地静态图片目录导入脚本
│
├── go.mod
├── go.sum
└── docs/README.md
```

---

## 七、分阶段实施计划

### 第一阶段：MVP 基础搭建（第 1-2 周）

**目标**：跑通「本地静态图片目录导入 → 语义搜索」完整链路

**Week 1：基础设施 + 数据模型**

| 任务 | 产出 | 优先级 |
|------|------|--------|
| 搭建 Docker Compose 环境 | API + Alloy Compose（Qdrant/存储外部） | P0 |
| 初始化 Go 项目结构 | 按目录规划创建骨架 | P0 |
| 实现 SQLite 数据模型 | memes、meme_annotations、meme_vectors 三张核心表 + GORM | P0 |
| 实现存储抽象层 | MinIO 上传/下载/URL 生成 | P0 |
| 配置管理 | Viper 读取 config.yaml + 环境变量 | P1 |

**Week 2：多模态 Embedding + VLM/OCR 辅助 + 搜索**

| 任务 | 产出 | 优先级 |
|------|------|--------|
| 实现数据源接口定义 | source/interface.go | P0 |
| 实现 LocalDir Adapter | 递归扫描本地静态图片目录 | P0 |
| 实现多模态 Embedding 服务 | 支持 image/text document mode 与 query embedding | P0 |
| 实现 VLM/OCR 辅助服务 | 生成 caption、OCR 和可展示分析结果 | P0 |
| 实现摄入流程 | 读取/存储图片 → image/caption 向量 → 写入 Qdrant | P0 |
| 实现文本搜索 API | POST /api/v1/search 接口 | P0 |
| 基础去重 | content_hash 精确去重 | P1 |

**MVP 里程碑验收**：
- [ ] 成功导入本地静态图片目录中的表情包（含 image/caption 向量）
- [ ] 输入「无语」返回相关表情包
- [ ] 输入「开心猫猫」返回相关表情包
- [ ] 文本搜索延迟 < 200ms

---

### 第二阶段：功能完善（第 3-4 周）

**目标**：完善核心功能，支持图搜图

**Week 3：图搜图 + 搜索增强**

| 任务 | 产出 | 优先级 |
|------|------|--------|
| 实现图片搜索 API | POST /api/v1/search/image（图搜图） | P0 |
| VLM/OCR 分析复用 | 按 meme_id + analyzer_model 复用 description/OCR/labels | P0 |
| 分类浏览 API | GET /api/v1/categories, GET /api/v1/memes?category=xxx | P0 |
| 标签过滤搜索 | 支持 Qdrant Payload 过滤 + 向量搜索组合 | P1 |
| GIF 排除策略 | 数据源跳过 + 摄入校验拒绝 | P0 |
| 搜索结果分页 | 游标分页，支持无限滚动 | P1 |

## 八、关键技术决策记录

### 8.1 为什么选择 Qdrant 而不是 Milvus/Pinecone？

| 对比项 | Qdrant | Milvus | Pinecone |
|--------|--------|--------|----------|
| 部署复杂度 | 单二进制，极简 | 依赖多，复杂 | 托管服务 |
| Go SDK | 官方支持 | 社区维护 | 官方支持 |
| 资源占用 | 轻量 | 较重 | N/A |
| 成本 | 开源免费 | 开源免费 | $70+/月 |
| 适合阶段 | MVP → 中等规模 | 大规模 | 快速验证 |

**结论**：MVP 阶段 Qdrant 的简洁性和 Go 支持是最佳选择。

### 8.2 为什么当前改为多模态 Embedding 主链路？

| 对比项 | 多模态 Embedding 主链路 | VLM 描述 + Text Embedding |
|--------|------------------------|---------------------------|
| 图片语义保真 | 高，图片直接入向量空间 | 受描述文本质量影响 |
| 文搜图路径 | 文本 query 直接匹配图片向量 | query 匹配描述向量 |
| 导入成本 | 主要是 embedding | VLM 推理 + embedding |
| 文字梗/OCR | 需要辅助 caption/BM25 补强 | VLM 描述天然会包含一部分 |
| 可解释性 | 依赖 payload/辅助描述展示 | 描述文本更直观 |

**结论**：当前默认使用多模态 image embedding 作为主检索信号，避免把图片语义压缩成一段描述文本；VLM/OCR 继续作为 caption、BM25、展示和筛选属性的辅助来源。

### 8.3 为什么 MVP 用 SQLite 不用 PostgreSQL？

| 对比项 | SQLite | PostgreSQL |
|--------|--------|------------|
| 部署复杂度 | 零，单文件 | 需独立服务 |
| 并发支持 | 有限（写锁） | 优秀 |
| 运维成本 | 零 | 需要备份、监控 |
| 功能 | 基础 SQL | JSONB、数组、全文检索 |
| 迁移成本 | → PostgreSQL 简单 | - |

**结论**：MVP 阶段数据量小（5000 张），并发低，SQLite 足够。使用 GORM 可以无缝迁移到 PostgreSQL。

### 8.4 为什么摄入和搜索分离为两个服务？

- **资源隔离**：摄入是 CPU/IO 密集型（下载、哈希计算），搜索是低延迟要求
- **独立扩展**：搜索需要水平扩展应对流量，摄入按需运行
- **部署灵活**：可以在低峰期运行摄入任务

---
