# 维护者工具

这些操作用于整理自己的图库。普通搜图只需要 `emomo search/get/download`。维护工具只操作指定本地目录，不上传、不发布、不调用模型，也不删除源图库。

## 导入已审核资料

```sh
emomo-library import /path/to/metadata.json --vocabulary /path/to/vocabulary.json --dir /path/to/new-catalog
emomo-library import /path/to/reviewed.jsonl --dir /path/to/new-reviewed-catalog
```

入口自动区分 JSON 数组和精修 JSONL，转换为同一规范记录与图库格式。JSONL 自带词表，不再同时指定 `--vocabulary`。导入不改变默认选择，需要使用时再执行 `emomo catalog use <目录>`。

普通 JSON 字段沿用 `id/category/description/image_text/subjects/intent_tags/scenarios/query_aliases/content_flags/quality/media_kind/preview_only/width/height/frames/file_path/sha256/public_release_clearance`。`file_path` 是来源完整图片绝对路径，SHA 必须匹配；只导入 `usable` / `object_sticker`。词表 `intents`、`subjects` 为名称到同义词数组的映射，`gaps` 为明确缺口的说明。

精修 JSONL 检查 `id/canonical_id/disposition/decision/searchable/ocr_review/search_text/image_text/tags/path/sha256/width/height/frames`，只接收单帧记录。canonical 主图默认可搜，duplicate_alias 保留完整图片和 get/download 能力，excluded 不导入。`partially_illegible` 的不确定文字不进入索引；可靠视觉标签保留。

两种输入都保留完整图片、稳定 ID 和重复关系，最终资产仅使用相对路径；不将原始绝对路径写进便携图库。索引与图片验证哈希，拒绝路径越界、符号链接逃逸或损坏数据。输出目录必须不存在，包括空目录；失败不会覆盖既有图库。

旧 `emomo catalog import` / `import-reviewed` 继续作为兼容入口。旧图库无需重建，`searchMode: facets` 作为历史格式字段读取，搜索都使用共享关键词规则。不同图库的 ID 仍属于各自图库，不能跨库混用。

## 快照、校验与差异

```sh
emomo-library prepare --catalog /path/to/catalog --dir /path/to/new-snapshot --collection first-batch --revision v1
emomo-library validate /path/to/new-snapshot
emomo-library diff /path/to/v1 /path/to/v2
emomo-library help
```

快照包含完整原图/动画、records.json、词表、哈希与审核清单。相同资产只存一次；collection 区分不同批次的本地 ID，相同内容生成相同 contentRevision。校验检查结构、路径、文件格式与完整字节，不替代视觉审核。

`diff` 报告新增、字段变化、换图、移除计划、词表变化以及所需资产和体积；不会实施删除。`review.json` 是当时的便览，`validate` 是当前校验结果。`publicReleaseReady=false` 表示工具不能批准公开传播；GIF/动画的远程兼容缺口单独列出。

数据及快照留在 Git/npm 包之外。保留原始档案；同一块外置盘上的副本不构成异地备份。正式发布继续使用[发布流程](../docs/PUBLIC_RELEASE.md)与已有增量覆盖检查。
