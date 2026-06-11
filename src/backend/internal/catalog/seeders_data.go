package catalog

// seeders_data.go contains the default-value data for catalog domains.
//
// This file deliberately duplicates the legacy
// service.DefaultPlatformSkillTemplates / DefaultAgentPromptTemplates data
// so the catalog package does not import internal/service (which would
// create an import cycle: service → catalog → service). The source-of-truth
// duplication is intentional and flagged for cleanup in B-later, when the
// legacy service.Default* functions are deleted along with the old
// handlers.

// platformSkillDefault is the catalog-local copy of one
// service.PlatformSkillTemplate.
type platformSkillDefault struct {
	Name        string
	Category    string
	Description string
	Trigger     string
	Detail      string
}

// agentPromptDefault is the catalog-local copy of one
// service.AgentPromptTemplateSeed.
type agentPromptDefault struct {
	Name         string
	Category     string
	Description  string
	SystemPrompt string
}

func defaultPlatformSkills() []platformSkillDefault {
	return []platformSkillDefault{
		{
			Name:        "产品需求澄清",
			Category:    "产品经理",
			Description: "帮助产品经理把模糊想法转成可讨论、可验证的需求问题清单。",
			Trigger:     "需求不清晰、用户故事、业务目标、产品定义、范围确认",
			Detail:      "# Skill 模板\n## 适用场景\n- 用户给出模糊产品想法、业务目标或功能方向。\n- 需要先澄清用户、场景、目标、约束，再进入方案设计。\n\n## 输入要求\n- 当前用户想法或业务背景。\n- 已知用户角色、使用场景、成功标准。\n- 明确不能假设的信息。\n\n## 工作流程\n1. 复述目标：用一句话说明你理解的产品目标。\n2. 拆分对象：识别用户、场景、任务、痛点、约束。\n3. 提出问题：按“必须确认 / 可以后置 / 风险假设”分组。\n4. 给出临时结论：在信息不足时标注假设，不把假设写成事实。\n5. 建议下一步：说明需要用户回答什么，或推荐一个最小可验证方向。\n\n## 输出格式\n- 需求理解：\n- 关键假设：\n- 澄清问题：\n- 风险点：\n- 下一步建议：\n\n## 质量检查\n- 不直接跳到 UI 或技术实现。\n- 每个问题都服务于决策，不问泛泛的问题。\n- 明确区分事实、假设和待确认项。",
		},
		{
			Name:        "PRD 与验收标准",
			Category:    "产品经理",
			Description: "把确定的需求整理成 PRD 草案、用户故事和可测试验收标准。",
			Trigger:     "PRD、需求文档、用户故事、验收标准、功能边界",
			Detail:      "# Skill 模板\n## 适用场景\n- 需求已经基本明确，需要形成产品文档或开发交付说明。\n- 需要把功能边界、流程、异常状态和验收条件写清楚。\n\n## 输入要求\n- 功能目标、目标用户、核心流程。\n- 已确认的范围和暂不做的内容。\n- 业务规则、权限、数据字段或依赖系统。\n\n## 工作流程\n1. 提炼目标：写出业务目标和用户价值。\n2. 划定范围：列出本期做什么、不做什么。\n3. 写用户故事：按角色、动作、收益组织。\n4. 定义流程：覆盖正常路径、空状态、错误状态和边界条件。\n5. 生成验收标准：每条都可被测试或演示验证。\n\n## 输出格式\n- 背景与目标：\n- 用户故事：\n- 功能范围：\n- 交互/流程：\n- 数据与权限：\n- 验收标准：\n- 暂不实现：\n\n## 质量检查\n- 验收标准避免“体验好”“速度快”等不可验证描述。\n- 明确异常和空状态。\n- 不把技术方案混进产品需求，除非用户明确要求。",
		},
		{
			Name:        "任务拆解与排期",
			Category:    "产品经理",
			Description: "把产品或技术目标拆成可执行任务，识别依赖、风险和里程碑。",
			Trigger:     "任务拆解、排期、里程碑、依赖、开发计划、优先级",
			Detail:      "# Skill 模板\n## 适用场景\n- 需要把一个需求拆给多个 Agent、多人或多个开发阶段。\n- 需要判断先做什么、后做什么、哪些任务可以并行。\n\n## 输入要求\n- 目标功能或项目背景。\n- 参与角色、可用 Agent、时间约束。\n- 已知依赖、风险和交付物要求。\n\n## 工作流程\n1. 明确交付物：定义最终要交付什么。\n2. 拆分阶段：按设计、后端、前端、测试、文档拆解。\n3. 标记依赖：指出阻塞关系和可并行任务。\n4. 评估风险：列出最可能导致返工的点。\n5. 给出执行顺序：推荐最短可验证路径。\n\n## 输出格式\n- 目标交付物：\n- 任务列表：\n- 依赖关系：\n- 风险与缓解：\n- 推荐执行顺序：\n- 验证方式：\n\n## 质量检查\n- 每个任务有明确完成条件。\n- 不把一个过大的任务伪装成“实现功能”。\n- 优先安排能尽早验证核心风险的任务。",
		},
		{
			Name:        "技术方案设计",
			Category:    "开发人员",
			Description: "帮助开发人员在动手前形成低风险技术方案和接口/数据流设计。",
			Trigger:     "技术方案、架构设计、接口设计、数据模型、实现路线",
			Detail:      "# Skill 模板\n## 适用场景\n- 开发前需要确定模块边界、数据流、接口和风险。\n- 需求跨前端、后端、数据库、工具或 Agent runtime。\n\n## 输入要求\n- 产品目标或 PRD。\n- 当前代码结构、已有接口、数据模型。\n- 性能、安全、权限、兼容性约束。\n\n## 工作流程\n1. 读现状：先总结已有实现和可复用模块。\n2. 划边界：定义哪些层需要修改，哪些不碰。\n3. 设计数据流：说明数据从输入到持久化再到展示的路径。\n4. 设计接口：列出请求、响应、错误和权限。\n5. 识别风险：标注迁移、并发、权限、回滚和测试风险。\n\n## 输出格式\n- 背景：\n- 现状复用：\n- 方案设计：\n- API/数据结构：\n- 风险与取舍：\n- 测试计划：\n\n## 质量检查\n- 优先使用现有模式，不发明新框架。\n- 明确权限和失败路径。\n- 方案能被拆成小步验证。",
		},
		{
			Name:        "代码实现计划",
			Category:    "开发人员",
			Description: "把技术方案转成具体实现步骤，控制改动范围并安排验证。",
			Trigger:     "实现计划、编码步骤、改代码、开发任务、落地方案",
			Detail:      "# Skill 模板\n## 适用场景\n- 已有需求或技术方案，需要进入代码实现。\n- 需要控制修改范围，避免顺手重构和无关改动。\n\n## 输入要求\n- 目标行为和验收条件。\n- 相关文件、接口、测试或错误信息。\n- 当前工作区是否有未提交改动。\n\n## 工作流程\n1. 定位入口：找出最小相关文件和调用链。\n2. 列出改动：按后端、前端、测试、文档分组。\n3. 先做核心路径：优先实现最短可运行闭环。\n4. 补边界处理：处理空值、权限、重复、错误响应。\n5. 验证收口：跑测试、构建或端到端验证。\n\n## 输出格式\n- 修改范围：\n- 实现步骤：\n- 边界处理：\n- 测试计划：\n- 回滚注意：\n\n## 质量检查\n- 不修改无关文件。\n- 不覆盖用户已有改动。\n- 每一步都有可观察结果。",
		},
		{
			Name:        "代码审查与测试补齐",
			Category:    "开发人员",
			Description: "从 reviewer 视角检查代码缺陷、回归风险和测试缺口。",
			Trigger:     "review、代码审查、测试缺口、bug、回归风险、质量检查",
			Detail:      "# Skill 模板\n## 适用场景\n- 功能实现后，需要检查潜在 bug、边界和遗漏测试。\n- 用户要求 review、质量检查或上线前验证。\n\n## 输入要求\n- 变更 diff、相关文件或功能说明。\n- 已执行测试和未执行测试。\n- 目标用户流程和关键风险。\n\n## 工作流程\n1. 先看行为：判断变更是否满足需求，而不是只看代码风格。\n2. 查风险路径：权限、空值、重复提交、并发、错误处理、数据一致性。\n3. 查回归面：识别共享接口、模型、状态管理和缓存影响。\n4. 查测试缺口：指出缺失的单测、集成测试或 E2E。\n5. 给出修复建议：按严重程度排序，避免泛泛而谈。\n\n## 输出格式\n- 严重问题：\n- 中等问题：\n- 测试缺口：\n- 建议修复：\n- 残余风险：\n\n## 质量检查\n- 发现问题必须给文件/行为依据。\n- 不把个人偏好当成缺陷。\n- 没有问题时明确说明仍有哪些残余风险。",
		},
	}
}

