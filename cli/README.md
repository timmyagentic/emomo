# Emomo Agent CLI

Agent 理解意图、改写检索词、看图选择；CLI 提供关键词检索、详情和完整原图获取。0.2.0 同时支持独立本地图库和既有远程 REST API。没有运行时 npm 依赖。需要 Node.js 22.13 或以上，本地模式使用内置 SQLite FTS5。

## 安装

```sh
cd cli
npm pack
npm install --global --ignore-scripts ./timmyagentic-emomo-cli-0.2.0.tgz
emomo skill install --agent codex
```

本版本是本地构建包，没有发布到 npm。CLI包不包含私有图库或模型凭证。skill随包分发，支持`--agent claude`、`--agent agents`或`--dir <skills-directory>`；已有不同内容不会覆盖。

## 使用本地图库

选择已经生成的独立图库目录：

```sh
emomo catalog use /path/to/catalog
emomo doctor
emomo search '阴阳怪气地同意' --limit 5
emomo search '疑惑' --subject 猫
emomo search '猜拳' --media animation
emomo search '饺子' --include-objects
emomo get <返回的-id>
emomo download <返回的-id> --dir /path/to/new-selection
```

本机配置默认在`~/.config/emomo/config.json`，可用`EMOMO_CONFIG_DIR`或`XDG_CONFIG_HOME`指定位置。每次调用可用`--catalog <directory>`或`EMOMO_CATALOG`覆盖。目录包含manifest、SQLite索引及assets完整图片，可整体复制移动后重新`catalog use`，不依赖原始工作树、Python、源图片绝对路径、后台服务或监听端口。

图片默认完整保留，不裁切或拆分。GIF原始帧保留，`image.frames`和`mediaKind`说明动画；`previewOnly=true`说明静态首帧，不能当原动画。取图前校验SHA-256，复制完整字节，原件不改动、已有输出不覆盖。PNG/JPEG/WebP/GIF均可来自本地图库；远程图片保留既有静态格式边界。

本地`get`返回验证过的`meme.localPath`供Agent看图；`download`返回`data.path`、MIME、大小和SHA。搜索保留原候选字段`id/url/description/category/tags/score/textPresence/image`，本地追加`imageText/subjects/scenarios/mediaKind/previewOnly/quality/contentFlags/publicReleaseClearance/match`。`url`在本地是file URL；Agent应使用get返回的实际路径或download进行检查，不把file URL当公网上的可分享地址。

`--media image|preview|animation`、`--subject`、`--intent`仅支持本地模式。默认只搜usable；`--include-objects`加入object_sticker，或`--category object_sticker`单独搜物件。`--text with`只使用已转录文字，`--text unknown`为文字状态未确定；没有OCR不等于确认无文字，因此本地`--text without`返回`text_absence_unverified`而不误称无文字。模型profile不可用，只有keyword/local。

口语理解由调用者Agent承担。例如“有点懵”→“不明白”，“我裂开了”→“我整个头大”，“笑不活了”→“哈哈哈 大笑”。引擎提供有限词表和确定性匹配理由，不调用模型，不声称自动理解所有句子。当前图库的礼貌拒绝、道歉、生日祝福返回gallery_gap。零结果和语气不适合都应真实说明。公开发布权限未核实不等于完成版权审核。

## 从已审阅元数据生成本地图库

```sh
emomo catalog import /path/to/metadata.json --vocabulary /path/to/vocabulary.json --dir /path/to/new-catalog
```

此命令是显式管理操作，不自动选择新图库。元数据数组沿用本地整理字段`id/category/description/image_text/subjects/intent_tags/scenarios/query_aliases/content_flags/quality/media_kind/preview_only/width/height/frames/file_path/sha256/public_release_clearance`。`file_path`是源完整图片绝对路径，SHA须匹配。只接受usable/object_sticker；隔离和排除记录不导入。复制后的catalog只存相对asset路径、注释及匿名sourceId，不携带原始本机路径。vocabulary的`intents`、`subjects`映射名称到同义词数组，`gaps`映射明确缺口短语到说明；不包含模型调用。

导入不覆盖已有目录，采用临时目录和完整校验后发布。本地catalog读取验证索引摘要；图片路径不能穿越catalog或通过符号链接逃逸。缺盘、损坏、缺图均明确报错，绝不偷偷转向远程API。

## 远程API模式

```sh
emomo search '下班 开会' --api-url https://example.com/agent/v1
```

选择顺序：`--catalog`；显式`--api-url`；`EMOMO_CATALOG`；`EMOMO_API_URL`；已保存catalog；默认远程API。`--catalog`与`--api-url`同时提供报错。默认远程地址仍是`https://api.emomo.net/agent/v1`。本地图库安装不部署或恢复生产服务。

远程鉴权保留`EMOMO_API_TOKEN`，只发给API；图片域名保留`EMOMO_IMAGE_HOSTS`校验，不转发Token。HTTPS/显式本机HTTP、响应大小、重定向、停服/鉴权/限流与不自动重试行为保持。零模型保证适用于本地模式和新的共享服务，不能代替任意自定义API的保证。

## 输出与验证

stdout始终一个`schemaVersion:1` JSON envelope，`ok:true`退出0，`ok:false`退出1。错误无原始文件系统/凭据内容；新增`LOCAL_CATALOG_UNAVAILABLE`、`LOCAL_IMAGE_UNAVAILABLE`、`INVALID_CATALOG`、`IMAGE_INTEGRITY_ERROR`、`CATALOG_EXISTS`。

```sh
npm run check
npm test
```

测试含远程协议、真实npm打包安装、本地离线搜索/详情/GIF下载、缺盘不远程fallback、SHA损坏、路径穿越/符号链接、物件/动画/文字状态与不覆盖行为。定向样例通过不是独立准确率评测，也不是生产云搜索验证。
