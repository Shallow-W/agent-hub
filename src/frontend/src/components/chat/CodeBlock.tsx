import React, { Suspense, lazy, useEffect, useState, type ReactNode } from 'react';
import { CheckOutlined, CopyOutlined, DownloadOutlined, EditOutlined, ExpandOutlined, EyeOutlined, SaveOutlined } from '@ant-design/icons';
import { Modal, Select, message as antMessage } from 'antd';
import type { Artifact } from '@/types/message';
import { listArtifactVersions, createArtifactVersion } from '@/api/artifact';
import { ApiError } from '@/api/client';
import { LANG_DISPLAY, highlightCode, inferDownloadName } from './highlight';
import styles from './CodeBlock.module.css';

// CodeMirror 较重，仅在用户切到“编辑”模式时才动态加载，保证首屏与“查看”模式不引入它。
const CodeEditor = lazy(() => import('./CodeEditor'));

/** 从 react-markdown 的 ReactNode 中递归提取纯文本。 */
export function extractText(node: ReactNode): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (node && typeof node === 'object' && 'props' in node) {
    return extractText((node as React.ReactElement).props.children);
  }
  return '';
}

interface CodeBlockProps {
  className?: string;
  children?: ReactNode;
  code?: string;
  language?: string;
  /** 产物文件名，用于全屏“下载”时推断文件名。 */
  filename?: string;
  /** 正文代码块是否显示“展开”入口。 */
  expandable?: boolean;
  /** 对应 code 产物的版本血缘根 id；有值才在全屏开放版本切换与“保存为新版本”。 */
  artifactRootId?: string;
}

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
  const [copied, setCopied] = useState(false);
  const [expandedOpen, setExpandedOpen] = useState(false);
  const [mode, setMode] = useState<'view' | 'edit'>('view');
  const [editedCode, setEditedCode] = useState(codeStr);

  // ── 版本能力（仅 artifactRootId 存在时启用） ──
  const [versions, setVersions] = useState<Artifact[]>([]);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';
  const highlighted = highlightCode(codeStr, lang || undefined);

  // 当前选中版本的产物（用于沿用 language/filename 等元信息）。
  const selectedArtifact = versions.find((v) => v.version === selectedVersion) ?? null;
  // 最新版本号（列表按 version 升序，末位为最新）。
  const latestVersion = versions.length ? versions[versions.length - 1]!.version : null;
  // 选中版本的基准内容：选到具体版本时用其 content，否则回退到原始代码。
  const baseCode = selectedArtifact?.content ?? codeStr;

  // 每次打开全屏都从基准内容起步，并默认“查看”模式。
  useEffect(() => {
    if (expandedOpen) {
      setEditedCode(baseCode);
      setMode('view');
    }
    // 仅在打开/关闭与基准内容变化时重置，避免编辑过程被覆盖。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [baseCode, expandedOpen]);

  // 懒拉版本列表：仅在打开全屏且有血缘根时拉取一次，默认选最新版。
  useEffect(() => {
    if (!expandedOpen || !artifactRootId) return;
    let cancelled = false;
    setVersionsLoading(true);
    listArtifactVersions(artifactRootId)
      .then((list) => {
        if (cancelled) return;
        setVersions(list);
        const latest = list.length ? list[list.length - 1]!.version : null;
        setSelectedVersion(latest);
      })
      .catch((err) => {
        if (cancelled) return;
        const msg = err instanceof ApiError ? err.message : '加载版本列表失败';
        antMessage.error(msg);
      })
      .finally(() => {
        if (!cancelled) setVersionsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [expandedOpen, artifactRootId]);

  // 切换版本：把查看/编辑区切到该版本内容，回到查看模式。
  const handleSelectVersion = (version: number) => {
    setSelectedVersion(version);
    const target = versions.find((v) => v.version === version);
    setEditedCode(target?.content ?? codeStr);
    setMode('view');
  };

  // 保存为新版本：POST 当前编辑内容，成功后刷新列表并选中新版本。
  const handleSaveAsNewVersion = async () => {
    if (!artifactRootId || saving) return;
    setSaving(true);
    try {
      const created = await createArtifactVersion(artifactRootId, {
        content: editedCode,
        type: 'code',
        ...(lang ? { language: lang } : {}),
        ...(selectedArtifact?.filename ? { filename: selectedArtifact.filename } : filename ? { filename } : {}),
      });
      const list = await listArtifactVersions(artifactRootId);
      setVersions(list);
      setSelectedVersion(created.version);
      setEditedCode(created.content ?? editedCode);
      antMessage.success(`已保存为 v${created.version}`);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : '保存新版本失败';
      antMessage.error(msg);
    } finally {
      setSaving(false);
    }
  };

  const handleCopy = (value = codeStr) => {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => { /* clipboard unavailable */ });
  };

  // 基于编辑后的最新内容下载为文件。
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

  const editedHighlighted = highlightCode(editedCode, lang || undefined);

  return (
    <div className={styles.codeBlockWrapper}>
      <div className={styles.codeHeader}>
        <span>{displayLang}</span>
        <div className={styles.codeActions}>
          {expandable && (
            <button
              className={styles.codeActionBtn}
              type="button"
              title="全屏查看代码"
              onClick={() => setExpandedOpen(true)}
            >
              <span className={styles.codeCopyIcon}>
                <ExpandOutlined />
              </span>
              <span className={styles.codeCopyText}>展开</span>
            </button>
          )}
          <button
            className={`${styles.codeActionBtn} ${copied ? styles.codeCopyBtnCopied : ''}`}
            type="button"
            title="复制代码"
            onClick={() => handleCopy()}
          >
            <span className={styles.codeCopyIcon}>
              {copied ? <CheckOutlined /> : <CopyOutlined />}
            </span>
            <span className={styles.codeCopyText}>{copied ? '已复制' : '复制'}</span>
          </button>
        </div>
      </div>
      <pre className={styles.codeBlock}>
        <code dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
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
                {artifactRootId && versions.length > 0 && (
                  <Select<number>
                    size="small"
                    className={styles.versionSelect}
                    value={selectedVersion ?? undefined}
                    loading={versionsLoading}
                    onChange={handleSelectVersion}
                    options={versions.map((v) => ({
                      value: v.version,
                      label: v.version === latestVersion
                        ? `v${v.version}（最新）`
                        : `v${v.version}`,
                    }))}
                  />
                )}
              </div>
              <div className={styles.expandedHeaderActions}>
                <button
                  className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`}
                  type="button"
                  title={mode === 'view' ? '切换到编辑' : '切换到查看'}
                  onClick={() => setMode(mode === 'view' ? 'edit' : 'view')}
                >
                  <span className={styles.codeCopyIcon}>
                    {mode === 'view' ? <EditOutlined /> : <EyeOutlined />}
                  </span>
                  <span className={styles.codeCopyText}>{mode === 'view' ? '编辑' : '查看'}</span>
                </button>
                <button
                  className={`${styles.codeActionBtn} ${styles.expandedCopyBtn} ${copied ? styles.codeCopyBtnCopied : ''}`}
                  type="button"
                  title="复制代码"
                  onClick={() => handleCopy(editedCode)}
                >
                  <span className={styles.codeCopyIcon}>
                    {copied ? <CheckOutlined /> : <CopyOutlined />}
                  </span>
                  <span className={styles.codeCopyText}>{copied ? '已复制' : '复制'}</span>
                </button>
                <button
                  className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`}
                  type="button"
                  title="下载为文件"
                  onClick={handleDownload}
                >
                  <span className={styles.codeCopyIcon}>
                    <DownloadOutlined />
                  </span>
                  <span className={styles.codeCopyText}>下载</span>
                </button>
                {mode === 'edit' && (
                  <button
                    className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`}
                    type="button"
                    title="恢复当前版本内容"
                    onClick={() => setEditedCode(baseCode)}
                  >
                    <span className={styles.codeCopyText}>重置</span>
                  </button>
                )}
                {mode === 'edit' && artifactRootId && (
                  <button
                    className={`${styles.codeActionBtn} ${styles.expandedCopyBtn}`}
                    type="button"
                    title="将当前编辑内容保存为新版本"
                    disabled={saving}
                    onClick={handleSaveAsNewVersion}
                  >
                    <span className={styles.codeCopyIcon}>
                      <SaveOutlined />
                    </span>
                    <span className={styles.codeCopyText}>{saving ? '保存中…' : '保存为新版本'}</span>
                  </button>
                )}
              </div>
            </div>
            {mode === 'view' ? (
              <pre className={styles.expandedCodeBlock}>
                <code dangerouslySetInnerHTML={{ __html: editedHighlighted }} />
              </pre>
            ) : (
              <div className={styles.expandedEditorWrapper}>
                <Suspense fallback={<div className={styles.editorLoading}>加载编辑器…</div>}>
                  <CodeEditor value={editedCode} language={lang || undefined} onChange={setEditedCode} />
                </Suspense>
              </div>
            )}
          </div>
        </Modal>
      )}
    </div>
  );
};
