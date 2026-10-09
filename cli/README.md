# Emomo Agent CLI

Agent 理解意图、改写检索词、看图选择；CLI 提供关键词检索、详情和完整原图获取。1.0.0-beta1 同时支持独立本地图库和既有远程 REST API。没有运行时 npm 依赖。需要 Node.js 22.13 或以上，本地模式使用内置 SQLite FTS5。

## 安装

把这一句话发给有终端权限的 Agent，即可让它执行安装、Skill 配置与验证：

> 请阅读 https://timmyagentic.si/install.md，直接为当前 Agent 安装并配置 Emomo，完成后验证搜索和图片下载。

手动安装：

```sh
npm install --global --ignore-scripts https://github.com/timmyagentic/emomo/releases/download/v1.0.0-beta1/timmyagentic-emomo-cli-1.0.0-beta1.tgz
emomo skill install --agent codex
```

预发布包随 [GitHub Release](https://github.com/timmyagentic/emomo/releases/tag/v1.0.0-beta1) 分发，当前未发布到 npm registry。CLI包不包含私有图库或模型凭证。skill随包分发，支持`--agent claude`、`--agent agents`或`--dir <skills-directory>`；已有不同内容不会覆盖。

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

选择顺序：`--catalog`；显式`--api-url`；`EMOMO_CATALOG`；`EMOMO_API_URL`；已保存catalog；默认远程API。`--catalog`与`--api-url`同时提供报错。默认远程地址仍是`https://api.timmyagentic.si/agent/v1`。本地图库安装不部署或恢复生产服务。

远程鉴权保留`EMOMO_API_TOKEN`，只发给API；图片域名保留`EMOMO_IMAGE_HOSTS`校验，不转发Token。HTTPS/显式本机HTTP、响应大小、重定向、停服/鉴权/限流与不自动重试行为保持。零模型保证适用于本地模式和新的共享服务，不能代替任意自定义API的保证。

## 输出与验证

stdout始终一个`schemaVersion:1` JSON envelope，`ok:true`退出0，`ok:false`退出1。错误无原始文件系统/凭据内容；新增`LOCAL_CATALOG_UNAVAILABLE`、`LOCAL_IMAGE_UNAVAILABLE`、`INVALID_CATALOG`、`IMAGE_INTEGRITY_ERROR`、`CATALOG_EXISTS`。

```sh
npm run check
npm test
```

测试含远程协议、真实npm打包安装、本地离线搜索/详情/GIF下载、缺盘不远程fallback、SHA损坏、路径穿越/符号链接、物件/动画/文字状态与不覆盖行为。定向样例通过不是独立准确率评测，也不是生产云搜索验证。


## 私有图库维护

同一安装包另提供 `emomo-library`。消费搜索命令和维护命令分开，维护工具只操作本机指定的目录，没有上传、删除、模型调用或发布命令。

```sh
emomo-library prepare --catalog /path/to/catalog --dir /path/to/new-snapshot --collection first-batch --revision v1
emomo-library validate /path/to/new-snapshot
emomo-library diff /path/to/v1 /path/to/v2
```

快照含完整原图/动画、records.json、词表、校验和及审核清单。已有目录包括空目录都拒绝覆盖；独立复制资产，不链接源图，精确重复资产只保存一次。collection 给本地 ID 提供作用域，不同批次须使用不同 collection，不能因编号相同误合并。contentRevision 由记录、词表及 collection 确定，相同内容再次制作快照保持相同版本摘要。

validate 核对记录结构、文件魔数、格式及每个资产 SHA-256，拒绝越界路径/符号链接；不重新解码或生成图片，不宣称重新确认了所有画面标签。diff 报告新增、字段变化、图像更换、移除计划、词表变化、需要的新资产及字节数。移除计划不会删除源图库或任何云端记录。外置盘需要挂载；快照仅是独立本地副本，未证明异地灾难恢复。

review.json 是可重新计算的便览，validate 输出才是当前验证结果。公开传播授权及远程兼容缺口按 ID 报告；GIF 和动画仍完整保留，但当前远程静态 API/下载器尚未支持它们。publicReleaseReady 固定为 false，此工具不能批准公开发布。

这是私有维护档案格式，未修改 canonical protobuf HTTP DTO。无网络请求，元数据不包含源文件绝对路径，图库不得放入 Git/npm 安装包。源规范记录未经授权不能清理；默认消费图库仍由 `emomo catalog use` 明确选择。


## 精修 JSONL 接入 Agent

```sh
emomo catalog import-reviewed /path/to/metadata.jsonl --dir /path/to/new-reviewed-catalog
emomo doctor --catalog /path/to/new-reviewed-catalog
emomo catalog use /path/to/new-reviewed-catalog
emomo search "猫 生气" --limit 5
emomo get <returned-id>
emomo download <returned-id> --dir /path/to/new-downloads
```

精修导入检查 `id/canonical_id/disposition/decision/searchable/ocr_review/search_text/image_text/tags/path/sha256/width/height/frames`。只接收单帧记录；canonical 主图进入默认搜索，duplicate_alias 完整复制并可 get/download，excluded 不导入。`get` 的 `versions` 列出同组 ID、尺寸和哈希；`canonicalId` 指向主图。原始元数据与图片保持不变。每个 ID 在选定图库内稳定，不跨不同图库混用。

只有确认或明确归一化的主要文字进入索引。`partially_illegible` 的文字不进入索引也不作为可靠 imageText 返回，仍保留视觉标签和 ocrReview。无主要文字不证明整图无水印，因此 `--text without` 仍保守返回 text_absence_unverified。

精修库使用多个关键词条件同时匹配及常用同义词，返回 match.terms。Agent 负责把对话改写成少量检索词并看图判断语气；检索分数不是意图置信度。负面/反讽内容可能命中同主题，不能直接把第一名当作适合发送的表情。旧版目录继续使用其既有词表；缺口只由该库显式词表声明。
