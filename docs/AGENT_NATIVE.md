# Emomo Agent-native 入口

用户只安装 CLI 和 skill，向 Agent 说“找张想下班但还要开会的表情包”。Agent 使用自己的理解和视觉能力组织查询、选择候选，Emomo 复用已有云端图库进行检索并提供真实图片。

```text
用户 → Agent + emomo skill → emomo CLI → /agent/v1 网关
                                         ↓
                                私有 Go 搜索 API
                                         ↓
                              现有 PostgreSQL / Qdrant
Agent ← 选图 / 下载 PNG、JPEG、WebP ← 现有 R2 图片
```

网站、移动端不参与这个流程。无需迁移或重新嵌入现有图库，不把服务端 HF、模型、数据库或对象存储密钥交给 Agent。CLI 是无状态客户端；skill 定义查询、候选检查、选图和失败处理。

## 接口边界

Cloudflare 网关 `SERVICE_MODE=agent` 时：

| 公共入口 | 上游映射 |
|---|---|
| `POST /agent/v1/search` | `POST /api/v1/search` |
| `GET /agent/v1/memes/:id` | `GET /api/v1/memes/:id` |
| `GET /agent/v1/categories` | `GET /api/v1/categories` |
| `GET /agent/v1/stats` | `GET /api/v1/stats` |

旧 `/api/v1` 无论 Agent API 是否启用都保持 410；其他路径包括 `/health`、根管理页面、整库列表和 `/search/stream` 不对外转发。Agent 模式不允许浏览器跨域来源；CLI 请求无需 CORS。搜索复用原有限流和请求体上限，默认候选数 8，拒绝空白/过长查询及非法候选数量。

`AGENT_API_ENABLED` 默认 false，所以部署这份网关也不会自动恢复搜索。只有显式 `SERVICE_MODE=legacy` 才恢复旧路由；变量缺失或拼错也会保留 Agent 接口边界。旧路由兼容模式只用于明确恢复旧客户端的单独操作；此改造不需要它。

## 启用共享搜索前

以下是生产启用清单，不是已完成的生产验证。当前网站、API 与 HF 后端已暂停，仓库已归档，本地包还没有公开发布。

1. 核对现有 PostgreSQL、Qdrant 的数据和服务状态。确认搜索后端可正常读取它们，不运行重新摄入、迁移或数据清理。
2. 在单独授权的启用步骤恢复私有搜索后端，检查启动日志、现有集合和真实搜索。HF 仍保持 private。
3. Agent 已负责查询组织和候选判断，可考虑把服务端 `QUERY_EXPANSION_ENABLED=false`、`AGENTIC_SEARCH_ENABLED=false`，避免重复调用规划/重排模型；嵌入检索仍需服务端配置的 embedding 能力。用真实查询比较质量后决定，不宣称模型费用归零。
4. 部署已验证的 Agent 模式网关，保留现有 `HF_TOKEN` secret、限流 binding 与上游配置；使用 `--keep-vars` 等方式时注意它不能替代对最终两个模式变量的核对。生产最终应为 `SERVICE_MODE=agent`、`AGENT_API_ENABLED=true`。
5. 用真实安装包运行 `emomo doctor`、语义搜索及图片下载；确认图片链接能用。不得先关掉 CLI 所需的 R2 公共图片入口。若以后改为签名图片访问，需要先验证签名链接与下载行为。
6. 同时回归旧 `/api/v1/search`、`/api/v1/memes` 仍为 410；网站保持暂停或独立页面，不依赖旧业务 API。域名邮箱规则不变。
7. 确认 CLI+skill 的版本和安装包后再决定公开发布渠道。解除 GitHub 归档、创建/合并 PR、发布 npm 和生产启用都分别记录实际结果，不能用本地测试替代。

## 验收层次

- 本地 CLI / 安装包：真实安装、输出契约、元数据、下载、skill 安装可以在不访问生产的情况下验证。
- 网关边界：旧客户端暂停、新路径映射、访问范围、参数和限流用聚焦测试验证。
- 生产搜索：必须用现有真实图库实测。API 返回 410 时应报告未启用，不生成假结果。

此次只增加 Agent 入口，不删除前端/移动端源码和历史数据，不改邮箱或共享的其他 Cloudflare 资源。
