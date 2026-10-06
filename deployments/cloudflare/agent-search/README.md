# Emomo 云端文字检索

独立 Worker + D1 FTS5，只读文字索引并返回现有 R2 图片地址。无 LLM/embedding、自动 OCR、查询改写、模型重排、旧后端代理、模型 SDK/密钥、AI binding 或外部 HTTP 调用。使用者的 Agent 用自己的会话生成关键词、看图选图，不把 Agent 凭证交给 Emomo。

这消除了新版搜索服务的模型调用费用；Workers、D1、R2、日志、域名及保留的其他服务仍可能收费。限流和缓存不是账户账单硬上限。任意自定义 CLI API、旧 Go 服务及另行运行旧摄入流水线不在零模型保证内。

## 检索契约

- 复用已有 description、OCR、事实 tags，不把爬虫来源词作为图片语义标签。中文使用确定性单字/相邻双字索引，查询双字词；“无语”与单字“猫”可检索。英文按词归一化，无模型分词。
- 字面词 OR 查询，BM25 排序：OCR 权重 4、描述 1、标签 2。score 单调归一化，不能解释为语义概率；未知同义词不会自动匹配，Agent 可改写一次。
- 沿用 canonical protobuf HTTP DTO，gen/ 同步 frontend/gen/ 生成输出，唯一 schema 源是 backend/proto/。total 为返回候选数。OCR 用于检索，原候选 DTO 没有 OCR 字段，未扩展 HTTP schema。
- 仅 search、单图详情、categories、stats，profile 为 keyword，collection 留空或 keyword。旧 /api/v1 返回 410，整库列表、SSE、管理及浏览器 Origin 关闭。类别最多列出 100 项。
- 查询 1–160 字/最多 64 词、默认 8 / 最多 100 候选、请求体 8 KiB；SQL 参数绑定，FTS 运算符不能注入。每 IP 30 次/60 秒；成功响应缓存 60 秒，搜索缓存键只用请求哈希；错误不缓存。缓存读前限流。
- 原图由 CLI 直接下载，Worker 不读写/代理图片。图片基地址由运维配置，key 逐段编码；离线输入拒绝公开/签名 URL。匿名接口缓存不区分用户，未引入调用者鉴权功能。

## 离线导入

这是可重建的 D1 检索副本，原 PostgreSQL/Qdrant/R2 保留，图片不搬迁。无需恢复 HF、重建向量或重新生成描述。新图片的文字标注必须由外部人工/使用者自己的 Agent 提供，云端不自动补标。

scripts/export-metadata.sql 在明确的 repeatable-read 只读事务中导出，每图选最近的 annotation，不包含来源私人信息、向量或连接凭据。2026-10-06 已对真实 PostgreSQL 执行并导出 12,895 行，全量本地 D1 导入、CLI 搜索/详情/真实图片下载通过；完整覆盖与缺口见 [图库导入审计](../../../docs/LIBRARY_IMPORT.md)。后续导入前仍需重新核对数据覆盖。通过现有安全连接，在仓库外保存 JSONL：

```sh
umask 077
psql -X -q -A -t -v ON_ERROR_STOP=1 -f scripts/export-metadata.sql > /tmp/emomo-metadata.jsonl
```

每行使用原 protobuf 字段；内部导入格式不是新 HTTP DTO：

```json
{"meme":{"id":"sample-id","storage_key":"sample.png","image_info":{"width":512,"height":512,"format":2},"tags":[],"category":""},"annotation":{"meme_id":"sample-id","description":"猫咪露出无语的表情","ocr_text":""}}
```

annotation 可缺省/null，text presence 此时为 unknown。已有 annotation.labels.has_text 决定有字/无字筛选，即使 OCR 为空也保留明确标记；没有 labels 才从 OCR 推导。None/无文字等 OCR 哨兵不进索引；词汇为空的图不会命中。以下步骤仅操作本机：

```sh
npm ci
npm run index:build -- /tmp/emomo-metadata.jsonl /tmp/emomo-index-data.sql
npx wrangler d1 migrations apply emomo-agent-index --local
npx wrangler d1 execute emomo-agent-index --local --file /tmp/emomo-index-data.sql
npm run dev -- --var AGENT_API_ENABLED:true
# 另一个终端
EMOMO_API_URL=http://127.0.0.1:8787/agent/v1 emomo search "无语 猫" --limit 8
```

输出 SQL 权限 0600，拒绝覆盖；失败删除本次部分输出。每行输入最多 128 KiB、SQL 每条最多 96 KiB，超限需离线检查后重新导出，不静默截断。Upsert 与触发器同步 FTS，不重复插入 ID。更新后缓存最多滞后 60 秒。

## 验证与上线

`npm run check` 校验生成代码、类型、真实本地 workerd/D1 测试和 dry-run。测试阻断/计数全部外部 HTTP，并执行实际 npm 包安装 → skill → CLI 搜索/详情/图片下载。它证明机制，不代表真实图库覆盖率与召回质量。本机 Node 24，Node 22 CI 已配置。

使用最新稳定 Miniflare v4 4.20260730.0 + Wrangler 4.116.0，避开 v5 alpha。本机运行/测试兼容日期 2026-07-30；生产配置为 2026-10-06，生产运行时仍需验证。最新官方类型/配置 schema 已对照，Env 由 Wrangler 生成。

默认 AGENT_API_ENABLED=false，无生产路由/preview URL，D1 为本机 UUID 占位。dry-run 不创建资源。生产需单独授权创建 D1、导入已有文字元数据、验证真实查询。emomo.net 当前是域名出售页，需先确定长期保留的 API/图片域名，再绑定或切换其唯一 Custom Domain 并启用。详见 [启用清单](../../../docs/AGENT_NATIVE.md)。旧 api-gateway 代理模型搜索，不能作为零模型服务或自动兜底。
