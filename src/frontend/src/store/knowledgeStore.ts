import { create } from 'zustand';
import type { KnowledgeBase, KnowledgeFile } from '@/types/knowledge';
import * as kbApi from '@/api/knowledge';

interface KnowledgeState {
  knowledgeBases: KnowledgeBase[];
  loading: boolean;
  error: string | null;
  loaded: boolean;

  fetchKnowledgeBases: (force?: boolean) => Promise<void>;
  createKnowledgeBase: (name: string, description?: string) => Promise<KnowledgeBase>;
  deleteKnowledgeBase: (id: string) => Promise<void>;
  updateVisibility: (id: string, visibility: 'private' | 'public') => Promise<void>;
  addFile: (kbId: string, file: File) => Promise<void>;
  removeFile: (kbId: string, fileId: string) => Promise<void>;
  smartRenameFile: (kbId: string, fileId: string) => Promise<KnowledgeFile>;
}

export const useKnowledgeStore = create<KnowledgeState>((set, get) => ({
  knowledgeBases: [],
  loading: false,
  error: null,
  loaded: false,

  fetchKnowledgeBases: async (force) => {
    const state = useKnowledgeStore.getState();
    if (!force && state.loaded) return;
    set({ loading: true, error: null });
    try {
      const list = await kbApi.getKnowledgeBases();
      set({ knowledgeBases: list, loading: false, loaded: true });
    } catch (err) {
      set({ error: (err as Error).message, loading: false });
    }
  },

  createKnowledgeBase: async (name, description) => {
    const kb = await kbApi.createKnowledgeBase({ name, description });
    set((s) => ({ knowledgeBases: [...s.knowledgeBases, kb] }));
    return kb;
  },

  deleteKnowledgeBase: async (id) => {
    await kbApi.deleteKnowledgeBase(id);
    set((s) => ({ knowledgeBases: s.knowledgeBases.filter((kb) => kb.id !== id) }));
  },

  updateVisibility: async (id, visibility) => {
    await kbApi.updateKnowledgeBase(id, { visibility });
    set((s) => ({
      knowledgeBases: s.knowledgeBases.map((kb) =>
        kb.id === id ? { ...kb, visibility } : kb,
      ),
    }));
  },

  addFile: async (kbId, file) => {
    await kbApi.uploadKnowledgeFile(kbId, file);
    // 上传成功后重新拉取列表以更新文件信息
    await get().fetchKnowledgeBases(true);
  },

  removeFile: async (kbId, fileId) => {
    await kbApi.deleteKnowledgeFile(kbId, fileId);
    // 删除成功后重新拉取列表以同步远端数据
    await get().fetchKnowledgeBases(true);
  },

  smartRenameFile: async (kbId, fileId) => {
    const updatedFile = await kbApi.smartRenameKnowledgeFile(kbId, fileId);
    set((s) => ({
      knowledgeBases: s.knowledgeBases.map((kb) => (
        kb.id === kbId
          ? {
              ...kb,
              files: kb.files.map((file) => (
                file.id === fileId ? updatedFile : file
              )),
            }
          : kb
      )),
    }));
    return updatedFile;
  },
}));

export function resetKnowledgeStore(): void {
  useKnowledgeStore.setState({
    knowledgeBases: [],
    loading: false,
    error: null,
    loaded: false,
  });
}
