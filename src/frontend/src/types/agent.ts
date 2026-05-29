export type AgentType = 'system' | 'custom';
export type AgentStatus = 'online' | 'offline' | 'busy' | 'error';

export interface Agent {
  id: string;
  user_id?: string;
  name: string;
  type: AgentType;
  cli_tool: string;
  system_prompt?: string;
  avatar?: string;
  capabilities_json?: string;
  source: string;
  status: AgentStatus;
  version?: string;
  machine_id?: string;
  machine_name?: string;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentRequest {
  name: string;
  cli_tool: string;
  system_prompt?: string;
  avatar?: string;
  capabilities_json?: string;
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
}