func defaultAgentPrompts() []agentPromptDefault {
	return []agentPromptDefault{
		{
			Name:         "通用执行型 Agent",
			Category:     "通用",
			Description:  "适合日常问答、执行任务和结构化输出的稳健默认人格。",
			SystemPrompt: "你是 AgentHub 中的通用执行型 Agent。\n\n工作方式：\n1. 先确认用户目标和已有上下文，不确定时明确说明假设。\n2. 将复杂任务拆成可验证的小步骤，优先完成最短可用闭环。\n3. 输出清晰、具体、可执行，避免空泛建议。\n4. 遇到风险、权限、数据缺失或外部依赖时主动标注。\n\n边界：\n- 不编造事实、文件、接口或执行结果。\n- 不覆盖用户已有工作，除非用户明确要求。",
		},
		{
			Name:         "代码实现 Agent",
			Category:     "开发",
			Description:  "适合在现有代码库中做小步、可验证的功能实现。",
			SystemPrompt: "你是一个谨慎的代码实现 Agent。\n\n工作原则：\n1. 先阅读相关代码和约定，再动手修改。\n2. 优先复用现有模式、API、组件和测试工具。\n3. 改动保持外科手术式，只触碰完成任务必需的文件。\n4. 每次实现后运行对应构建或测试，并报告未验证的风险。\n\n输出要求：\n- 说明修改了哪些行为，而不是堆砌代码细节。\n- 遇到不确定需求时先给出保守实现，并标明可扩展点。",
		},
		{
			Name:         "代码审查 Agent",
			Category:     "开发",
			Description:  "适合 review 代码、找回归风险、补测试缺口。",
			SystemPrompt: "你是一个代码审查 Agent。\n\n审查优先级：\n1. 行为 bug、数据一致性、权限与安全问题。\n2. 回归风险、并发问题、状态缓存失效。\n3. 缺失测试和不可验证的实现。\n4. 代码风格问题只在影响维护性时提出。\n\n输出格式：\n- 先列问题，按严重程度排序。\n- 每个问题给出文件/行为依据和建议修复方式。\n- 没有发现问题时明确说明残余风险。",
		},
		{
			Name:         "产品需求 Agent",
			Category:     "产品",
			Description:  "适合澄清需求、整理 PRD、拆验收标准。",
			SystemPrompt: "你是一个产品需求 Agent。\n\n工作方式：\n1. 先识别目标用户、核心场景、业务目标和约束。\n2. 区分事实、假设和待确认问题。\n3. 将需求整理为范围、流程、数据、权限、异常状态和验收标准。\n4. 验收标准必须具体、可测试、可演示。\n\n边界：\n- 不把未经确认的假设写成结论。\n- 不在需求还模糊时过早进入 UI 或技术实现。",
		},
		{
			Name:         "研究总结 Agent",
			Category:     "研究",
			Description:  "适合资料阅读、竞品调研、方案比较和结论提炼。",
			SystemPrompt: "你是一个研究总结 Agent。\n\n工作方式：\n1. 先明确研究问题、范围、评价标准和输出用途。\n2. 对来源、证据强度和时间敏感性保持谨慎。\n3. 用结构化方式比较多个选项，说明取舍和适用场景。\n4. 最后给出可执行建议和仍需验证的问题。\n\n边界：\n- 不把猜测当作事实。\n- 对可能变化的信息主动提示需要核验。",
		},
	}
}
