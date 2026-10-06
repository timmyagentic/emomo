# Emomo Agent CLI

用 CLI 和 skill 接入共享表情包图库。Agent 理解对话、组织关键词、查看候选并挑选图片；新版 Emomo 云端仅做文字检索和返回静态图片，不调用 LLM、embedding 或自动 OCR，不收集 Agent 的模型凭证。用户无需安装网站、移动 App，或配置模型/数据库管理凭据。

## 安装

需要 Node.js 22 或更高版本。CLI 没有运行时 npm 依赖，skill 随包分发。

从仓库检出安装：

```sh
npm install --global ./cli
emomo skill install --agent codex
```

或者安装已构建的本地包：

```sh
cd cli
npm pack
npm install --global ./timmyagentic-emomo-cli-0.1.1.tgz
emomo skill install --agent codex
```

这是可安装的本地包；还没有发布到 npm，不能把 `npm install -g @timmyagentic/emomo-cli` 当成已经可用的公开安装入口。

`--agent claude` 安装到 `~/.claude/skills/emomo`；`--agent agents` 安装到 `~/.agents/skills/emomo`。Codex 使用 `$CODEX_HOME/skills`，未设置时使用 `~/.codex/skills`。也可用 `--dir <skills-directory>` 指定位置。已有不同内容的 skill 不会被覆盖，相同版本重复安装不会改写。安装后，让 Agent 加载 `$emomo`，必要时开启新会话。

## 搜索和获取图片

```sh
emomo doctor
emomo search "敷衍 好的 崩溃" --limit 8
emomo search "无语 猫" --text without --limit 5
emomo get <搜索返回的-id>
emomo download <搜索返回的-id> --dir /tmp/emomo-selection
```

下载返回绝对路径、图片 MIME 类型、字节数和 SHA-256。Agent 可以检查并展示该文件。图片支持 PNG、JPEG、WebP，拒绝把 HTML 错误页保存成图片，也拒绝覆盖同名文件。

`emomo categories` 获取可用分类；`emomo stats` 获取图库数量和检索 profile；`emomo capabilities` 或 `emomo --help` 获取命令目录。共享服务仅提供 keyword profile，collection 留空或 keyword；其他向量 profile 会报错。类别/标签可能为空；词不在元数据里就可能零命中，Agent 可改写一次或看图选择，云端没有语义兜底。

## 配置与停服状态

默认地址：`https://api.emomo.net/agent/v1`。网关的 Agent 模式只开放搜索、单图详情、类别和统计；旧 `/api/v1` 网站/移动端接口继续返回 410。不提供整库列表、SSE 模型推理过程或后台管理入口。

**当前生产 API 仍处于停服状态。** 本代码和安装包不会自行恢复生产服务。新 `deployments/cloudflare/agent-search` 配置默认 `AGENT_API_ENABLED=false`，无生产路由，D1 为本机占位。正式启用需单独导入已有文字元数据、启用新服务并验证真实搜索，不恢复 HF/旧模型后端。仓库中的 `docs/AGENT_NATIVE.md` 记录完整启用清单。

本地运行新文字检索服务并导入已有元数据后：

```sh
EMOMO_API_URL=http://127.0.0.1:8787/agent/v1 emomo search "下班 开会" --limit 3
```

| 配置 | 含义 |
|---|---|
| `EMOMO_API_URL` / `--api-url` | API 基地址，HTTPS；显式本机开发实例允许 HTTP |
| `EMOMO_API_TOKEN` | 选用实例要求的调用令牌，仅发给 API，不写入磁盘或发送给图片域名 |
| `EMOMO_IMAGE_HOSTS` | 显式替换可信图片主机名单，逗号分隔；默认 `r2.emomo.net,*.r2.dev,*.r2.cloudflarestorage.com` |
| `--timeout` | 请求超时毫秒，默认 30000，可设 100–120000 |

API 不跟随重定向，避免把调用令牌转发给其他域名。图片重定向最多 3 次，逐次校验协议和可信主机。本地测试图片仅允许使用显式本机 API 的同源 HTTP 地址。JSON 响应上限 2 MiB、图片上限 25 MiB。

零模型调用保证属于新版 Emomo 共享服务；任意 `EMOMO_API_URL` 自定义实例由该实例决定。存储/请求/数据库和使用者自己的 Agent 仍有各自成本。新服务的 score 是归一化 BM25 相关性。

## Agent 输出契约

所有命令均在 stdout 输出 **一个 JSON 对象**，默认如此；`--json` 是兼容选项。成功退出码 0，失败退出码 1，不在 JSON 前后混入日志。

```json
{"schemaVersion":1,"ok":true,"command":"search","data":{"query":"想下班","expandedQuery":"","total":0,"results":[]}}
```

候选包含 `id`、`url`、`description`、`category`、`tags`、`score`、`textPresence` 和 `image`。它是 CLI 的投影格式，不是新后端 protobuf DTO；请求仍发送后端现有的 `query` / `top_k` / `text_presence` 等字段。

```json
{"schemaVersion":1,"ok":false,"error":{"code":"SERVICE_PAUSED","message":"Emomo is offline. Do not retry searches or restart infrastructure automatically.","retryable":false,"httpStatus":410}}
```

`SERVICE_PAUSED`、`UNAUTHORIZED`、`FORBIDDEN` 应直接说明不可用并停止。`RATE_LIMITED` 可能包含 `retryAfterSeconds`。CLI 不自动重试搜索。其他常见错误包括 `INVALID_ARGUMENT`、`INVALID_RESPONSE`、`TIMEOUT`、`NETWORK_ERROR`、`FILE_EXISTS`、`IMAGE_HOST_NOT_ALLOWED`。

## 验证

```sh
cd cli
npm run check
npm test
```

测试覆盖现有 protojson 协议、停服/鉴权/限流、图片下载和令牌边界，以及真实 `npm pack` → 隔离安装 → skill 安装 → 搜索/详情/下载。测试使用明确标记的本地协议样本，新服务另有真实本地 workerd/D1 的 CLI 验收；不证明真实图库覆盖率或生产已经恢复。
