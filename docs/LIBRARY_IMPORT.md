# Emomo 现有图库导入审计

核对时间：2026-10-06（Asia/Shanghai）。范围为现有 R2 / Supabase 数据只读审计、备份、离线转换与全量本地 D1 演练。用户授权恢复 Supabase，随后明确要求完成后继续运行。新 D1 的生产创建、导入、Worker 上线、域名切换和发布均未执行。

## 结论

现有图库可以直接复用已有描述/OCR，建立不调用 LLM 和 embedding 的检索副本。数据库有 **12,895 张图**，其中 **12,830 张已有描述，文本可搜索覆盖 99.50%**。全部数据库图片 key 都能对应到 R2；无需搬迁图片或重算向量。

65 张图只有“有无文字”标记，缺少描述与 OCR，先保留详情和图片，不把它们当作可搜索图片。R2 多出 1 张没有数据库记录的图片，先保留并列入核对清单。新云端仅做确定性文字检索，使用者自己的 Agent 理解需求、改写关键词、看图选图；云端不自动补标。

## 真实数据盘点

| 数据源 | 实际数量/体积 | 本次处理 |
|---|---:|---|
| R2 emomo | 12,896 个 JPEG；1,015,073,367 字节 | 留在原桶；完成全量清单 |
| R2 emomo-v2 | 0 个对象 | 无需导入 |
| PostgreSQL memes | 12,895 行 | 全量导出，保留原 ID/key/图片信息 |
| PostgreSQL meme_annotations | 12,926 行 | 12,895 行关联现有图片；31 行孤立记录仅备份 |
| PostgreSQL meme_metadata | 12,932 行 | 出处/作者/爬虫词仅保留在源和私有备份 |
| PostgreSQL meme_vectors | 25,784 行 | 向量关联记录仅备份；不进入新索引 |
| 本地 D1 检索副本 | 12,895 行；42,528,768 字节 | 完成全量导入和实际查询验证 |

四张公共业务表为 PostgreSQL 17.6。原表的 image_info、tags、labels 是 JSON 文本，不能将 PostgreSQL dump 直接当作 D1 导入 SQL。

图片 key 全部形如 `<两位 MD5 前缀>/<32 位 MD5>.jpeg`，文件名前缀和 ETag 对应。不存在空文件、重复 ETag；最小 1,490 字节，最大 709,338 字节。35 张图片的公开 JPEG 头读取成功，7 张完整下载的大小和 MD5 与清单一致；全量逐字节校验尚未执行。最早/最近对象时间为 2026-05-04 至 2026-06-09。

12,895 个数据库 ID、storage key、content hash 都无重复；图片格式均为 JPEG，尺寸均为正数，JSON 可解析。所有数据库 key 都在 R2 中，缺失图片数为 0。

R2 孤立图片：`58/58b318c273710e5cf02a72ff16dbc03e.jpeg`。它缺少原数据库 ID 和标注，不能用文件名冒充 canonical ID。保留对象，待核对后补建明确的业务记录；本次没有删除任何对象。

## 文字、标签和历史数据质量

| 项目 | 数量 | 含义 |
|---|---:|---|
| 非空描述 | 12,830 | 已包含主体、情绪、动作、场景和网络梗，可直接索引 |
| 规范化后非空 OCR | 7,734 | 清除 None/无文字等哨兵和空白后统计 |
| 原有 has_text=true | 7,770 | 保留原分析器/人工标记 |
| 原有 has_text=false | 5,125 | 保留原分析器/人工标记 |
| 无描述且无 OCR | 65 | 31 张标记有文字、34 张标记无文字；不可文字检索 |
| 原标记与 OCR 推导不同 | 42 | 已修正导入器，明确标记优先，避免改变筛选结果 |
| 非空 tags/category | 0 / 0 | 类别列表为空是源数据现状；不编造类别 |

关联标注中：glm-4.6v 11,751 条、manual-chat-vlm-20260609 1,079 条、codex-local-text-presence-20260612 65 条。每张现存图恰有一条标注，没有“最新标注覆盖已有完整描述”的情况。最长描述/OCR 均为 286 字；输入完全满足现有字段上限。

这些描述已经生成，导入只是复制已有成果，不会重新触发 AI 费用。后续 65 张补标由外部人工或用户自己的 Agent 提供；源模型历史调用质量不等于已经逐图人工验收。

另有 31 条孤立 annotation、31 条孤立出处记录、48 条关联到已不存在图片的向量记录。完整保留在源和四表备份，不清理、不参与 canonical 图库索引。

PostgreSQL 中保存的是向量关系和 Qdrant point ID，不是向量本体。旧关系记录包括 image Qwen3-VL 12,919 条、caption Qwen3-VL 11,879 条、caption BM25 986 条。Qdrant 连接本次 TLS 失败，向量本体和恢复能力 UNVERIFIED；新文字检索不依赖它。

