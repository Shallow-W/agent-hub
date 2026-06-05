import React, { useEffect, useState, type ReactNode } from 'react';
import { CheckOutlined, CopyOutlined, ExpandOutlined } from '@ant-design/icons';
import { Modal } from 'antd';
import { LANG_DISPLAY, highlightCode } from './highlight';
import styles from './CodeBlock.module.css';

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
  /** 正文代码块是否显示“展开”入口。 */
  expandable?: boolean;
}

export const CodeBlock: React.FC<CodeBlockProps> = ({
  className,
  children,
  code,
  language,
  expandable = false,
}) => {
  const lang = language ?? (className?.replace('language-', '') || '');
  const codeStr = (code ?? extractText(children)).replace(/\n$/, '');
  const [copied, setCopied] = useState(false);
  const [expandedOpen, setExpandedOpen] = useState(false);
  const [editedCode, setEditedCode] = useState(codeStr);

  const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';
  const highlighted = highlightCode(codeStr, lang || undefined);

  useEffect(() => {
    if (expandedOpen) setEditedCode(codeStr);
  }, [codeStr, expandedOpen]);

  const handleCopy = (value = codeStr) => {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => { /* clipboard unavailable */ });
  };

  const handleEditorKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== 'Tab') return;
    event.preventDefault();
    const target = event.currentTarget;
    const start = target.selectionStart;
    const end = target.selectionEnd;
    const nextCode = `${editedCode.slice(0, start)}  ${editedCode.slice(end)}`;
    setEditedCode(nextCode);
    window.requestAnimationFrame(() => {
      target.selectionStart = start + 2;
      target.selectionEnd = start + 2;
    });
  };

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
                title="恢复原始代码"
                onClick={() => setEditedCode(codeStr)}
              >
                <span className={styles.codeCopyText}>重置</span>
              </button>
            </div>
            <textarea
              className={styles.expandedCodeEditor}
              value={editedCode}
              spellCheck={false}
              onChange={(event) => setEditedCode(event.target.value)}
              onKeyDown={handleEditorKeyDown}
            />
          </div>
        </Modal>
      )}
    </div>
  );
};
