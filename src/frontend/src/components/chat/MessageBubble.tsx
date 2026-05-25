import React, { useState, useCallback } from 'react';
import type { Message } from '@/types/message';
import styles from './MessageBubble.module.css';

interface MessageBubbleProps {
  message: Message;
  streaming?: boolean;
}

/** 简单分割：用 ``` 包裹的代码块渲染为带头部的暗色代码区域 */
function CodeBlock({ code, lang }: { code: string; lang: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }, [code]);

  return (
    <div className={styles.codeBlockWrapper}>
      <div className={styles.codeHeader}>
        <span>{lang || 'Code'}</span>
        <button className={styles.copyBtn} type="button" aria-label="复制代码" onClick={handleCopy}>
          {copied ? '✓ 已复制' : '\u{1F4CB} 复制'}
        </button>
      </div>
      <pre className={styles.codeBlock}>
        <code>{code}</code>
      </pre>
    </div>
  );
}

function renderContent(content: string): React.ReactNode {
  const parts = content.split(/(```[\s\S]*?```)/g);

  return parts.map((part, i) => {
    if (part.startsWith('```') && part.endsWith('```')) {
      const lines = part.slice(3, -3);
      const firstNewline = lines.indexOf('\n');
      const langMatch = firstNewline >= 0 ? lines.slice(0, firstNewline).trim() : '';
      const code =
        firstNewline >= 0 ? lines.slice(firstNewline + 1) : lines;
      return <CodeBlock key={i} code={code} lang={langMatch} />;
    }
    // 保留换行
    return part.split('\n').map((line, j, arr) => (
      <React.Fragment key={`${i}-${j}`}>
        {line}
        {j < arr.length - 1 && <br />}
      </React.Fragment>
    ));
  });
}

function formatTimestamp(dateStr: string): string {
  const d = new Date(dateStr);
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  return `${hh}:${mm}`;
}

export const MessageBubble: React.FC<MessageBubbleProps> = ({
  message,
  streaming = false,
}) => {
  const isUser = message.role === 'user';

  return (
    <div className={`${styles.bubble} ${isUser ? styles.bubbleUser : styles.bubbleAssistant}`}>
      {!isUser && (
        <div className={styles.avatar} aria-hidden="true">A</div>
      )}
      <div className={`${styles.content} ${isUser ? styles.contentUser : styles.contentAssistant}`}>
        {!isUser && (
          <div className={styles.meta}>
            <span className={styles.agentName}>Agent</span>
          </div>
        )}
        <div
          className={`${styles.inner} ${isUser ? styles.innerUser : styles.innerAssistant}`}
        >
          {renderContent(message.content)}
          {streaming && <span className={styles.streamingCursor} />}
        </div>
        <div
          className={`${styles.timestamp} ${isUser ? styles.timestampUser : styles.timestampAssistant}`}
        >
          {formatTimestamp(message.created_at)}
        </div>
      </div>
    </div>
  );
};
