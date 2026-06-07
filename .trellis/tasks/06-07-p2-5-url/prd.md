# P2-5 部署发布：预览URL与源码下载

## 背景
需求文档 P2-5「部署发布」：聊天中发送"部署"指令，Agent 返回部署状态卡片；一键生成
预览 URL / 静态站点部署 / 容器化部署 / 源码打包下载。

经与产品确认，本次范围**只做真实可用的两项**：
- **预览 URL**（webpage 产物落盘生成可访问链接，等价于静态站点部署）
- **源码打包下载**（任意产物打 zip 下载）

容器化部署不做（需 Docker 基础设施，Demo 性价比低），状态卡片中不出现该选项。

## 范围（In Scope）
1. 部署一个已存在的 artifact（按 root_id），生成可公开访问的预览 URL。
2. 任意 artifact 打包成 zip 提供下载。
3. 聊天流内联「部署状态卡片」：状态徽标 + 可点 URL + 二维码 + 下载按钮。
4. 触发入口：产物卡片/工作区上的「部署」按钮。
5. （加分，时间允许）聊天里发"部署"文本关键词触发部署。

## 非范围（Out of Scope）
- 容器化部署、自定义域名、CDN、HTTPS 证书。
- 多文件站点打包（当前 artifact 为单 Content，webpage=单 HTML）。
- 部署版本回滚（沿用 artifact 版本血缘即可，不单独做）。

## 复用现有结构
- 产物模型 `model.Artifact`（webpage.Content = 整段 HTML）。
- 鉴权范式 `ArtifactService.checkAccess`（root_id → conversation → member）。
- 静态文件服务范式 `/api/uploads/*filepath`、`/api/ppt-preview/*filepath`（main.go）。
- 路由组 `/api/artifacts/:rootId/...`（main.go:316）。
- 消息卡片内联渲染 `ArtifactCard.tsx`。

## 数据模型
`deployments` 表（迁移 027）：
- id (uuid pk)
- artifact_root_id (text/uuid)  — 部署的产物血缘根
- conversation_id (text/uuid)   — 鉴权与卡片归属
- mode (text)                   — preview | download
- status (text)                 — pending | success | failed
- url (text, nullable)          — 预览访问地址 / 下载地址
- error (text, nullable)
- created_at (timestamptz)

## 接口
- `POST /api/artifacts/:rootId/deploy`  body `{ mode: "preview" }` → 创建部署，落盘，返回 Deployment（含 url）
- `GET  /api/deployments/:id`           → 查询部署状态
- `GET  /api/sites/:id/*filepath`       → 静态服务已部署站点（公开，照抄 uploads 范式）
- `GET  /api/deployments/:id/download`  → 返回产物 zip

## 验收标准
- 对一个 webpage 产物点「部署」，能在聊天流看到状态卡片，URL 可在新标签页打开看到网页。
- 二维码手机扫码可访问（同局域网）。
- 「下载」按钮能下载到包含产物源码的 zip。
- 非对话成员访问部署接口返回 403。
- 后端 `go test ./...` 通过；前端 lint/type-check 通过。
