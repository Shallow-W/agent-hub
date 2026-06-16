import React, { useEffect } from 'react';
import KnowledgePanel from '@/components/knowledge/KnowledgePanel';
import KnowledgeFilePreview from '@/components/knowledge/KnowledgeFilePreview';
import { useUIStore } from '@/store/uiStore';
import { useKnowledgeStore } from '@/store/knowledgeStore';
import styles from '@/layout/AppLayout.module.css';

const KnowledgeView: React.FC = () => {
  const selectedKnowledgeFile = useUIStore((s) => s.selectedKnowledgeFile);
  const selectedKbId = useUIStore((s) => s.selectedKbId);
  const setSelectedKnowledgeFile = useUIStore((s) => s.setSelectedKnowledgeFile);
  const knowledgeBases = useKnowledgeStore((s) => s.knowledgeBases);

  // Sync selected file when knowledge base data updates
  useEffect(() => {
    if (!selectedKnowledgeFile || !selectedKbId) return;
    const kb = knowledgeBases.find((k) => k.id === selectedKbId);
    if (!kb) return;
    const updated = kb.files?.find((f) => f.id === selectedKnowledgeFile.id);
    if (updated && updated !== selectedKnowledgeFile) {
      setSelectedKnowledgeFile(updated, selectedKbId);
    }
  }, [knowledgeBases, selectedKnowledgeFile, selectedKbId, setSelectedKnowledgeFile]);

  return (
    <>
      <KnowledgePanel
        onFileSelect={setSelectedKnowledgeFile}
        selectedFileId={selectedKnowledgeFile?.id ?? null}
        selectedKbId={selectedKbId}
      />
      {/* 右侧面板 */}
      {selectedKnowledgeFile && selectedKbId ? (
        <KnowledgeFilePreview file={selectedKnowledgeFile} kbId={selectedKbId} />
      ) : (
        <div className={styles.emptyRightPanel}>
          <div className={styles.emptyRightIcon}>📚</div>
          <div className={styles.emptyRightTitle}>知识库管理</div>
          <div className={styles.emptyRightDesc}>在左侧面板中管理你的知识库和文件</div>
        </div>
      )}
    </>
  );
};

export default KnowledgeView;
