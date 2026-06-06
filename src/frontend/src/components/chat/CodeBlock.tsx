import React, { Suspense, lazy, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  CheckOutlined,
  CopyOutlined,
  DiffOutlined,
  DownloadOutlined,
  EditOutlined,
  ExpandOutlined,
  EyeOutlined,
  RobotOutlined,
  RollbackOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import { Input, Modal, Select, message as antMessage } from 'antd';
import type { Artifact } from '@/types/message';
import { aiEditArtifact, createArtifactVersion, listArtifactVersions } from '@/api/artifact';
import { ApiError } from '@/api/client';
import { LANG_DISPLAY, highlightCode, inferDownloadName } from './highlight';
import styles from './CodeBlock.module.css';

const CodeEditor = lazy(() => import('./CodeEditor'));
const DiffView = lazy(() => import('./DiffView'));
const { TextArea } = Input;

export function extractText(node: ReactNode): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (React.isValidElement(node)) {
    return extractText((node.props as { children?: ReactNode }).children);
  }
  return '';
}

interface CodeBlockProps {
  className?: string;
  children?: ReactNode;
  code?: string;
  language?: string;
  filename?: string;
  expandable?: boolean;
  artifactRootId?: string;
}

type CodeMode = 'view' | 'edit' | 'diff';

