import React, { Suspense, lazy, useEffect, useState, type ReactNode } from 'react';
import { CheckOutlined, CopyOutlined, DownloadOutlined, EditOutlined, ExpandOutlined, EyeOutlined } from '@ant-design/icons';
import { Modal } from 'antd';
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
}

export const CodeBlock: React.FC<CodeBlockProps> = ({
  className,
  children,
  code,
  language,
  filename,
  expandable = false,
}) => {
  const lang = language ?? (className?.replace('language-', '') || '');
  const codeStr = (code ?? extractText(children)).replace(/\n$/, '');
  const [copied, setCopied] = useState(false);
  const [expandedOpen, setExpandedOpen] = useState(false);
  const [mode, setMode] = useState<'view' | 'edit'>('view');
  const [editedCode, setEditedCode] = useState(codeStr);

  const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';
  const highlighted = highlightCode(codeStr, lang || undefined);

  // 每次打开全屏都从原始代码起步，并默认“查看”模式。
  useEffect(() => {
    if (expandedOpen) {
      setEditedCode(codeStr);
      setMode('view');
    }
  }, [codeStr, expandedOpen]);

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
              <span>{displayLang}</span>
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
                    title="恢复原始代码"
                    onClick={() => setEditedCode(codeStr)}
                  >
                    <span className={styles.codeCopyText}>重置</span>
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
