import React, { useEffect, useRef, useState, useCallback } from 'react';
import {
  Button,
  Switch,
  Spin,
  Popconfirm,
  message as antMessage,
} from 'antd';
import {
  PlusOutlined,
  ReloadOutlined,
  DeleteOutlined,
  FileOutlined,
  UploadOutlined,
  DatabaseOutlined,
  LockOutlined,
  GlobalOutlined,
  CloseOutlined,
  InboxOutlined,
} from '@ant-design/icons';
import { useKnowledgeStore } from '@/store/knowledgeStore';
import type { KnowledgeFile } from '@/types/knowledge';
import { formatFileSize } from '@/types/attachment';
import styles from './KnowledgePanel.module.css';

/** 格式化文件大小 */
function formatBytes(bytes: number): string {
  return formatFileSize(bytes);
}

interface KnowledgePanelProps {
  onFileSelect?: (file: KnowledgeFile, kbId: string) => void;
  selectedFileId?: string | null;
  selectedKbId?: string | null;
}

const KnowledgePanel: React.FC<KnowledgePanelProps> = ({
  onFileSelect,
  selectedFileId,
  selectedKbId,
}) => {
  const {
    knowledgeBases,
    loading,
    fetchKnowledgeBases,
    createKnowledgeBase,
    deleteKnowledgeBase,
    updateVisibility,
    addFile,
    removeFile,
  } = useKnowledgeStore();

  useEffect(() => {
    fetchKnowledgeBases().catch(() => {});
  }, [fetchKnowledgeBases]);

  // 创建知识库弹窗状态
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newKbName, setNewKbName] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  const openCreateModal = useCallback(() => {
    setNewKbName('');
    setShowCreateModal(true);
    // 延迟聚焦输入框
    setTimeout(() => inputRef.current?.focus(), 50);
  }, []);

  const handleCreateConfirm = useCallback(() => {
    const name = newKbName.trim();
    if (!name) {
      antMessage.warning('请输入知识库名称');
      return;
    }
    setShowCreateModal(false);
    doCreate(name);
  }, [newKbName]);

  const handleCreateKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') {
        handleCreateConfirm();
      } else if (e.key === 'Escape') {
        setShowCreateModal(false);
      }
    },
    [handleCreateConfirm],
  );

  const doCreate = async (name: string) => {
    try {
      await createKnowledgeBase(name);
      antMessage.success('知识库创建成功');
    } catch {
      antMessage.error('创建失败');
    }
  };

  // 删除知识库
  const handleDelete = async (id: string) => {
    try {
      await deleteKnowledgeBase(id);
      antMessage.success('已删除');
    } catch {
      antMessage.error('删除失败');
    }
  };

  // 切换可见性
  const handleVisibilityChange = async (id: string, checked: boolean) => {
    const visibility = checked ? 'public' : 'private';
    try {
      await updateVisibility(id, visibility);
    } catch {
      antMessage.error('更新失败');
    }
  };

  // 上传文件
  const handleUpload = (kbId: string) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.multiple = true;
    input.onchange = async () => {
      if (!input.files) return;
      for (const file of Array.from(input.files)) {
        try {
          await addFile(kbId, file);
          antMessage.success(`${file.name} 上传成功`);
        } catch {
          antMessage.error(`${file.name} 上传失败`);
        }
      }
    };
    input.click();
  };

  // 删除文件
  const handleFileDelete = async (kbId: string, fileId: string) => {
    try {
      await removeFile(kbId, fileId);
      antMessage.success('文件已删除');
    } catch {
      antMessage.error('删除失败');
    }
  };

  if (loading && knowledgeBases.length === 0) {
    return (
      <>
        <div className={styles.header}>
          <span className={styles.headerTitle}>知识库</span>
          <div className={styles.headerTools}>
            <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={() => fetchKnowledgeBases()} />
            <Button type="text" icon={<PlusOutlined />} aria-label="新建" onClick={openCreateModal} />
          </div>
        </div>
        <div className={styles.loadingWrap}>
          <Spin />
        </div>
      </>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <span className={styles.headerTitle}>知识库</span>
        <div className={styles.headerTools}>
          <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={() => fetchKnowledgeBases()} />
          <Button type="text" icon={<PlusOutlined />} aria-label="新建" onClick={openCreateModal} />
        </div>
      </div>

      <div className={styles.scrollArea}>
        {knowledgeBases.length === 0 ? (
          <div className={styles.emptyState}>
            <InboxOutlined className={styles.emptyIcon} />
            <div className={styles.emptyTitle}>暂无知识库</div>
            <div className={styles.emptyDesc}>
              点击右上角 + 创建你的第一个知识库，<br />
              上传文件供 Agent 使用
            </div>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={openCreateModal}
              className={styles.createBtn}
            >
              新建知识库
            </Button>
          </div>
        ) : (
          <div className={styles.cardGrid}>
            {knowledgeBases.map((kb) => (
              <div className={styles.kbCard} key={kb.id}>
                {/* 卡片头部：名称 + 删除 */}
                <div className={styles.cardHeader}>
                  <div className={styles.cardTitle}>
                    <div className={styles.kbIcon}>
                      <DatabaseOutlined />
                    </div>
                    <span className={styles.kbName} title={kb.name}>{kb.name}</span>
                  </div>
                  <Popconfirm
                    title="确定删除该知识库？"
                    description="删除后文件将无法恢复"
                    onConfirm={() => handleDelete(kb.id)}
                    okText="删除"
                    cancelText="取消"
                    okButtonProps={{ danger: true }}
                  >
                    <button className={styles.deleteBtn} type="button" aria-label="删除知识库">
                      <DeleteOutlined />
                    </button>
                  </Popconfirm>
                </div>

                {/* 可见性开关 */}
                <div className={styles.visibilityRow}>
                  <span className={styles.visibilityLabel}>
                    {kb.visibility === 'public' ? (
                      <>
                        <GlobalOutlined className={styles.visibilityIcon} />
                        公开
                      </>
                    ) : (
                      <>
                        <LockOutlined className={styles.visibilityIcon} />
                        私有
                      </>
                    )}
                  </span>
                  <Switch
                    size="small"
                    checked={kb.visibility === 'public'}
                    checkedChildren="公开"
                    unCheckedChildren="私有"
                    onChange={(checked) => handleVisibilityChange(kb.id, checked)}
                  />
                </div>

                {/* 文件区域 */}
                <div className={styles.fileSection}>
                  <div className={styles.fileSectionHeader}>
                    <span className={styles.fileLabel}>
                      文件 <span className={styles.fileCount}>{kb.file_count}</span>
                    </span>
                    <button
                      className={styles.uploadBtn}
                      type="button"
                      aria-label="上传文件"
                      onClick={() => handleUpload(kb.id)}
                    >
                      <UploadOutlined />
                    </button>
                  </div>
                  {kb.files.length > 0 ? (
                    <div className={styles.fileList}>
                      {kb.files.map((file) => {
                        const isSelected = selectedFileId === file.id && selectedKbId === kb.id;
                        return (
                          <div
                            className={`${styles.fileItem} ${isSelected ? styles.fileItemSelected : ''}`}
                            key={file.id}
                            onClick={() => onFileSelect?.(file, kb.id)}
                            role="button"
                            tabIndex={0}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter' || e.key === ' ') {
                                e.preventDefault();
                                onFileSelect?.(file, kb.id);
                              }
                            }}
                          >
                            <FileOutlined className={styles.fileIcon} />
                            <span className={styles.fileName} title={file.filename}>{file.filename}</span>
                            <span className={styles.fileSize}>{formatBytes(file.size)}</span>
                            <button
                              className={styles.fileDeleteBtn}
                              type="button"
                              aria-label="删除文件"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleFileDelete(kb.id, file.id);
                              }}
                            >
                              <CloseOutlined />
                            </button>
                          </div>
                        );
                      })}
                    </div>
                  ) : (
                    <div className={styles.emptyFiles}>暂无文件，点击 ↑ 上传</div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 创建知识库弹窗 */}
      {showCreateModal && (
        <div className={styles.modalOverlay} onClick={() => setShowCreateModal(false)}>
          <div className={styles.modalContent} onClick={(e) => e.stopPropagation()}>
            <div className={styles.modalTitle}>新建知识库</div>
            <input
              ref={inputRef}
              className={styles.modalInput}
              type="text"
              placeholder="请输入知识库名称"
              maxLength={50}
              value={newKbName}
              onChange={(e) => setNewKbName(e.target.value)}
              onKeyDown={handleCreateKeyDown}
            />
            <div className={styles.modalActions}>
              <button
                className={styles.modalCancelBtn}
                type="button"
                onClick={() => setShowCreateModal(false)}
              >
                取消
              </button>
              <button
                className={styles.modalOkBtn}
                type="button"
                onClick={handleCreateConfirm}
              >
                创建
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default KnowledgePanel;
