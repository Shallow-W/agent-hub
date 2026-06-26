// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Agent } from '@/types/agent';

vi.mock('@/api/agent', () => ({
  getAgents: vi.fn(),
  updateAgentToolsConfig: vi.fn(),
}));

import * as agentApi from '@/api/agent';
import { resetAgentStore, useAgentStore } from '../agentStore';

function makeAgent(toolsConfig: string): Agent {
  return {
    id: 'agent-1',
    user_id: 'user-1',
    name: 'Codex',
    type: 'custom',
    cli_tool: 'codex',
    tools_config: toolsConfig,
    source: 'daemon',
    status: 'online',
    created_at: '2026-06-25T00:00:00Z',
    updated_at: '2026-06-25T00:00:00Z',
  };
}

describe('agentStore', () => {
  beforeEach(() => {
    resetAgentStore();
    vi.clearAllMocks();
  });

  it('keeps saved tools config when an older agent list request resolves later', async () => {
    const oldAgent = makeAgent('{"toolset":"none","allowed_tools":[]}');
    const updatedAgent = makeAgent('{"allowed_tools":["list_tasks"]}');
    let resolveOldFetch: (agents: Agent[]) => void = () => {};

    vi.mocked(agentApi.getAgents)
      .mockImplementationOnce(() => new Promise<Agent[]>((resolve) => {
        resolveOldFetch = resolve;
      }))
      .mockResolvedValueOnce([updatedAgent]);
    vi.mocked(agentApi.updateAgentToolsConfig).mockResolvedValue(updatedAgent);

    const initialFetch = useAgentStore.getState().fetchAgents(true);
    const saved = await useAgentStore.getState().updateAgentToolsConfig(
      'agent-1',
      '{"allowed_tools":["list_tasks"]}',
      false,
    );
    resolveOldFetch([oldAgent]);
    await initialFetch;

    expect(saved.tools_config).toBe(updatedAgent.tools_config);
    expect(useAgentStore.getState().agents[0]?.tools_config).toBe(updatedAgent.tools_config);
  });
});