## 字段如何导入

| 来源字段 | 新索引/接口 | 处理方式 |
|---|---|---|
| memes.id | memes.id / canonical Meme.id | 原样保留，不重新编号 |
| storage_key | meme_json；运行时拼图片地址 | 保留相对 key，不固化签名 URL |
| content_hash | meme_json | 原样保留，便于离线核对 |
| image_info | meme_json | JSON 解析后沿用 protobuf 图片字段 |
| tags/category | meme_json / category / tag_terms | 沿用事实字段；目前全空 |
| annotation.description | description / description_terms | 无模型复制；中文单字和相邻双字，英文按词 |
| annotation.ocr_text | ocr_text / ocr_terms | 清理无文字哨兵；OCR 检索权重高于描述 |
| annotation.labels.has_text | text_presence | 有明确标记时优先；无 labels 才从 OCR 推导 |
| analyzer_model / timestamps | 私有原始快照和备份 | 用于审计，不扩展公共 HTTP DTO |
| 来源 URL、作者、抓取搜索词 | 原 PostgreSQL 和私有备份 | 不当作事实标签，不进入公共搜索索引 |
| 向量记录与 Qdrant 本体 | 原服务/私有备份 | 不迁移，不重算，不作为搜索兜底 |

导出 SQL 每图选 updated_at / created_at 最新的 annotation，再用 id 排序保证确定性。当前每图只有一条，未来需要重新审查标注质量，不能假定最新记录总是最好。

## 已完成的真实导入验证

在 repeatable-read 明确只读事务中导出 metadata.jsonl；`psql -q` 避免 BEGIN/SET/COMMIT 状态文本混入 JSONL。再次执行更新后的正式导出 SQL，12,895 行与保存快照逐字节相同。事务池不保证启动 PGOPTIONS 生效，因此导出脚本本身明确 BEGIN READ ONLY。

实际 `index:build` 将全部 12,895 行转为 SQLite upsert SQL，零拒绝。最长单条 SQL 5,194 字节。全量导入真实 Miniflare/workerd D1 后：

- 总记录 12,895，文字可搜索 12,830；有字/无字分别 7,770 / 5,125，与源标记一致。
- FTS external-content 完整性检查通过；关闭本地运行时后，以只读方式检查 SQLite 文件，integrity_check 返回 ok。
- 20 组查询全部返回有效 HTTP 200；19 组真实关键词有候选，预设不存在的关键词返回 0。
- CLI 对实际索引搜索、读取详情、下载真实 R2 JPEG 全部通过；下载 93,169 字节，文件 SHA-256 有记录。
- Worker 的所有外部 HTTP 请求都被阻断并计数，实际调用为 0；未运行旧摄入或任何模型。
- 聚焦回归先复现“有文字标记 + 空 OCR 被误判无文字”，修正后通过；npm run check 包含 12 个测试、类型/生成代码和 dry-run，均通过。

| 查询 | 字面匹配候选数 | API 实际返回数 |
|---|---:|---:|
| 无语 | 1,781 | 8 |
| 猫 | 2,995 | 8 |
| 狗 | 519 | 8 |
| 谢谢老板 | 82 | 8 |
| 下班 | 78 | 8 |
| 晚安 | 16 | 8 |
| 抱抱 | 22 | 8 |
| hello | 58 | 8 |
| good night | 6 | 6 |

这些是字面匹配和功能验收，不是语义准确率。当前多词 OR 查询会返回部分词匹配，比如“谢谢老板”包含“谢谢老婆”或“老板画饼”的候选。Agent 应据描述和图片筛选；需要时拆分/改写关键词。未知同义词、没有文本的视觉意图和 65 张空标注图无法保证召回。20 次本地查询约 9.5–34.4 毫秒，不能据此承诺生产延迟。

## 私有快照与重建材料

受管 worktree 的 gitignored 目录：

`data/agent-import-20261006/`

目录权限 0700，文件 0600。全量图像仍在 R2，本次没有下载整个桶。材料包括：

| 文件 | 大小 | 用途 |
|---|---:|---|
| r2-manifest.json | 2,453,196 B | 全量对象清单 |
| raw-metadata.jsonl | 13,488,739 B | 保留关联标注原始 JSON、分析器和时间 |
| metadata.jsonl | 9,762,471 B | canonical 离线导入输入 |
| index-data.sql | 27,646,271 B | 确定性 D1 upsert 数据 |
| legacy-business-tables.dump | 7,327,957 B | 四张业务表完整 schema+data，含孤立/出处/向量关系 |
| annotation-gaps.jsonl | 65 行 | 无描述/OCR 的后续补标清单 |
| local-d1-rehearsal.json | 验证记录 | 实际结果、完整性、CLI 下载与 0 外部调用证据 |
| audit-evidence.json | 聚合证据 | 数据统计、只读标记、文件 SHA-256 和边界检查 |
| local-d1-pT6XpH/ | 本地运行态 | 已完成全量导入的实际本地 D1 |

