# Emomo — 给你的 Agent，一点表情

[官网](https://timmyagentic.si) · [文档](https://timmyagentic.si/docs) · [v1.0.0-beta1](https://github.com/timmyagentic/emomo/releases/tag/v1.0.0-beta1)

Agent 理解语境、看图选择，Emomo 提供关键词搜索、详情和完整原图。公开图库含 7,302 张审核主图；467 张重复版本不重复进入公开索引。共享搜索服务不调用模型，不需要模型密钥。

## 开始

需要 Node.js 22.13+。预发布包随 GitHub Release 分发，尚未发布到 npm registry。

把这一句话发给有终端权限的 Agent，即可让它执行安装、Skill 配置与验证：

> 请阅读 https://timmyagentic.si/install.md，直接为当前 Agent 安装并配置 Emomo，完成后验证搜索和图片下载。

手动安装：

```sh
npm install -g --ignore-scripts https://github.com/timmyagentic/emomo/releases/download/v1.0.0-beta1/timmyagentic-emomo-cli-1.0.0-beta1.tgz
emomo skill install --agent codex
emomo search "开心" --limit 5
emomo download <返回的-id> --dir ./memes
```

Skill 支持 Codex、Claude Code 和通用 Agent 目录。所有命令输出 JSON，下载返回绝对路径、MIME、字节数与 SHA-256。请先检查图片再选择，分数不是语义置信度。

## 模式

- **公开图库**：`https://api.timmyagentic.si/agent/v1`，Worker + D1 FTS5/BM25，图片从 `images.timmyagentic.si` 下载。PNG、JPEG、静态 WebP；每个边缘节点按 IP 约 30 次/分钟，匿名只读。多个关键词 OR 召回，不保证理解所有语义。
- **本地图库**：离线 SQLite，支持完整图片、动画、审核标签和重复版本。缺盘不自动回退云端。见 [CLI 文档](cli/README.md)。

## 仓库

| 路径 | 用途 |
| --- | --- |
| `cli/` | 零运行时依赖 CLI、Skill、本地图库工具 |
| `deployments/cloudflare/agent-search/` | 确定性公开文字检索 |
| `website/` | 新官网、安装与接口文档 |
| `backend/`、`frontend/`、`mobile/` | 保留的旧产品源码，不是本次公开服务 |

HTTP DTO 以 `backend/proto/` 为唯一源。旧模型后台不会作为搜索兜底；旧 HF 自动推送已改为手动触发。发布运行说明见 [Beta 预发布](docs/PUBLIC_RELEASE.md)。

## 开发验证

```sh
cd cli && npm run check && npm test
cd ../deployments/cloudflare/agent-search && npm ci && npm run check
```

检查包含真实本地 workerd/D1 与 npm 包安装。生产可用性以线上验收为准，测试 fixture 不等于真实图库覆盖证明。

## 许可

代码为 MIT。图片权利属于各自权利人，不适用代码许可证，也不提供商用授权承诺。权利人可通过 [Issue](https://github.com/timmyagentic/emomo/issues) 提供图片 ID 请求处理。
