// 会话中 Agent 的角色（持久化在 conversation_agents.role 列）。
//
// 与 service/tool_config.go、daemon 中的 "orchestrator" 模板名不同，
// 此类型仅供 @mention 编排使用。
export type ConversationAgentRole = 'robot' | 'orchestrator' | 'worker' | 'observer';

// 显式常量，避免字面量散落全栈。
export const ROLE_ROBOT: ConversationAgentRole = 'robot';
export const ROLE_ORCHESTRATOR: ConversationAgentRole = 'orchestrator';
export const ROLE_WORKER: ConversationAgentRole = 'worker';
export const ROLE_OBSERVER: ConversationAgentRole = 'observer';

/** 可通过 API 赋值给 conversation agent 的角色集合。 */
export const ASSIGNABLE_ROLES: ReadonlyArray<ConversationAgentRole> = [
  ROLE_ORCHESTRATOR,
  ROLE_WORKER,
];

export function isAssignableRole(role: ConversationAgentRole | string | undefined | null): boolean {
  return role === ROLE_ORCHESTRATOR || role === ROLE_WORKER;
}
