import React, { Suspense, lazy, useEffect, useState } from 'react';
import { Modal, Spin } from 'antd';
import { FileExclamationOutlined } from '@ant-design/icons';
import { fileDiff, type DiffContent } from '@/api/files';
import { ApiError } from '@/api/client';
import { message as antMessage } from '@/utils/message';
import styles from './DiffViewer.module.css';

// DiffView（@codemirror/merge）懒加载
const DiffView = lazy(() => import('./DiffView'));

interface DiffViewerProps {
  open: boolean;
  onClose: () => void;
  /** agent ID（调 daemon RPC 查 git diff 用）。 */
  agentId: string;
  /** 项目根目录（来自 diff 卡片的 workDir）。 */
  workDir: string;
  /** 要对比的文件相对路径（相对 workDir）。 */
  filePath: string;
  title?: string;
  /** 预取的 diff 内容（来自 DiffCard 预取缓存）。有值时跳过 RPC，秒开。 */
  initialDiff?: DiffContent;
}

/**
 * 文件版本对比查看器——单文件 git diff。
 *
 * 数据源：优先用父组件预取的 initialDiff（秒开）；否则调 fileDiff（daemon action=diff）
 * 拿该文件的前后内容（默认工作区 vs HEAD），用 @codemirror/merge 的 MergeView 渲染双栏对比。
 *
 * 与旧版的区别：不再依赖 artifact 版本表，直接查 git，每个文件独立对比。
 */
export const DiffViewer: React.FC<DiffViewerProps> = ({ open, onClose, agentId, workDir, filePath, title, initialDiff }) => {
  const [loading, setLoading] = useState(false);
  const [oldDoc, setOldDoc] = useState('');
  const [newDoc, setNewDoc] = useState('');
  const [error, setError] = useState('');

  const language = inferLanguageFromPath(filePath);

  useEffect(() => {
    if (!open || !agentId || !workDir || !filePath) return;
    // 有预取缓存：直接用，跳过 RPC
    if (initialDiff) {
      setOldDoc(initialDiff.oldContent);
      setNewDoc(initialDiff.newContent);
      if (initialDiff.oldContent === initialDiff.newContent) {
        setError('该文件无改动（工作区与 HEAD 一致）');
      }
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError('');
    setOldDoc('');
    setNewDoc('');
    fileDiff(agentId, workDir, filePath)
      .then(({ oldContent, newContent }) => {
        if (cancelled) return;
        setOldDoc(oldContent);
        setNewDoc(newContent);
        // 前后内容完全相同（无改动）或都为空
        if (oldContent === newContent) {
          setError('该文件无改动（工作区与 HEAD 一致）');
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : '加载 diff 失败');
          if (err instanceof ApiError && err.status !== 503) {
            antMessage.error(err.message);
          }
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, agentId, workDir, filePath, initialDiff]);

  const renderContent = () => {
    if (loading) {
      return (
        <div className={styles.hintArea}>
          <Spin /> 加载文件对比...
        </div>
      );
    }
    if (error) {
      return <div className={styles.hintArea}>{error}</div>;
    }
    if (!oldDoc && !newDoc) {
      return <div className={styles.hintArea}>无内容</div>;
    }
    return (
      <div className={styles.diffWrapper}>
        <div className={styles.diffColHeader}>
          <span className={styles.colHeaderOld}>旧版本（HEAD）</span>
          <span className={styles.colHeaderNew}>新版本（工作区）</span>
        </div>
        <Suspense
          fallback={
            <div className={styles.hintArea}>
              <Spin /> 加载对比视图...
            </div>
          }
        >
          <div className={styles.diffPanel}>
            <DiffView oldDoc={oldDoc} newDoc={newDoc} language={language} />
          </div>
        </Suspense>
      </div>
    );
  };

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width="90vw"
      style={{ top: 24, maxWidth: 'none' }}
      title={
        <span>
          <FileExclamationOutlined /> {title || '文件对比'} · <code>{filePath}</code>
        </span>
      }
      destroyOnHidden
    >
      <div className={styles.viewerBody}>{renderContent()}</div>
    </Modal>
  );
};

/** 从文件路径推断代码语言。 */
function inferLanguageFromPath(filepath: string): string {
  const ext = filepath.split('.').pop()?.toLowerCase() || '';
  const map: Record<string, string> = {
    ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript', mjs: 'javascript',
    py: 'python', go: 'go', rs: 'rust', java: 'java', rb: 'ruby', kt: 'kotlin',
    html: 'html', css: 'css', scss: 'css', less: 'css', json: 'json', md: 'markdown',
    yml: 'yaml', yaml: 'yaml', toml: 'toml', sh: 'shell', bash: 'shell',
    sql: 'sql', xml: 'xml', vue: 'html', php: 'php', c: 'c', cpp: 'cpp', h: 'c',
  };
  return map[ext] || '';
}
