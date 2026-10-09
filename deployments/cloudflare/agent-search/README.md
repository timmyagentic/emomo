# Emomo 云端文字检索

独立 Worker + D1 FTS5，只读文字索引并返回现有 R2 图片地址。无 LLM/embedding、自动 OCR、查询改写、模型重排、旧后端代理、模型 SDK/密钥、AI binding 或外部 HTTP 调用。使用者的 Agent 用自己的会话生成关键词、看图选图，不把 Agent 凭证交给 Emomo。

这消除了新版搜索服务的模型调用费用；Workers、D1、R2、日志、域名及保留的其他服务仍可能收费。限流和缓存不是账户账单硬上限。任意自定义 CLI API、旧 Go 服务及另行运行旧摄入流水线不在零模型保证内。

## 1.0 正式服务

官网 https://timmyagentic.si，API https://api.timmyagentic.si/agent/v1，图片 https://images.timmyagentic.si。正式配置已启用，使用独立 D1。公开索引为 7,302 张审核主图，重复版本不导入，不确定 OCR 不索引。发布流程见 [PUBLIC_RELEASE](../../../docs/PUBLIC_RELEASE.md)。以下旧库导出记录仅作历史参考。

## 检索契约

- 复用已有 description、OCR、事实 tags，不把爬虫来源词作为图片语义标签。中文使用确定性单字/相邻双字索引，查询双字词；“无语”与单字“猫”可检索。英文按词归一化，无模型分词。
- 字面词 OR 查询，BM25 排序：OCR 权重 4、描述 1、标签 2。score 单调归一化，不能解释为语义概率；未知同义词不会自动匹配，Agent 可改写一次。
- 沿用 canonical protobuf HTTP DTO，gen/ 同步 frontend/gen/ 生成输出，唯一 schema 源是 backend/proto/。total 为返回候选数。OCR 用于检索，原候选 DTO 没有 OCR 字段，未扩展 HTTP schema。
- 仅 search、单图详情、categories、stats，profile 为 keyword，collection 留空或 keyword。旧 /api/v1 返回 410，整库列表、SSE、管理及浏览器 Origin 关闭。类别最多列出 100 项。
- 查询 1–160 字/最多 64 词、默认 8 / 最多 100 候选、请求体 8 KiB；SQL 参数绑定，FTS 运算符不能注入。每个 Cloudflare 边缘节点按 IP 约 30 次/60 秒，计数最终一致，不是全局硬上限；Worker 内部成功响应缓存 60 秒，搜索缓存键只用请求哈希；错误不缓存。缓存读前限流。
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

当前配置 AGENT_API_ENABLED=true，绑定 api.timmyagentic.si 与专用生产 D1。后续生产变更仍需明确授权。emomo.net 出售页及旧模型后台保持独立；旧 api-gateway 不能作为零模型服务或自动兜底。


## 首期静态快照导入（2026-10-08）

首期只做 PNG、JPEG、静态 WebP，所有 GIF（包括单帧 GIF）与动画排除；完整原件和私有快照仍保留。

```sh
npm run static:prepare -- /path/to/private-snapshot /path/to/new-static-bundle
npm run static:verify -- /path/to/static-bundle /path/to/new-receipts /path/to/query-cases.json
```

prepare 验证整个规范快照，从中生成 canonical meme/annotation JSONL、D1 SQL、对象清单和完整静态图片字节。重复别名排除，partially_illegible OCR 不导入。ID 为 collection 与本地 ID 的组合，object key 使用 collection 与 SHA-256。输出目录拒绝覆盖，图像不裁切或重编码。准备包仍是私有副本，publicReleaseReady=false，没有云端上传或发布命令。

内部导入输入支持 `text_presence`（1未知、2有字、3无字）及 `search_aliases`。显式文字状态不能与明确 labels 矛盾；缺 OCR 的新快照记录保留 unknown。审核别名/场景进入 FTS，不写入公共 Meme.tags；原描述/OCR、审核标签/主体继续复用。旧数据库导出没有这些附加字段时保持旧标签逻辑，未修改 HTTP proto。

默认搜索不混入 category=object_sticker。要搜索物件，CLI 显式传 `--category object_sticker`；stats 统计仍包括所有已导入静态记录。旧类别/旧记录继续正常检索。缓存键增加静态策略前缀，避免复用旧混合类别结果；尚未实现持续更新的服务端图库 revision。

verify 使用真实本地 workerd/D1，临时 HTTP 桥仅监听127.0.0.1，CLI显式选择该地址（不会改用户默认配置）。它阻断/计数 Worker 出站 HTTP，校验全体图片哈希、canonical 行与 SQL 的一致性，真实 npm 离线安装 → skill → CLI 搜索/选图/详情/下载、三种格式、拒绝覆盖、未知文字、GIF未导入和缺图错误。运行结束关闭服务和临时 D1；receipt 及下载保存于指定新目录。图像查看可使用保存的文件，未提供浏览器网页验收。

查询用例 JSON 是数组：`[{"userQuery":"用户原话","query":"Agent关键词","expectedIds":["first-batch-local-205"],"category":"usable"}]`。通过标准为前8条存在预期候选，不要求第一条总是最好；未知描述范围仍由Agent看图选择。实际私有样例留在外置盘，不进入Git。
