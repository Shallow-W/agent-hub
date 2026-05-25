import React from 'react';
import type { Message } from '@/types/message';
import styles from './MessageBubble.module.css';

interface MessageBubbleProps {
  message: Message;
  streaming?: boolean;
}

/** 简单分割：用 ``` 包裹的代码块渲染为 pre/code */
function renderContent(content: string): React.ReactNode {
  const parts = content.split(/(```[\s\S]*?```)/g);

  return parts.map((part, i) => {
    if (part.startsWith('```') && part.endsWith('```')) {
      const lines = part.slice(3, -3);
      const firstNewline = lines.indexOf('\n');
      const code =
        firstNewline >= 0 ? lines.slice(firstNewline + 1) : lines;
      return (
        <pre className={styles.codeBlock} key={i}>
          <code>{code}</code>
        </pre>
      );
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
      <div>
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
