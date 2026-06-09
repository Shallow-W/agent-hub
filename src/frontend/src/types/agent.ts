export type AgentType = 'system' | 'custom';
export type AgentStatus = 'online' | 'offline' | 'busy' | 'error' | 'stopped';

export interface Agent {
  id: string;
  user_id?: string;
  name: string;
  type: AgentType;
  cli_tool: string;
  system_prompt?: string;
  tools_config?: string;
  avatar?: string;
  capabilities_json?: string;
  custom_skills?: string;
  tags?: string;
  source: string;
  status: AgentStatus;
  version?: string;
  machine_id?: string;
  machine_name?: string;
  enable_management_tools?: boolean;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

export interface PlatformSkill {
  id: string;
  user_id: string;
  name: string;
  category?: string;
  description?: string;
  trigger?: string;
  detail?: string;
  created_at: string;
  updated_at: string;
}

export interface PlatformSkillRequest {
  name: string;
  category?: string;
  description?: string;
  trigger?: string;
  detail?: string;
}

export interface AgentPromptTemplate {
  id: string;
  user_id: string;
  name: string;
  category?: string;
  description?: string;
  system_prompt?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentPromptTemplateRequest {
  name: string;
  category?: string;
  description?: string;
  system_prompt?: string;
}

export interface AgentRequest {
  name: string;
  cli_tool: string;
  system_prompt?: string;
  tools_config?: string;
  avatar?: string;
  capabilities_json?: string;
  enable_management_tools?: boolean;
}

export interface DaemonMachine {
  id: string;
  user_id: string;
  name: string;
  machine_id: string;
  status: 'pending' | 'connected' | 'offline';
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentCandidate {
  id: string;
  machine_id: string;
  machine_name: string;
  name: string;
  cli_tool: string;
  version?: string;
  capabilities_json?: string;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateDaemonMachineRequest {
  name: string;
}

export interface CreateDaemonMachineResponse {
  machine: DaemonMachine;
  api_key: string;
  daemon_source_path: string;
  daemon_npm_path: string;
}

export interface AddCandidateAgentRequest {
  name: string;
  cli_tool: string;
  system_prompt?: string;
  tools_config?: string;
  custom_skills?: string;
}

export interface OpenSkillLocationRequest {
  source_path: string;
}
