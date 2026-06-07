import { create } from 'zustand';
import type {
  Agent,
  AgentCandidate,
  AgentRequest,
  AgentStatus,
  CreateDaemonMachineResponse,
  DaemonMachine,
} from '@/types/agent';
import * as agentApi from '@/api/agent';

interface AgentState {
  agents: Agent[];
  machines: DaemonMachine[];
  candidates: AgentCandidate[];
  loading: boolean;
  machineLoading: boolean;
  error: string | null;
  fetchAgents: () => Promise<void>;
  fetchDaemonMachines: () => Promise<void>;
  deleteDaemonMachine: (id: string) => Promise<void>;
  fetchAgentCandidates: () => Promise<void>;
  createDaemonMachine: (name: string) => Promise<CreateDaemonMachineResponse>;
  addAgentCandidate: (id: string, name: string, systemPrompt?: string) => Promise<Agent>;
  createAgent: (body: AgentRequest) => Promise<Agent>;
  updateAgent: (id: string, body: AgentRequest) => Promise<Agent>;
  openSkillLocation: (id: string, sourcePath: string) => Promise<void>;
  deleteAgent: (id: string) => Promise<void>;
  startAgent: (id: string) => Promise<void>;
  stopAgent: (id: string) => Promise<void>;
  restartAgent: (id: string) => Promise<void>;
  updateAgentStatus: (agentId: string, status: AgentStatus) => void;
}

function sortAgents(list: Agent[]): Agent[] {
  return [...list].sort((a, b) => {
    if (a.status !== b.status) return a.status === 'online' ? -1 : 1;
    if (a.type !== b.type) return a.type === 'system' ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
}

export const useAgentStore = create<AgentState>((set) => ({
  agents: [],
  machines: [],
  candidates: [],
  loading: false,
  machineLoading: false,
  error: null,

  fetchAgents: async () => {
    set({ loading: true, error: null });
    try {
      const agents = await agentApi.getAgents();
      set({ agents: sortAgents(agents) });
    } catch (err) {
      const message = err instanceof Error ? err.message : '查询 Agent 失败';
      set({ error: message });
      throw err;
    } finally {
      set({ loading: false });
    }
  },

  fetchDaemonMachines: async () => {
    set({ machineLoading: true, error: null });
    try {
      const machines = await agentApi.getDaemonMachines();
      set({ machines });
    } catch (err) {
      const message = err instanceof Error ? err.message : '查询电脑连接失败';
      set({ error: message });
      throw err;
    } finally {
      set({ machineLoading: false });
    }
  },

  createDaemonMachine: async (name) => {
    const result = await agentApi.createDaemonMachine({ name });
    set((state) => ({ machines: [result.machine, ...state.machines] }));
    return result;
  },

  deleteDaemonMachine: async (id) => {
    await agentApi.deleteDaemonMachine(id);
    set((state) => ({
      machines: state.machines.filter((machine) => machine.id !== id),
      candidates: state.candidates.filter((candidate) => candidate.machine_id !== id),
      agents: state.agents.filter((agent) => agent.machine_id !== id),
    }));
  },

  fetchAgentCandidates: async () => {
    set({ machineLoading: true, error: null });
    try {
      const candidates = await agentApi.getAgentCandidates();
      set({ candidates });
    } catch (err) {
      const message = err instanceof Error ? err.message : '查询候选 Agent 失败';
      set({ error: message });
      throw err;
    } finally {
      set({ machineLoading: false });
    }
  },

  addAgentCandidate: async (id, name, systemPrompt) => {
    const payload = systemPrompt ? { name, system_prompt: systemPrompt } : { name };
    const agent = await agentApi.addAgentCandidate(id, payload);
    set((state) => ({
      agents: sortAgents([...state.agents.filter((item) => item.id !== agent.id), agent]),
    }));
    return agent;
  },

  createAgent: async (body) => {
    const agent = await agentApi.createAgent(body);
    set((state) => ({ agents: sortAgents([...state.agents, agent]) }));
    return agent;
  },

  updateAgent: async (id, body) => {
    const agent = await agentApi.updateAgent(id, body);
    set((state) => ({
      agents: sortAgents(state.agents.map((item) => (
        item.id === id ? agent : item
      ))),
    }));
    return agent;
  },

  openSkillLocation: async (id, sourcePath) => {
    await agentApi.openSkillLocation(id, { source_path: sourcePath });
  },

  deleteAgent: async (id) => {
    await agentApi.deleteAgent(id);
    set((state) => ({
      agents: state.agents.filter((agent) => agent.id !== id),
    }));
  },

  startAgent: async (id) => {
    await agentApi.startAgent(id);
    const agents = await agentApi.getAgents();
    set({ agents: sortAgents(agents) });
  },

  stopAgent: async (id) => {
    await agentApi.stopAgent(id);
    const agents = await agentApi.getAgents();
    set({ agents: sortAgents(agents) });
  },

  restartAgent: async (id) => {
    await agentApi.restartAgent(id);
    const agents = await agentApi.getAgents();
    set({ agents: sortAgents(agents) });
  },

  updateAgentStatus: (agentId, status) => {
    set((state) => ({
      agents: sortAgents(
        state.agents.map((a): Agent => (a.id === agentId ? { ...a, status } : a)),
      ),
    }));
  },
}));
