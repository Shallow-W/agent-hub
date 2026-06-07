import { useEffect, useRef } from 'react';
import { useAgentStore } from '@/store/agentStore';

const STATUS_POLL_MS = 300_000;

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

  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    fetchAgents();
    fetchDaemonMachines();
    fetchAgentCandidates();

    const id = setInterval(() => {
      if (mountedRef.current) {
        fetchAgents().catch(() => {});
        fetchDaemonMachines().catch(() => {});
      }
    }, STATUS_POLL_MS);

    return () => {
      mountedRef.current = false;
      clearInterval(id);
    };
  }, [fetchAgents, fetchDaemonMachines, fetchAgentCandidates]);

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
