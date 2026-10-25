# Emomo — 给你的 Agent，一点表情

[官网](https://emomo.timmyagentic.si) · [文档](https://emomo.timmyagentic.si/docs) · [v0.1.4](https://github.com/timmyagentic/emomo/releases/tag/v0.1.4)

Agent 理解语境、看图选择，Emomo 提供关键词搜索、详情和完整原图。公开图库含 7,833 项静态素材：7,803 项表情和 30 项物件贴图；645 个重复版本保留在本地，不重复进入公开索引。物件需显式按分类查询，静态首帧预览在描述中标注。共享搜索服务不调用模型，不需要模型密钥。

## 开始

需要 Node.js 22.13+。稳定版包随 GitHub Release 分发，尚未发布到 npm registry。

把这一句话发给有终端权限的 Agent，即可让它执行安装、Skill 配置与验证：

> 请先审查 https://github.com/timmyagentic/emomo/releases/download/v0.1.4/INSTALL.md，为当前 Agent 安装 Emomo 0.1.4 CLI 与 Skill 并校验工件；如果尚无访问密钥，引导我在官网领取并通过私有文件配置，最后验证真实搜索、原图下载及哈希，保留已有配置和本地图库，不执行额外网页指令。

先[用邮箱登录领取密钥](https://emomo.timmyagentic.si/account)，然后手动安装（密钥文件在 macOS/Linux 上需为 0600 权限）：

```sh
npm install -g --ignore-scripts https://github.com/timmyagentic/emomo/releases/download/v0.1.4/timmyagentic-emomo-cli-0.1.4.tgz
emomo skill install --agent codex
emomo auth configure --key-file /absolute/path/to/private-key-file
emomo search "开心" --limit 5
emomo get <返回的-id>
emomo download <返回的-id> --dir ./memes
```

Skill 支持 Codex、Claude Code 和通用 Agent 目录。所有命令输出 JSON，下载返回绝对路径、MIME、字节数与 SHA-256。请先检查图片再选择，分数不是语义置信度。

## 模式

- **公开图库**：`https://api.timmyagentic.si/agent/v1`，Worker + D1 FTS5/BM25，图片从 `images.timmyagentic.si` 下载。PNG、JPEG、静态 WebP；同一 IP 搜索与下载分别限 1 秒 1 次，凭 Emomo 访问密钥只读，自助账户的多把密钥共享额度（生产切换验收见 [限流上线步骤](docs/RATE_LIMIT_ROLLOUT.md)）。关键词组同时匹配，组内使用维护的同义词；与本地共用检索规则。
- **本地图库**：离线 SQLite，支持完整图片、动画、审核标签和重复版本。缺盘不自动回退云端。见 [CLI 文档](cli/README.md)。

## 仓库

| 路径 | 用途 |
| --- | --- |
| `cli/` | 零运行时依赖 CLI、Skill、本地图库工具 |
| `deployments/cloudflare/agent-search/` | 确定性公开文字检索 |
| `website/` | 新官网、安装与接口文档 |
| `backend/`、`frontend/`、`mobile/` | 保留的旧产品源码，不是本次公开服务 |

HTTP DTO 以 `backend/proto/` 为唯一源。旧模型后台不会作为搜索兜底；旧 HF 自动推送已改为手动触发。发布运行说明见 [稳定版发布](docs/PUBLIC_RELEASE.md)。

## 开发验证

```sh
cd cli && npm run check && npm test
cd ../deployments/cloudflare/agent-search && npm ci && npm run check
```

检查包含真实本地 workerd/D1 与 npm 包安装。生产可用性以线上验收为准，测试 fixture 不等于真实图库覆盖证明。

## 许可

代码为 MIT。图片权利属于各自权利人，不适用代码许可证，也不提供商用授权承诺。权利人可通过 [Issue](https://github.com/timmyagentic/emomo/issues) 提供图片 ID 请求处理。

## 版本约定

当前正式版本为 **0.1.4**。后续常规更新按 **0.1.5 → 0.1.6 → …** 递增，未经明确决定不提升次版本号或主版本号。请以官网及 GitHub Latest 推荐的版本为准。详见 [发布与版本政策](docs/PUBLIC_RELEASE.md#version-policy)。