export const CodeBlock: React.FC<CodeBlockProps> = ({
  className,
  children,
  code,
  language,
  filename,
  expandable = false,
  artifactRootId,
}) => {
  const lang = language ?? (className?.replace('language-', '') || '');
  const codeStr = (code ?? extractText(children)).replace(/\n$/, '');
  const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';

  const [copied, setCopied] = useState(false);
  const [expandedOpen, setExpandedOpen] = useState(false);
  const [mode, setMode] = useState<CodeMode>('view');
  const [editedCode, setEditedCode] = useState(codeStr);
  const [selectedCode, setSelectedCode] = useState('');
  const [aiInstruction, setAiInstruction] = useState('');
  const [aiEditing, setAiEditing] = useState(false);
  const inlineCodeRef = useRef<HTMLPreElement>(null);
  const expandedCodeRef = useRef<HTMLPreElement>(null);

  const [versions, setVersions] = useState<Artifact[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const [diffOldVersion, setDiffOldVersion] = useState<number | null>(null);
  const [diffNewVersion, setDiffNewVersion] = useState<number | null>(null);

  const selectedArtifact = versions.find((v) => v.version === selectedVersion) ?? null;
  const latestVersion = versions.length ? versions[versions.length - 1]!.version : null;
  const baseCode = selectedArtifact?.content ?? codeStr;
  const canDiff = Boolean(artifactRootId && versions.length >= 2);

  const highlighted = useMemo(() => highlightCode(codeStr, lang || undefined), [codeStr, lang]);
  const editedHighlighted = useMemo(() => highlightCode(editedCode, lang || undefined), [editedCode, lang]);
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

  const refreshVersions = async (created?: Artifact, fallbackContent = codeStr) => {
    if (!artifactRootId) return;
    const list = await listArtifactVersions(artifactRootId);
    setVersions(list);
    syncDiffDefaults(list);
    if (created) {
      setSelectedVersion(created.version);
      setEditedCode(created.content ?? fallbackContent);
      return;
    }
    const latest = list.length ? list[list.length - 1]!.version : null;
    setSelectedVersion(latest);
  };

  useEffect(() => {
    if (!expandedOpen) return;
    setEditedCode(baseCode);
    setSelectedCode('');
    setAiInstruction('');
    setMode('view');
  }, [baseCode, expandedOpen]);

  useEffect(() => {
    if (!expandedOpen || !artifactRootId) return;
    let cancelled = false;
    setVersionsLoading(true);
    listArtifactVersions(artifactRootId)
      .then((list) => {
        if (cancelled) return;
        setVersions(list);
        setSelectedVersion(list.length ? list[list.length - 1]!.version : null);
        syncDiffDefaults(list);
      })
      .catch((err) => {
        if (cancelled) return;
        antMessage.error(err instanceof ApiError ? err.message : '加载版本列表失败');
      })
      .finally(() => {
        if (!cancelled) setVersionsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [expandedOpen, artifactRootId]);

  const handleSelectVersion = (version: number) => {
    setSelectedVersion(version);
    const target = versions.find((v) => v.version === version);
    setEditedCode(target?.content ?? codeStr);
    setSelectedCode('');
    setMode('view');
  };

  const createNewVersionFromContent = async (content: string): Promise<number> => {
    if (!artifactRootId) throw new Error('no artifactRootId');
    const created = await createArtifactVersion(artifactRootId, {
      content,
      type: 'code',
      ...(lang ? { language: lang } : {}),
      ...(selectedArtifact?.filename ? { filename: selectedArtifact.filename } : filename ? { filename } : {}),
    });
    await refreshVersions(created, content);
    return created.version;
  };

  const captureSelectionFrom = (container: HTMLElement | null) => {
    const selection = window.getSelection();
    if (!selection || selection.rangeCount === 0 || !container) return;
    const range = selection.getRangeAt(0);
    if (!container.contains(range.commonAncestorContainer)) return;
    const text = selection.toString();
    setSelectedCode(text.trim() ? text : '');
  };

  const handleSaveAsNewVersion = async () => {
    if (!artifactRootId || saving) return;
    setSaving(true);
    try {
      const newVer = await createNewVersionFromContent(editedCode);
      antMessage.success(`已保存为 v${newVer}`);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : '保存新版本失败');
    } finally {
      setSaving(false);
    }
  };

  const handleRollback = async () => {
    if (!artifactRootId || saving || !selectedArtifact) return;
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

  const handleAIEdit = async () => {
    if (!artifactRootId || aiEditing) return;
    const instruction = aiInstruction.trim();
    if (!instruction) {
      antMessage.warning('先写一下你希望 AI 怎么改');
      return;
    }

    setAiEditing(true);
    try {
      const created = await aiEditArtifact(artifactRootId, {
        instruction,
        ...(selectedCode.trim() ? { selection: selectedCode } : {}),
      });
      await refreshVersions(created, created.content ?? editedCode);
      setAiInstruction('');
      setSelectedCode('');
      setMode('view');
      setExpandedOpen(true);
      antMessage.success(`AI 已生成 v${created.version}`);
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : 'AI 修改失败');
    } finally {
      setAiEditing(false);
    }
  };

  const handleCopy = (value = codeStr) => {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => {
      antMessage.error('复制失败');
    });
  };

  const handleDownload = () => {
    const blob = new Blob([editedCode], { type: 'text/plain;charset=utf-8' });
    const objectUrl = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = objectUrl;
    anchor.download = inferDownloadName(filename, lang || undefined);
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
    URL.revokeObjectURL(objectUrl);
  };

  const renderAIEditPanel = (compact = false) => (
    <div className={`${styles.aiEditPanel} ${compact ? styles.aiEditPanelCompact : ''}`}>
      <div className={styles.aiEditMeta}>
        <span className={styles.aiEditTitle}>
          <RobotOutlined />
          AI 局部修改
        </span>
        <span className={styles.aiEditSelection}>
          {selectedCode.trim() ? `已选中 ${selectedCode.length} 个字符` : '未选中代码，将按整份代码修改'}
        </span>
      </div>
      <div className={styles.aiEditControls}>
        <TextArea
          className={styles.aiEditInput}
          value={aiInstruction}
          onChange={(event) => setAiInstruction(event.target.value)}
          placeholder="描述你想怎么改选中的代码"
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

  return (
    <div className={styles.codeBlockWrapper}>
      <div className={styles.codeHeader}>
        <span>{displayLang}</span>
        <div className={styles.codeActions}>
          {expandable && (
            <button className={styles.codeActionBtn} type="button" title="展开代码" onClick={() => setExpandedOpen(true)}>
              <span className={styles.codeCopyIcon}><ExpandOutlined /></span>
              <span className={styles.codeCopyText}>展开</span>
            </button>
          )}
          <button
            className={`${styles.codeActionBtn} ${copied ? styles.codeCopyBtnCopied : ''}`}
            type="button"
            title="复制代码"
            onClick={() => handleCopy()}
          >
            <span className={styles.codeCopyIcon}>{copied ? <CheckOutlined /> : <CopyOutlined />}</span>
            <span className={styles.codeCopyText}>{copied ? '已复制' : '复制'}</span>
          </button>
        </div>
      </div>
      <pre
        ref={inlineCodeRef}
        className={styles.codeBlock}
        onMouseUp={() => captureSelectionFrom(inlineCodeRef.current)}
        onKeyUp={() => captureSelectionFrom(inlineCodeRef.current)}
      >
        <code dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
      {artifactRootId && selectedCode.trim() && !expandedOpen && renderAIEditPanel(true)}

      {expandable && (
        <Modal
          open={expandedOpen}
          onCancel={() => setExpandedOpen(false)}
          footer={null}
          width="94vw"
          style={{ top: 16, maxWidth: 'none' }}
          title={displayLang || '代码'}
          className={styles.expandedCodeModal}
          destroyOnClose
        >
          <div className={styles.expandedCodeView}>
            <div className={styles.expandedCodeHeader}>
              <div className={styles.expandedHeaderLeft}>
                <span>{displayLang}</span>
                {mode === 'diff' ? (
                  <>
                    <span className={styles.diffLabel}>旧</span>
                    <Select<number>
                      size="small"
                      className={styles.versionSelect}
                      value={diffOldVersion ?? undefined}
                      onChange={setDiffOldVersion}
                      options={versionOptions}
                    />
                    <span className={styles.diffLabel}>新</span>
                    <Select<number>
                      size="small"
                      className={styles.versionSelect}
                      value={diffNewVersion ?? undefined}
                      onChange={setDiffNewVersion}
                      options={versionOptions}
                    />
                  </>
                ) : (
                  artifactRootId && versions.length > 0 && (
                    <Select<number>
                      size="small"
                      className={styles.versionSelect}
                      value={selectedVersion ?? undefined}
                      loading={versionsLoading}
                      onChange={handleSelectVersion}
                      options={versionOptions}
                    />
                  )
                )}
              </div>

              <div className={styles.expandedHeaderActions}>
                {mode === 'diff' ? (
                  <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={() => setMode('view')}>
                    <span className={styles.codeCopyIcon}><EyeOutlined /></span>
                    <span className={styles.codeCopyText}>查看</span>
                  </button>
                ) : (
                  <>
                    <button
                      className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`}
                      type="button"
                      onClick={() => setMode(mode === 'view' ? 'edit' : 'view')}
                    >
                      <span className={styles.codeCopyIcon}>{mode === 'view' ? <EditOutlined /> : <EyeOutlined />}</span>
                      <span className={styles.codeCopyText}>{mode === 'view' ? '编辑' : '查看'}</span>
                    </button>
                    {canDiff && (
                      <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={() => setMode('diff')}>
                        <span className={styles.codeCopyIcon}><DiffOutlined /></span>
                        <span className={styles.codeCopyText}>Diff</span>
                      </button>
                    )}
                    {mode === 'view' && artifactRootId && selectedVersion !== null && selectedVersion !== latestVersion && (
                      <button
                        className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`}
                        type="button"
                        disabled={saving}
                        onClick={handleRollback}
                      >
                        <span className={styles.codeCopyIcon}><RollbackOutlined /></span>
                        <span className={styles.codeCopyText}>{saving ? '回滚中...' : '回滚到此版本'}</span>
                      </button>
                    )}
                    <button
                      className={`${styles.codeActionBtn} ${styles.expandedCopyBtn} ${copied ? styles.codeCopyBtnCopied : ''}`}
                      type="button"
                      onClick={() => handleCopy(editedCode)}
                    >
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
                  <button className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`} type="button" onClick={() => setEditedCode(baseCode)}>
                    <span className={styles.codeCopyText}>重置</span>
                  </button>
                )}
                {mode === 'edit' && artifactRootId && (
                  <button
                    className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`}
                    type="button"
                    disabled={saving}
                    onClick={handleSaveAsNewVersion}
                  >
                    <span className={styles.codeCopyIcon}><SaveOutlined /></span>
                    <span className={styles.codeCopyText}>{saving ? '保存中...' : '保存为新版本'}</span>
                  </button>
                )}
              </div>
            </div>

            {mode === 'view' && (
              <>
                {artifactRootId && selectedCode.trim() && renderAIEditPanel()}
                <pre
                  ref={expandedCodeRef}
                  className={styles.expandedCodeBlock}
                  onMouseUp={() => captureSelectionFrom(expandedCodeRef.current)}
                  onKeyUp={() => captureSelectionFrom(expandedCodeRef.current)}
                >
                <code dangerouslySetInnerHTML={{ __html: editedHighlighted }} />
                </pre>
              </>
            )}

            {mode === 'edit' && (
              <div className={styles.expandedEditorWrapper}>
                {artifactRootId && renderAIEditPanel()}
                <Suspense fallback={<div className={styles.editorLoading}>加载编辑器...</div>}>
                  <CodeEditor
                    value={editedCode}
                    language={lang || undefined}
                    onChange={setEditedCode}
                    onSelectionChange={setSelectedCode}
                  />
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
      )}
    </div>
  );
};
