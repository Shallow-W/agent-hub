# 外部化附件和图片存储

## Goal

让聊天附件、图片缩略图和知识库文件的二进制内容落到可配置的服务器文件目录中，数据库只保存元信息与可迁移的相对路径。前端可以直接使用后端返回的访问 URL 展示图片、下载文件，并且后续更换服务器时只需要调整配置中的存储目录和公网基址。

## What I Already Know

- 用户提供的目标服务器为一台 2C2G 云主机，项目远端工作目录为 `/root/rep/agent-hub`。
- 当前聊天附件链路已经不是把文件二进制存入数据库：`UploadService` 写入 `upload.dir/originals` 和 `upload.dir/thumbnails`，`message_attachments` 表存 `file_path`、`thumbnail_path`、大小、MIME、宽高等元信息。
- 当前知识库文件链路同样写入 `upload.dir/knowledge/<kbID>`，数据库保存 `knowledge_files.file_path`、预览文本和元信息。
- 现有静态访问路径是鉴权路由 `/api/uploads/*filepath`，前端手动拼 `/api` + `file_path`，并通过 query token 给 `<img>/<a>` 使用。
- 当前返回模型没有统一的绝对 URL 字段，迁移到公网服务器时前端和外部访问者依赖相对路径，不够清晰。

## Assumptions

- MVP 使用服务器本地磁盘作为对象存储根目录，不引入 S3/MinIO；通过配置抽象为后续替换对象存储留余地。
- 聊天附件下载/预览继续需要登录 token，避免把私人聊天文件完全公开。
- 配置中的公网基址可以是 `http://111.228.35.61:8080`、域名或反代后的 HTTPS 地址；代码不能硬编码当前 IP。
- 数据库中继续保存历史相对路径，新增 URL 字段作为 API 计算字段，不需要迁移历史数据。

## Requirements

- 后端上传配置支持：
  - `upload.dir`：本地文件根目录，例如生产环境可设置为 `/root/agenthub-data/uploads`。
  - `upload.public_base_url`：可选公网基址，用于拼接绝对文件访问 URL；为空时返回相对 URL。
- 聊天上传接口 `/api/upload` 返回原有字段之外，还返回：
  - `url`：原文件访问 URL。
  - `thumbnail_url`：缩略图访问 URL（图片才有）。
- 历史消息接口、未读消息接口、WebSocket 推送中的 `attachments` 同样包含 `url` 和 `thumbnail_url`。
- 前端附件渲染优先使用后端返回的 URL 字段，兼容旧数据没有 URL 时按旧 `file_path` 兜底拼接。
- 知识库文件列表返回可选 `url` 字段，便于前端或 Agent 工具引用；文件内容接口仍做权限校验。
- 静态文件服务必须防路径穿越，保留 `nosniff` 和图片 inline / 其他文件 attachment 的行为。
- 配置示例和文档说明如何在新服务器上改 IP/域名而不改数据库。

## Acceptance Criteria

- [ ] `upload.public_base_url` 为空时，本地开发仍能用相对 `/api/uploads/...` 链接访问附件。
- [ ] `upload.public_base_url` 设置为 `http://111.228.35.61:8080` 时，上传响应和消息附件 JSON 中返回绝对 URL。
- [ ] 图片消息使用缩略图 URL 展示，点击使用原图 URL。
- [ ] PDF/PPT/普通文件下载和预览入口仍可工作。
- [ ] 知识库文件列表包含可访问 URL，删除知识库或文件仍删除对应物理文件。
- [ ] 后端 Go tests 通过；前端 TypeScript 编译通过或明确记录阻塞。

## Out of Scope

- 本任务不搭建 Nginx/HTTPS/CDN。
- 本任务不迁移历史物理文件到新服务器；只保证配置切换后新旧相对路径可继续解析。
- 本任务不实现 S3/MinIO 驱动，但会保留后续替换的配置边界。
- 本任务不把聊天附件改成完全公开免鉴权访问。

## Technical Notes

- 相关后端文件：`src/backend/internal/service/upload.go`、`src/backend/internal/model/attachment.go`、`src/backend/internal/repository/attachment.go`、`src/backend/cmd/server/main.go`、`src/backend/internal/service/knowledge.go`。
- 相关前端文件：`src/frontend/src/types/attachment.ts`、`src/frontend/src/components/chat/MessageAttachmentView.tsx`、`src/frontend/src/api/upload.ts`、`src/frontend/src/api/knowledge.ts`。
- 现有 DB 字段 `file_path` 当前值形如 `uploads/originals/<sha>.<ext>`；静态路由挂载在 `/api/uploads/*filepath`，因此 URL 拼接需要去掉或兼容 `uploads` 前缀。
