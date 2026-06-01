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

export async function openSkillLocation(
  id: string,
  body: OpenSkillLocationRequest,
): Promise<void> {
  return post<void>(`/api/agents/${id}/skills/open-location`, body);
}
