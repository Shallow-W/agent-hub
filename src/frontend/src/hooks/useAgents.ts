import { useEffect } from 'react';
import { useAgentStore } from '@/store/agentStore';

export function useAgents() {
  const agents = useAgentStore((s) => s.agents);
  const machines = useAgentStore((s) => s.machines);
  const candidates = useAgentStore((s) => s.candidates);
  const loading = useAgentStore((s) => s.loading);
  const machineLoading = useAgentStore((s) => s.machineLoading);
  const error = useAgentStore((s) => s.error);
  const fetchAgents = useAgentStore((s) => s.fetchAgents);
  const fetchDaemonMachines = useAgentStore((s) => s.fetchDaemonMachines);
  const deleteDaemonMachine = useAgentStore((s) => s.deleteDaemonMachine);
  const fetchAgentCandidates = useAgentStore((s) => s.fetchAgentCandidates);
  const createDaemonMachine = useAgentStore((s) => s.createDaemonMachine);
  const addAgentCandidate = useAgentStore((s) => s.addAgentCandidate);
  const createAgent = useAgentStore((s) => s.createAgent);
  const updateAgent = useAgentStore((s) => s.updateAgent);
  const deleteAgent = useAgentStore((s) => s.deleteAgent);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  return {
    agents,
    machines,
    candidates,
    loading,
    machineLoading,
    error,
    createDaemonMachine,
    deleteDaemonMachine,
    addAgentCandidate,
    create: createAgent,
    update: updateAgent,
    remove: deleteAgent,
    refresh: fetchAgents,
    refreshMachines: fetchDaemonMachines,
    refreshCandidates: fetchAgentCandidates,
  };
}
