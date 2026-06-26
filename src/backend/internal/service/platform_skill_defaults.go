// TODO(B5): delete this file — data is duplicated in catalog/seeders_data.go.
// ImportDefaults should be migrated to use catalog.Service.ImportDefaults directly.
package service

type PlatformSkillTemplate struct {
	Name        string
	Category    string
	Description string
	Trigger     string
	Detail      string
}

func DefaultPlatformSkillTemplates() []PlatformSkillTemplate {
	return []PlatformSkillTemplate{
		{
			Name:        "产品需求澄清",
			Category:    "产品经理",
			Description: "帮助产品经理把模糊想法转成可讨论、可验证的需求问题清单。",
			Trigger:     "需求不清晰、用户故事、业务目标、产品定义、范围确认",
			Detail: `# Skill 模板
## 适用场景
- 用户给出模糊产品想法、业务目标或功能方向。
- 需要先澄清用户、场景、目标、约束，再进入方案设计。

## 输入要求
- 当前用户想法或业务背景。
- 已知用户角色、使用场景、成功标准。
- 明确不能假设的信息。

## 工作流程
1. 复述目标：用一句话说明你理解的产品目标。
2. 拆分对象：识别用户、场景、任务、痛点、约束。
3. 提出问题：按“必须确认 / 可以后置 / 风险假设”分组。
4. 给出临时结论：在信息不足时标注假设，不把假设写成事实。
5. 建议下一步：说明需要用户回答什么，或推荐一个最小可验证方向。

## 输出格式
- 需求理解：
- 关键假设：
- 澄清问题：
- 风险点：
- 下一步建议：

## 质量检查
- 不直接跳到 UI 或技术实现。
- 每个问题都服务于决策，不问泛泛的问题。
- 明确区分事实、假设和待确认项。`,
		},
		{
			Name:        "PRD 与验收标准",
			Category:    "产品经理",
			Description: "把确定的需求整理成 PRD 草案、用户故事和可测试验收标准。",
			Trigger:     "PRD、需求文档、用户故事、验收标准、功能边界",
			Detail: `# Skill 模板
## 适用场景
- 需求已经基本明确，需要形成产品文档或开发交付说明。
- 需要把功能边界、流程、异常状态和验收条件写清楚。

## 输入要求
- 功能目标、目标用户、核心流程。
- 已确认的范围和暂不做的内容。
- 业务规则、权限、数据字段或依赖系统。

## 工作流程
1. 提炼目标：写出业务目标和用户价值。
2. 划定范围：列出本期做什么、不做什么。
3. 写用户故事：按角色、动作、收益组织。
4. 定义流程：覆盖正常路径、空状态、错误状态和边界条件。
5. 生成验收标准：每条都可被测试或演示验证。

## 输出格式
- 背景与目标：
- 用户故事：
- 功能范围：
- 交互/流程：
- 数据与权限：
- 验收标准：
- 暂不实现：

## 质量检查
- 验收标准避免“体验好”“速度快”等不可验证描述。
- 明确异常和空状态。
- 不把技术方案混进产品需求，除非用户明确要求。`,
		},
		{
			Name:        "任务拆解与排期",
			Category:    "产品经理",
			Description: "把产品或技术目标拆成可执行任务，识别依赖、风险和里程碑。",
			Trigger:     "任务拆解、排期、里程碑、依赖、开发计划、优先级",
			Detail: `# Skill 模板
## 适用场景
- 需要把一个需求拆给多个 Agent、多人或多个开发阶段。
- 需要判断先做什么、后做什么、哪些任务可以并行。

## 输入要求
- 目标功能或项目背景。
- 参与角色、可用 Agent、时间约束。
- 已知依赖、风险和交付物要求。

## 工作流程
1. 明确交付物：定义最终要交付什么。
2. 拆分阶段：按设计、后端、前端、测试、文档拆解。
3. 标记依赖：指出阻塞关系和可并行任务。
4. 评估风险：列出最可能导致返工的点。
5. 给出执行顺序：推荐最短可验证路径。

## 输出格式
- 目标交付物：
- 任务列表：
- 依赖关系：
- 风险与缓解：
- 推荐执行顺序：
- 验证方式：

## 质量检查
- 每个任务有明确完成条件。
- 不把一个过大的任务伪装成“实现功能”。
- 优先安排能尽早验证核心风险的任务。`,
		},
		{
			Name:        "技术方案设计",
			Category:    "开发人员",
			Description: "帮助开发人员在动手前形成低风险技术方案和接口/数据流设计。",
			Trigger:     "技术方案、架构设计、接口设计、数据模型、实现路线",
			Detail: `# Skill 模板
## 适用场景
- 开发前需要确定模块边界、数据流、接口和风险。
- 需求跨前端、后端、数据库、工具或 Agent runtime。

## 输入要求
- 产品目标或 PRD。
- 当前代码结构、已有接口、数据模型。
- 性能、安全、权限、兼容性约束。

## 工作流程
1. 读现状：先总结已有实现和可复用模块。
2. 划边界：定义哪些层需要修改，哪些不碰。
3. 设计数据流：说明数据从输入到持久化再到展示的路径。
4. 设计接口：列出请求、响应、错误和权限。
5. 识别风险：标注迁移、并发、权限、回滚和测试风险。

## 输出格式
- 背景：
- 现状复用：
- 方案设计：
- API/数据结构：
- 风险与取舍：
- 测试计划：

## 质量检查
- 优先使用现有模式，不发明新框架。
- 明确权限和失败路径。
- 方案能被拆成小步验证。`,
		},
		{
			Name:        "代码实现计划",
			Category:    "开发人员",
			Description: "把技术方案转成具体实现步骤，控制改动范围并安排验证。",
			Trigger:     "实现计划、编码步骤、改代码、开发任务、落地方案",
			Detail: `# Skill 模板
## 适用场景
- 已有需求或技术方案，需要进入代码实现。
- 需要控制修改范围，避免顺手重构和无关改动。

## 输入要求
- 目标行为和验收条件。
- 相关文件、接口、测试或错误信息。
- 当前工作区是否有未提交改动。

## 工作流程
1. 定位入口：找出最小相关文件和调用链。
2. 列出改动：按后端、前端、测试、文档分组。
3. 先做核心路径：优先实现最短可运行闭环。
4. 补边界处理：处理空值、权限、重复、错误响应。
5. 验证收口：跑测试、构建或端到端验证。

## 输出格式
- 修改范围：
- 实现步骤：
- 边界处理：
- 测试计划：
- 回滚注意：

## 质量检查
- 不修改无关文件。
- 不覆盖用户已有改动。
- 每一步都有可观察结果。`,
		},
		{
			Name:        "代码审查与测试补齐",
			Category:    "开发人员",
			Description: "从 reviewer 视角检查代码缺陷、回归风险和测试缺口。",
			Trigger:     "review、代码审查、测试缺口、bug、回归风险、质量检查",
			Detail: `# Skill 模板
## 适用场景
- 功能实现后，需要检查潜在 bug、边界和遗漏测试。
- 用户要求 review、质量检查或上线前验证。

## 输入要求
- 变更 diff、相关文件或功能说明。
- 已执行测试和未执行测试。
- 目标用户流程和关键风险。

## 工作流程
1. 先看行为：判断变更是否满足需求，而不是只看代码风格。
2. 查风险路径：权限、空值、重复提交、并发、错误处理、数据一致性。
3. 查回归面：识别共享接口、模型、状态管理和缓存影响。
4. 查测试缺口：指出缺失的单测、集成测试或 E2E。
5. 给出修复建议：按严重程度排序，避免泛泛而谈。

## 输出格式
- 严重问题：
- 中等问题：
- 测试缺口：
- 建议修复：
- 残余风险：

## 质量检查
- 发现问题必须给文件/行为依据。
- 不把个人偏好当成缺陷。
- 没有问题时明确说明仍有哪些残余风险。`,
		},
		{
			Name:        "应用部署指南",
			Category:    "工程实践",
			Description: "编写 Dockerfile 并调用 deploy_project 部署到公网：平台执行 docker build/run + 隧道。覆盖静态站点、React、Node 后端、Python 后端等场景的 Dockerfile 模板。",
			Trigger:     "部署、上线、发布、给别人访问、生成预览链接、生成 URL、deploy、dockerfile",
			Detail: "# 应用部署指南\n\n## 核心流程（agent 主导）\n部署分两步，职责清晰：\n1. **你（agent）**：在代码目录写好 `Dockerfile`（FROM + 业务构建 + EXPOSE <端口>）\n2. **平台**：调用 `deploy_project({ source_dir, port })` 后，平台执行 docker build/run + cloudflared 公网隧道，返回 URL（4 小时有效）\n\n**你必须写 Dockerfile**——平台不再自动生成。不写则部署直接报错。\n\n## 工具\n`deploy_project({ source_dir: \"代码目录绝对路径\", port: <容器端口, 默认80> })`\n- `source_dir` 必填，是代码所在目录（含 Dockerfile）\n- `port` 对应 Dockerfile 的 EXPOSE 端口，默认 80\n- 返回 `{ deployed, deploy_id, url, container, expires_at }`\n- URL 不会自动验证可访问性，拿到后可自行 curl 测试或告知用户\n\n`stop_deploy({ deploy_id })` —— 停止部署，deploy_id 来自 deploy_project 返回值\n\n## Dockerfile 模板（按场景选用）\n\n### 静态站点（纯 HTML）\n```dockerfile\nFROM nginx:alpine\nCOPY . /usr/share/nginx/html\nEXPOSE 80\n```\n\n### React / Vite / Vue（构建后 serve dist）\n```dockerfile\nFROM node:20-alpine AS build\nWORKDIR /app\nCOPY package*.json ./\nRUN npm ci\nCOPY . .\nRUN npm run build\n\nFROM nginx:alpine\nCOPY --from=build /app/dist /usr/share/nginx/html\nEXPOSE 80\n```\n注意：Vite 默认输出 `dist`，Next.js 输出 `.next` 或 `out`，按实际构建产物调整 COPY 源。\n\n### Node 后端（Express / Koa / Fastify）\n```dockerfile\nFROM node:20-alpine\nWORKDIR /app\nCOPY package*.json ./\nRUN npm ci --omit=dev\nCOPY . .\nEXPOSE 3000\nCMD [\"node\", \"server.js\"]\n```\n注意：EXPOSE 的端口要和 deploy_project 的 port 参数一致。后端服务通常监听 3000/8080，记得传 `port: 3000`。\n\n### Python 后端（FastAPI / Flask）\n```dockerfile\nFROM python:3.12-slim\nWORKDIR /app\nCOPY requirements.txt .\nRUN pip install --no-cache-dir -r requirements.txt\nCOPY . .\nEXPOSE 8000\nCMD [\"uvicorn\", \"main:app\", \"--host\", \"0.0.0.0\", \"--port\", \"8000\"]\n```\n\n### Go 后端（多阶段构建）\n```dockerfile\nFROM golang:1.22-alpine AS build\nWORKDIR /src\nCOPY go.* ./\nRUN go mod download\nCOPY . .\nRUN go build -o /app/server .\n\nFROM alpine:latest\nCOPY --from=build /app/server /app/server\nEXPOSE 8080\nCMD [\"/app/server\"]\n```\n\n## 常见坑\n- **构建上下文**：Dockerfile 里的 `COPY . .` 复制的是 source_dir 整个目录。加 `.dockerignore` 排除 node_modules、.git、dist 等，避免镜像过大。\n- **端口对应**：Dockerfile `EXPOSE <N>` 必须与 deploy_project 的 `port: <N>` 一致，否则访问不通。\n- **监听地址**：服务必须监听 `0.0.0.0` 而非 `127.0.0.1`，否则容器外访问不到。\n- **权限**：镜像内避免用 root 跑未知文件，必要时 `USER node`。\n\n## 工作流程\n1. 写好项目代码到某个目录\n2. 按上面的模板写 Dockerfile 放进该目录（EXPOSE 端口记下来）\n3. 调 `deploy_project({ source_dir, port })`\n4. 拿到 URL，自行 curl 测试或直接告知用户\n5. 任务完成后调 `render_card(card_type=\"project\", work_dir=<目录>)` 上报工作目录\n\n## 注意\n- URL 4 小时后自动失效\n- 本机必须装 Docker\n- 部署的是本机代码，不跨机器",
		},
	}
}
