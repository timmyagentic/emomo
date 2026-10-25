# 原生移动端退役记录

Emomo 原生 iOS / Android App 于 **2026-10-25** 正式退役，停止功能维护、依赖更新、CI 检查、EAS 构建和上架准备。现行产品为 [CLI + Skill、官网和独立 API](../README.md)。网页的手机宽度适配继续保留。

## 已移除的入口

- `mobile/`：Expo / React Native 源码、生成 DTO、依赖锁文件、原生配置、素材和商店元数据。
- `Mobile Artifacts` 手动打包工作流、`Mobile required checks` 作业及移动端 CI 校验脚本。
- 开发 `full` profile 中的移动端安装、生成和检查；移动端不再参与 CI 范围输出。剩余 CLI、Agent、Go、旧 Web 和网关门禁保持。
- 移动端实现计划、上架清单、当前产品文档中的运行和分发说明。旧 App 隐私声明仅作历史说明保留。

## 源码与本地改动

退役前完整源码位于提交 [`864bec427608391b7fcd3dac254ffbab3a8506be`](https://github.com/timmyagentic/emomo/tree/864bec427608391b7fcd3dac254ffbab3a8506be/mobile)，可在 Git 历史中查看或导出；无需在当前主干维持可构建副本。

本次在独立 worktree 操作，不清理、覆盖或提交源主检出的未提交改动和未跟踪文件。旧 App 的最近搜索保存在设备本地；退役不迁移或清除这些数据。

## 服务与分发边界

现行 CLI、Skill、官网、账户、搜索 API、图片下载和图库不依赖原生 App；本次不修改其运行代码、部署配置、协议或数据，也不发布新 CLI 版本。旧 App 依赖旧 `/api/v1`，不承诺继续可用；不得为其恢复旧模型后台或将密钥打包进客户端。

现行 CLI 需有终端权限的 Agent，不能替代手机 App 的系统分享与相册交互。

本记录确认仓库层面的正式退役。Apple / TestFlight、安卓渠道、EAS 的账号、历史构建、凭据、订阅和实际安装量未在本次核实（**UNVERIFIED**），不宣称已完成各渠道后台下架；本次不删除这些外部资源。