pg_restore 能读取四表 dump，COPY 行数与源四表完全相同。实际恢复到另一个 PostgreSQL 实例尚未执行，标记 UNVERIFIED；它不是包含 Auth/Storage/整个 Supabase 项目的全量平台备份，也不包含 R2 图像或 Qdrant 向量本体。

关键 SHA-256：

- metadata.jsonl：77e1a17164a769e20474c388de7c614d52b4d785aa045a01d8d367bff4d36a73
- index-data.sql：4fbff603b2f8923162ac176a8e2465840a792cccc70dcb65d881652341c47579
- legacy-business-tables.dump：67bdacffa7dc643e5ddc85bd77cef7783c3c3bf6139270b189a681fef6229c8f

不要把原始 JSONL、业务 dump 或出处内容加入 Git、公开 npm 包、日志或公共索引。它们已保留在本机私有目录。

## 后续生产导入步骤

下列步骤是可执行的方案，本次未创建或修改生产 D1。

1. 复核本快照的记录数、文件 hash、源是否变化，以及 65 张缺口和 1 张孤立对象仍在清单。保留 R2/源四表和备份。
2. 单独授权后新建独立 D1，填入真实 database_id。先应用 `migrations/0001_text_index.sql`，再导入 index-data.sql；禁止将 PostgreSQL dump 直接执行到 D1。
3. 保持旧 API 停服，验证新 D1 总数、有无字分布、FTS 内容一致性，重跑代表性关键词/过滤/详情/图片读取。验证生产读写计量、兼容日期和 DB 实际占用。
4. 部署独立 Worker，先保持 AGENT_API_ENABLED=false。检查 bundle/binding 无模型 SDK、AI binding、旧上游代理或模型凭据。禁止启动旧 Go InitDB/摄入脚本来“补数据”。
5. emomo.net 已是域名出售页，当前图片地址依赖 r2.emomo.net。先确定长期持有的 API/图片域名，再单独授权 DNS/Custom Domain 和 CLI 默认地址；源 key 可复用，图片不用重新上传。
6. 通过真实安装包验收搜索、详情、下载和旧 /api/v1 的 410 边界，再显式启用新 Agent API。故障时禁用新 API，保留索引和源数据；禁止退回旧模型后端。
7. GitHub 解归档、PR、npm 公开发布和正式安装分别决定、分别验证。

D1 支持 SQLite FTS5，当前导入单条 SQL 远小于官方 100,000 字节上限，42.5 MB 本地索引也远小于单个免费 D1 的 500 MB 上限；远程计费占用以生产实测为准。[D1 官方限制](https://developers.cloudflare.com/d1/platform/limits/)

包含 FTS 虚拟表的数据库不能依赖普通 D1 SQL export 作为唯一恢复方式。保留 canonical JSONL + schema migration + index-data.sql，恢复时重建副本；不要为了导出而删除正在使用的 FTS 表。[D1 导入导出说明](https://developers.cloudflare.com/d1/best-practices/import-export-data/)

新版搜索不调用模型，但 Workers/D1 请求、索引触发器写入、R2 操作、日志、域名和保留资源仍可能收费。FTS 写入不等于只写 12,895 行；每 IP 限流和缓存也不是账户总额硬上限。这里没有承诺零总费用。[D1 计价说明](https://developers.cloudflare.com/d1/platform/pricing/)

## 当前运行状态和待处理项

Supabase 页面为 Healthy，按用户明确要求继续运行。旧 api.emomo.net 的 /api/v1 和 /agent/v1 实际检查均为 410 SERVICE_PAUSED；本次恢复数据库没有恢复旧应用 API。新生产 D1、Worker、域名切换和包发布均未执行。

另一个真实配置风险：四张业务表 RLS 均关闭，anon 角色拥有 SELECT/INSERT/UPDATE/DELETE 权限，Data API 开关仍开启。已向用户提出关闭 Data API 的具体确认；未获批准前不修改该配置。建议关闭 Data API，同时保留数据库和 SQL 访问。此配置不影响本地 D1 已验证结果。[Supabase API 安全说明](https://supabase.com/docs/guides/api/securing-your-api)

仍需生产验证：远程导入/计费占用、2026-10-06 生产兼容日期、跨网络图片访问和运行期账单。文本覆盖已实测，整体语义召回质量未做标注集评估。下一步应是单独授权的新 D1 导入与灰度验收，而不是重跑 AI 标注。
