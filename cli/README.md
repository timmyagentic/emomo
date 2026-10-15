# Emomo CLI

给 Agent 搜索真实表情、查看详情、获取完整图片。Agent 负责理解语气和看图选择；CLI 与服务端不调用模型。需要 Node.js 22.13+，无运行时 npm 依赖。

## 安装一次

先到[账户页](https://emomo.timmyagentic.si/account)登录并领取访问密钥，保存到私有文件（macOS/Linux 权限 0600）。

```sh
npm install -g --ignore-scripts https://github.com/timmyagentic/emomo/releases/download/v0.1.4/timmyagentic-emomo-cli-0.1.4.tgz
emomo skill install --agent codex
emomo auth configure --key-file /absolute/private-key-file
```

Skill 也支持 `--agent claude`、`--agent agents` 或 `--dir <skills-directory>`，拒绝覆盖不同的现有内容。包由 GitHub Release 分发，尚未发布到 npm registry。[完整安装与校验说明](https://emomo.timmyagentic.si/install.md)。

## 日常三步

```sh
emomo search "猫 生气" --limit 5
emomo get <返回的-id>
emomo download <返回的-id> --dir ./memes
```

关键词组需要同时命中，优先匹配原词；没有完整原词命中时，才使用维护的同义词，再按文字相关性排序。本地和官方服务使用同一实现；私有图库可以保留自己的词表。不同图库的数据与词表不同，结果可能不同。关键词匹配不判断讽刺、礼貌或场景，Agent 应查看真实图片；没有合适结果就说明缺口。

常用参数：`--limit`（默认8，1–100）、`--category`、`--text any|unknown|with|without`。物件贴图默认排除，使用 `--category object_sticker` 单独查。缺少 OCR 不证明图片无字；本地 `--text without` 返回 `text_absence_unverified`。

下载返回绝对路径、MIME、大小和 SHA-256，拒绝覆盖文件，完整保留原图。`integrity=verified` 表示与来源 SHA-256 一致；`verified_legacy_md5` 是旧数据兼容校验；`unverified_no_hash` 仅校验了下载字节自身。首帧预览不等于完整动画。

## 当前图库与诊断

```sh
emomo doctor
emomo --help
emomo --help --all
```

默认帮助只展示三步工作流、密钥配置和诊断。`doctor` 集中返回当前 `library`、密钥配置及是否用于本次请求、连通性、图库统计；本地还校验图片哈希。配置存在不单独证明密钥有效。搜索、详情、下载也返回 `library`，直接说明使用了本地还是线上。

新安装默认使用官方服务；已有本地选择保持不变。明确选择常用图库：

```sh
emomo catalog use /path/to/catalog
emomo catalog use https://api.timmyagentic.si/agent/v1
```

每次可用 `--catalog <目录>` 或 `--api-url <地址>` 覆盖，不能同时提供。兼容原环境变量，优先顺序为命令参数、`EMOMO_CATALOG`、`EMOMO_API_URL`、保存的选择、官方服务；`library.selectedBy` 说明实际来源。配置位于 `~/.config/emomo/`，支持 `EMOMO_CONFIG_DIR` / `XDG_CONFIG_HOME`。

本地模式离线工作，缺盘明确失败，不自动转向远程。现有目录只读；普通和精修目录都在内存中构建统一检索索引，无需重导入或修改原图。可选 `--subject`、`--intent`、`--media image|preview|animation`、`--include-objects`，这些筛选仅支持本地。完整 GIF 可本地读取，公开服务仍只提供静态 PNG/JPEG/WebP。

## 进阶与维护

- `emomo-library import <JSON或JSONL> --dir <新目录>` 是维护导入入口；快照、校验、差异比较见[维护说明](MAINTENANCE.md)。
- 旧 `stats`、`categories`、`auth status`、`catalog path`、`capabilities`、两种 `catalog import` 仍兼容，列在 `--help --all`；日常状态查询统一使用 `doctor`。
- `auth clear` 清除保存的密钥。`skill install/path` 用于安装排查。固定的 `--profile`、`--collection` 保留兼容，不需要常规传入。
- 远程凭据仅发送给绑定地址。环境变量 `EMOMO_API_TOKEN` 必须绑定 `EMOMO_API_TOKEN_URL` 或 `EMOMO_API_URL`；切换库不转移密钥。官方密钥也用于规范官方图片下载，带鉴权的重定向被拒绝。
- 遇到 429/503 遵守 `retryAfterSeconds`，不自动循环重试。[服务配额与访问说明](../docs/ACCESS_CONTROL.md)。

每条命令 stdout 输出一个 `schemaVersion:1` JSON；成功 `ok:true` / 退出0，失败 `ok:false` / 退出1。`--json` 可省略，`--version` 查询版本，`--timeout` 控制远程超时。

开发验证：`npm run check && npm test`。真实生产可用性需要另外验证线上搜索与原图下载。
