import { create } from 'zustand';
import type { KnowledgeFile } from '@/types/knowledge';

/**
 * 全局 UI 选择状态，供多个路由视图共享。
 * 从 AppLayout 的 local state 提取，避免跨路由组件传递。
 */
interface UIState {
  selectedAgentId: string | null;
  selectedMachineId: string | null;
  selectedKnowledgeFile: KnowledgeFile | null;
  selectedKbId: string | null;

  setSelectedAgent: (id: string | null) => void;
  setSelectedMachine: (id: string | null) => void;
  setSelectedKnowledgeFile: (file: KnowledgeFile | null, kbId: string | null) => void;
  resetUIStore: () => void;
}

export const useUIStore = create<UIState>((set) => ({
  selectedAgentId: null,
  selectedMachineId: null,
  selectedKnowledgeFile: null,
  selectedKbId: null,

  setSelectedAgent: (id) => set({ selectedAgentId: id }),
  setSelectedMachine: (id) => set({ selectedMachineId: id }),
  setSelectedKnowledgeFile: (file, kbId) =>
    set({ selectedKnowledgeFile: file, selectedKbId: kbId }),
  resetUIStore: () =>
    set({
      selectedAgentId: null,
      selectedMachineId: null,
      selectedKnowledgeFile: null,
      selectedKbId: null,
    }),
}));
