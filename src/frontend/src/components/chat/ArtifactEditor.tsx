import React, { Suspense, lazy, useEffect, useRef, useState } from 'react';
import { Modal, Select, Input } from 'antd';
import {
  CheckOutlined,
  CopyOutlined,
  DiffOutlined,
  DownloadOutlined,
  EditOutlined,
  EyeOutlined,
  RobotOutlined,
  RollbackOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import { message as antMessage } from '@/utils/message';
import type { Artifact } from '@/types/message';
import { aiEditArtifact, createArtifactVersion, listArtifactVersions } from '@/api/artifact';
import { ApiError } from '@/api/client';
import { CodeBlock } from './CodeBlock';
import { WebpageFrame } from './WebpageFrame';
import { inferDownloadName } from './highlight';
import type { CodeSelectViewHandle } from './CodeSelectView';
import styles from './CodeBlock.module.css';

const CodeEditor = lazy(() => import('./CodeEditor'));
const CodeSelectView = lazy(() => import('./CodeSelectView'));
const DiffView = lazy(() => import('./DiffView'));
const { TextArea } = Input;

const SELECTION_PREVIEW_LIMIT = 240;

/**
 * 编辑器语言推断：code 用 artifact.language；webpage 固定 html；document 固定 markdown。
 * 决定 CodeMirror/DiffView 的语法高亮和编辑器模式。
 */
function resolveEditorLanguage(artifact: Artifact): string {
  if (artifact.type === 'webpage') return 'html';
  if (artifact.type === 'document') {
    // document 可能是 markdown 或纯文本——有 language 用 language，否则 markdown
    return artifact.language || 'markdown';
  }
  return artifact.language || '';
}

/** 显示标题：用于 Modal 标题。 */
function editorTitle(artifact: Artifact): string {
  return artifact.title || artifact.filename || (artifact.type === 'webpage' ? '网页产物' : artifact.type === 'document' ? '文档产物' : '代码产物');
}

/**
 * 统一产物编辑器——从 CodeBlock 展开态提炼的全功能工作台。
 * 所有文本类产物（code/webpage/document）共享：版本历史 + view/edit/diff 三模式 + AI 编辑。
 *
 * 设计要点：
 * - view 模式按 artifact.type 分发预览：code→CodeSelectView、webpage→WebpageFrame、document→CodeBlock(markdown 渲染)
 * - edit 模式统一用 CodeEditor（webpage 编辑 HTML、document 编辑 markdown）
 * - 版本/Diff/回滚/AI编辑逻辑与原 CodeBlock 一致，createVersion 的 type 跟随 artifact.type
 */
export interface ArtifactEditorProps {
  artifact: Artifact;
  open: boolean;
  onClose: () => void;
}

type EditorMode = 'view' | 'edit' | 'diff';

export const ArtifactEditor: React.FC<ArtifactEditorProps> = ({ artifact, open, onClose }) => {
  const rootId = artifact.root_id || artifact.id;
  const lang = resolveEditorLanguage(artifact);
  const baseContent = artifact.content ?? '';

  const [mode, setMode] = useState<EditorMode>('view');
  const [editedContent, setEditedContent] = useState(baseContent);
  const [selectedCode, setSelectedCode] = useState('');
  const [aiInstruction, setAiInstruction] = useState('');
  const [aiEditing, setAiEditing] = useState(false);
  const [copied, setCopied] = useState(false);

  const [versions, setVersions] = useState<Artifact[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const [diffOldVersion, setDiffOldVersion] = useState<number | null>(null);
  const [diffNewVersion, setDiffNewVersion] = useState<number | null>(null);

  const selectViewRef = useRef<CodeSelectViewHandle>(null);

  const selectedArtifact = versions.find((v) => v.version === selectedVersion) ?? null;
  const latestVersion = versions.length ? versions[versions.length - 1]!.version : null;
  // view/diff 基准内容：优先选中版本的 content，回退到传入 artifact 的 content
  const viewBase = selectedArtifact?.content ?? baseContent;
  const canDiff = Boolean(rootId && versions.length >= 2);

  const diffOldDoc = versions.find((v) => v.version === diffOldVersion)?.content ?? '';
  const diffNewDoc = versions.find((v) => v.version === diffNewVersion)?.content ?? '';
  const versionOptions = versions.map((v) => ({
    value: v.version,
    label: v.version === latestVersion ? `v${v.version}（最新）` : `v${v.version}`,
  }));

  const syncDiffDefaults = (list: Artifact[]) => {
    if (list.length >= 2) {
      setDiffOldVersion(list[list.length - 2]!.version);
      setDiffNewVersion(list[list.length - 1]!.version);
    }
  };

  const refreshVersions = async (created?: Artifact, fallbackContent = viewBase) => {
    if (!rootId) return;
    const list = await listArtifactVersions(rootId);
    setVersions(list);
    syncDiffDefaults(list);
    if (created) {
      setSelectedVersion(created.version);
      setEditedContent(created.content ?? fallbackContent);
      return;
    }
    const latest = list.length ? list[list.length - 1]!.version : null;
    setSelectedVersion(latest);
  };

  // 打开时重置状态 + 拉版本列表
  useEffect(() => {
    if (!open) return;
    setEditedContent(baseContent);
    setSelectedCode('');
    setAiInstruction('');
    setMode('view');
    if (!rootId) return;
    let cancelled = false;
    setVersionsLoading(true);
    listArtifactVersions(rootId)
      .then((list) => {
        if (cancelled) return;
        setVersions(list);
        setSelectedVersion(list.length ? list[list.length - 1]!.version : null);
        syncDiffDefaults(list);
      })
      .catch((err) => {
        if (!cancelled) antMessage.error(err instanceof ApiError ? err.message : '加载版本列表失败');
      })
      .finally(() => {
        if (!cancelled) setVersionsLoading(false);
      });
    return () => { cancelled = true; };
  }, [open, rootId, baseContent]);

  const handleSelectVersion = (version: number) => {
    setSelectedVersion(version);
    const target = versions.find((v) => v.version === version);
    setEditedContent(target?.content ?? viewBase);
    setSelectedCode('');
    setMode('view');
  };

  const createNewVersionFromContent = async (content: string): Promise<number> => {
    if (!rootId) throw new Error('no rootId');
    const created = await createArtifactVersion(rootId, {
      content,
      type: artifact.type,
      ...(lang ? { language: lang } : {}),
      ...(artifact.filename ? { filename: artifact.filename } : {}),
    });
    await refreshVersions(created, content);
    return created.version;
  };

  const handleSaveAsNewVersion = async () => {
    if (!rootId || saving) return;
    setSaving(true);
    try {
      const newVer = await createNewVersionFromContent(editedContent);
      antMessage.success(`已保存为 v${newVer}`);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : '保存新版本失败');
    } finally {
      setSaving(false);
    }
  };

  const handleRollback = async () => {
    if (!rootId || saving || !selectedArtifact) return;
    setSaving(true);
    try {
      const newVer = await createNewVersionFromContent(selectedArtifact.content ?? '');
      antMessage.success(`已回滚，生成 v${newVer}`);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : '回滚失败');
    } finally {
      setSaving(false);
    }
  };

  const handleApplyDiff = async () => {
    if (!rootId || saving) return;
    const target = versions.find((v) => v.version === diffNewVersion);
    if (!target) {
      antMessage.warning('请先选择要应用的新版本');
      return;
    }
    const nextContent = target.content ?? '';
    if (target.version === latestVersion) {
      setSelectedVersion(target.version);
      setEditedContent(nextContent);
      setMode('view');
      antMessage.success('已应用当前 Diff');
      return;
    }
    setSaving(true);
    try {
      const newVer = await createNewVersionFromContent(nextContent);
      setMode('view');
      antMessage.success(`已应用 Diff，生成 v${newVer}`);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : '应用 Diff 失败');
    } finally {
      setSaving(false);
    }
  };

  const handleAIEdit = async () => {
    if (!rootId || aiEditing) return;
    const instruction = aiInstruction.trim();
    if (!instruction) {
      antMessage.warning('先写一下你希望 AI 怎么改');
      return;
    }
    setAiEditing(true);
    try {
      const created = await aiEditArtifact(rootId, {
        instruction,
        ...(selectedCode.trim() ? { selection: selectedCode } : {}),
        // 基于用户选中的版本编辑（而非总是最新版）
        version: selectedVersion ?? undefined,
      });
      await refreshVersions(created, created.content ?? editedContent);
      setAiInstruction('');
      setSelectedCode('');
      setMode('view');
      antMessage.success(`AI 已生成 v${created.version}`);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : 'AI 修改失败');
    } finally {
      setAiEditing(false);
    }
  };

  const handleSelectAll = () => {
    if (selectViewRef.current) {
      selectViewRef.current.selectAll();
    } else {
      setSelectedCode(editedContent);
    }
  };

  const handleCopy = (value: string) => {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => antMessage.error('复制失败'));
  };

  const handleDownload = () => {
    const blob = new Blob([editedContent], { type: 'text/plain;charset=utf-8' });
    const objectUrl = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = objectUrl;
    anchor.download = inferDownloadName(artifact.filename, lang || undefined);
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
    URL.revokeObjectURL(objectUrl);
  };

  const selectionPreview = selectedCode.length > SELECTION_PREVIEW_LIMIT
    ? `${selectedCode.slice(0, SELECTION_PREVIEW_LIMIT)}…`
    : selectedCode;

  const renderAIEditPanel = (compact = false) => (
    <div className={`${styles.aiEditPanel} ${compact ? styles.aiEditPanelCompact : ''}`}>
      <div className={styles.aiEditMeta}>
        <span className={styles.aiEditTitle}><RobotOutlined /> AI 局部修改</span>
        <span className={styles.aiEditMetaRight}>
          <span className={styles.aiEditSelection}>
            {selectedCode.trim() ? `已选中 ${selectedCode.length} 个字符` : '未选中内容，将按整份修改'}
          </span>
          <button className={styles.aiEditSelectAll} type="button" disabled={aiEditing} onClick={handleSelectAll}>全选</button>
        </span>
      </div>
      {selectedCode.trim() && (
        <pre className={styles.aiEditPreview} title="当前选中内容">{selectionPreview}</pre>
      )}
      <div className={styles.aiEditControls}>
        <TextArea
          className={styles.aiEditInput}
          value={aiInstruction}
          onChange={(e) => setAiInstruction(e.target.value)}
          placeholder="描述你想怎么改"
          autoSize={{ minRows: 1, maxRows: 3 }}
          disabled={aiEditing}
        />
        <button
          className={`${styles.codeActionBtn} ${styles.expandedCopyBtn} ${styles.aiEditButton}`}
          type="button"
          disabled={aiEditing}
          onClick={handleAIEdit}
        >
          <span className={styles.codeCopyIcon}><RobotOutlined /></span>
          <span className={styles.codeCopyText}>{aiEditing ? '修改中...' : '让 AI 改'}</span>
        </button>
      </div>
    </div>
  );

  // view 模式：按 artifact.type 分发预览组件
  const renderViewContent = () => {
    if (artifact.type === 'webpage') {
      // webpage：iframe 预览（url 或 srcDoc）。
      // srcDoc iframe 是同源的，onMouseUp 时读 iframe contentWindow 的选区，接通 AI 局部修改。
      const doc = selectedArtifact?.content ?? baseContent;
      return (
        <div
          className={styles.expandedEditorWrapper}
          onMouseUp={() => {
            try {
              const iframe = document.querySelector('iframe');
              const sel = iframe?.contentWindow?.getSelection();
              const text = sel?.toString() ?? '';
              setSelectedCode(text.trim() ? text : '');
            } catch { /* 跨域 iframe 无法读选区，忽略 */ }
          }}
        >
          {rootId && renderAIEditPanel()}
          <WebpageFrame srcDoc={doc} />
        </div>
      );
    }
    if (artifact.type === 'document') {
      // document：复用 CodeBlock 的 markdown 渲染（CodeBlock 内联展示 + 展开）
      const doc = selectedArtifact?.content ?? baseContent;
      return (
        <div className={styles.expandedEditorWrapper}>
          {rootId && renderAIEditPanel()}
          <CodeBlock
            code={doc}
            language={lang}
            filename={artifact.filename}
            onSelectionChange={setSelectedCode}
          />
        </div>
      );
    }
    // code（默认）：可选中的代码视图
    return (
      <>
        {rootId && renderAIEditPanel()}
        <div className={styles.expandedEditorWrapper}>
          <Suspense fallback={<div className={styles.editorLoading}>加载代码视图...</div>}>
            <CodeSelectView
              ref={selectViewRef}
              value={editedContent}
              language={lang || undefined}
              onSelectionChange={setSelectedCode}
            />
          </Suspense>
        </div>
      </>
    );
  };

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width="94vw"
      style={{ top: 16, maxWidth: 'none' }}
      title={editorTitle(artifact)}
      className={styles.expandedCodeModal}
      destroyOnHidden
    >
      <div className={styles.expandedCodeView}>
        {/* 顶栏：版本/Diff 选择 + 模式切换 + 操作 */}
        <div className={styles.expandedCodeHeader}>
          <div className={styles.expandedHeaderLeft}>
            <span>{lang}</span>
            {mode === 'diff' ? (
              <>
                <span className={styles.diffLabel}>旧</span>
                <Select<number> size="small" className={styles.versionSelect} value={diffOldVersion ?? undefined} onChange={setDiffOldVersion} options={versionOptions} />
                <span className={styles.diffLabel}>新</span>
                <Select<number> size="small" className={styles.versionSelect} value={diffNewVersion ?? undefined} onChange={setDiffNewVersion} options={versionOptions} />
              </>
            ) : (
              rootId && versions.length > 0 && (
                <Select<number> size="small" className={styles.versionSelect} value={selectedVersion ?? undefined} loading={versionsLoading} onChange={handleSelectVersion} options={versionOptions} />
              )
            )}
          </div>
          <div className={styles.expandedHeaderActions}>
            {mode === 'diff' ? (
              <>
                <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn} ${styles.applyDiffBtn}`} type="button" disabled={saving || diffNewVersion === null} onClick={handleApplyDiff}>
                  <span className={styles.codeCopyIcon}><SaveOutlined /></span>
                  <span className={styles.codeCopyText}>{saving ? '应用中...' : '一键应用 Diff'}</span>
                </button>
                <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={() => setMode('view')}>
                  <span className={styles.codeCopyIcon}><EyeOutlined /></span>
                  <span className={styles.codeCopyText}>查看</span>
                </button>
              </>
            ) : (
              <>
                <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={() => setMode(mode === 'view' ? 'edit' : 'view')}>
                  <span className={styles.codeCopyIcon}>{mode === 'view' ? <EditOutlined /> : <EyeOutlined />}</span>
                  <span className={styles.codeCopyText}>{mode === 'view' ? '编辑' : '查看'}</span>
                </button>
                {canDiff && (
                  <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={() => setMode('diff')}>
                    <span className={styles.codeCopyIcon}><DiffOutlined /></span>
                    <span className={styles.codeCopyText}>Diff</span>
                  </button>
                )}
                {mode === 'view' && rootId && selectedVersion !== null && selectedVersion !== latestVersion && (
                  <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" disabled={saving} onClick={handleRollback}>
                    <span className={styles.codeCopyIcon}><RollbackOutlined /></span>
                    <span className={styles.codeCopyText}>{saving ? '回滚中...' : '回滚到此版本'}</span>
                  </button>
                )}
                <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn} ${copied ? styles.codeCopyBtnCopied : ''}`} type="button" onClick={() => handleCopy(editedContent)}>
                  <span className={styles.codeCopyIcon}>{copied ? <CheckOutlined /> : <CopyOutlined />}</span>
                  <span className={styles.codeCopyText}>{copied ? '已复制' : '复制'}</span>
                </button>
                <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={handleDownload}>
                  <span className={styles.codeCopyIcon}><DownloadOutlined /></span>
                  <span className={styles.codeCopyText}>下载</span>
                </button>
              </>
            )}
            {mode === 'edit' && (
              <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={() => setEditedContent(viewBase)}>
                <span className={styles.codeCopyText}>重置</span>
              </button>
            )}
            {mode === 'edit' && rootId && (
              <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" disabled={saving} onClick={handleSaveAsNewVersion}>
                <span className={styles.codeCopyIcon}><SaveOutlined /></span>
                <span className={styles.codeCopyText}>{saving ? '保存中...' : '保存为新版本'}</span>
              </button>
            )}
          </div>
        </div>

        {/* 内容区：按模式分发 */}
        {mode === 'view' && renderViewContent()}

        {mode === 'edit' && (
          <div className={styles.expandedEditorWrapper}>
            {rootId && renderAIEditPanel()}
            <Suspense fallback={<div className={styles.editorLoading}>加载编辑器...</div>}>
              <CodeEditor value={editedContent} language={lang || undefined} onChange={setEditedContent} onSelectionChange={setSelectedCode} />
            </Suspense>
          </div>
        )}

        {mode === 'diff' && (
          <div className={styles.expandedEditorWrapper}>
            <Suspense fallback={<div className={styles.editorLoading}>加载对比视图...</div>}>
              <DiffView oldDoc={diffOldDoc} newDoc={diffNewDoc} language={lang || undefined} />
            </Suspense>
          </div>
        )}
      </div>
    </Modal>
  );
};
