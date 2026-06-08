import { del, get, post, put } from './client';
import type {
  Agent,
  AgentCandidate,
  AgentRequest,
  AddCandidateAgentRequest,
  CreateDaemonMachineRequest,
  CreateDaemonMachineResponse,
  DaemonMachine,
  OpenSkillLocationRequest,
} from '@/types/agent';

export async function getAgents(): Promise<Agent[]> {
  const agents = await get<Agent[] | null>('/api/agents');
  return agents ?? [];
}

export async function createAgent(body: AgentRequest): Promise<Agent> {
  return post<Agent>('/api/agents', body);
}

export async function updateAgent(id: string, body: AgentRequest): Promise<Agent> {
  return put<Agent>(`/api/agents/${id}`, body);
}

export async function updateAgentAvatar(id: string, avatar: string): Promise<Agent> {
  return put<Agent>(`/api/agents/${id}/avatar`, { avatar });
}

export async function deleteAgent(id: string): Promise<void> {
  return del<void>(`/api/agents/${id}`);
}

export async function getDaemonMachines(): Promise<DaemonMachine[]> {
  const machines = await get<DaemonMachine[] | null>('/api/daemon/machines');
  return machines ?? [];
}

export async function createDaemonMachine(
  body: CreateDaemonMachineRequest,
): Promise<CreateDaemonMachineResponse> {
  return post<CreateDaemonMachineResponse>('/api/daemon/machines', body);
}

export async function deleteDaemonMachine(id: string): Promise<void> {
  return del<void>(`/api/daemon/machines/${id}`);
}

export async function getAgentCandidates(): Promise<AgentCandidate[]> {
  const candidates = await get<AgentCandidate[] | null>('/api/daemon/agent-candidates');
  return candidates ?? [];
}

export async function addAgentCandidate(
  id: string,
  body: AddCandidateAgentRequest,
): Promise<Agent> {
  return post<Agent>(`/api/daemon/agent-candidates/${id}/add`, body);
}

export interface MachineConnectResponse {
  command: string;
  api_key: string;
  daemon_npm_path: string;
  machine: DaemonMachine;
}

export async function getMachineConnectCommand(id: string): Promise<MachineConnectResponse> {
  return get<MachineConnectResponse>(`/api/daemon/machines/${id}/connect`);
}

export async function openSkillLocation(
  id: string,
  body: OpenSkillLocationRequest,
): Promise<void> {
  return post<void>(`/api/agents/${id}/skills/open-location`, body);
}

export async function startAgent(id: string): Promise<void> {
  return post<void>(`/api/agents/${id}/start`);
}

export async function stopAgent(id: string): Promise<void> {
  return post<void>(`/api/agents/${id}/stop`);
}

export async function restartAgent(id: string): Promise<void> {
  return post<void>(`/api/agents/${id}/restart`);
}

export async function updateAgentTags(id: string, tags: string): Promise<Agent> {
  return put<Agent>(`/api/agents/${id}/tags`, { tags });
}

export async function updateCustomSkills(id: string, customSkills: string): Promise<Agent> {
  return put<Agent>(`/api/agents/${id}/custom-skills`, { custom_skills: customSkills });
}
