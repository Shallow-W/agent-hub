import React, { useState, type ReactNode } from 'react';
import { CheckOutlined, CopyOutlined } from '@ant-design/icons';
import { LANG_DISPLAY, highlightCode } from './highlight';
import styles from './CodeBlock.module.css';

/** 递归从 ReactNode 提取纯文本（处理 react-markdown v10 元素子节点）。 */
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
  /** markdown 用法：language-xxx className；优先级低于显式 code/language */
  className?: string;
  /** markdown 用法：渲染子节点中提取代码 */
  children?: ReactNode;
  /** 产物用法：显式传入源码 */
  code?: string;
  /** 产物用法：显式传入语言 */
  language?: string;
}

/**
 * 带语言头与复制按钮的代码块。
 * 同时服务于 markdown 围栏代码（className+children）与结构化产物（code+language），
 * 避免在 ArtifactCard / Workspace 里重复粘贴一份代码卡。
 */
export const CodeBlock: React.FC<CodeBlockProps> = ({
  className,
  children,
  code,
  language,
}) => {
  const lang = language ?? (className?.replace('language-', '') || '');
  const codeStr = (code ?? extractText(children)).replace(/\n$/, '');
  const [copied, setCopied] = useState(false);

  const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';
  const highlighted = highlightCode(codeStr, lang || undefined);

  const handleCopy = () => {
    navigator.clipboard.writeText(codeStr).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => { /* clipboard unavailable */ });
  };

  return (
    <div className={styles.codeBlockWrapper}>
      <div className={styles.codeHeader}>
        <span>{displayLang}</span>
        <button
          className={`${styles.codeCopyBtn} ${copied ? styles.codeCopyBtnCopied : ''}`}
          type="button"
          title="复制代码"
          onClick={handleCopy}
        >
          <span className={styles.codeCopyIcon}>
            {copied ? <CheckOutlined /> : <CopyOutlined />}
          </span>
          <span className={styles.codeCopyText}>{copied ? '已复制' : '复制'}</span>
        </button>
      </div>
      <pre className={styles.codeBlock}>
        <code dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
    </div>
  );
};
