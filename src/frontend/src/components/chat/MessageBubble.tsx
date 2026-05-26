import React from 'react';
import { Avatar, Typography } from 'antd';
import { UserOutlined } from '@ant-design/icons';
import type { Message } from '@/types/message';
import styles from './MessageBubble.module.css';

const { Paragraph, Text } = Typography;

interface MessageBubbleProps {
  message: Message;
  streaming?: boolean;
  showAvatar?: boolean;
  isGrouped?: boolean;
}

function CodeBlock({ code, lang }: { code: string; lang: string }) {
  return (
    <div className={styles.codeBlockWrapper}>
      <div className={styles.codeHeader}>
        <span>{lang || 'Code'}</span>
      </div>
      <Paragraph
        copyable={{ text: code, tooltips: ['复制', '已复制'] }}
        style={{ margin: 0 }}
      >
        <pre className={styles.codeBlock}>
          <code>{code}</code>
        </pre>
      </Paragraph>
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
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const msgDate = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');

  if (msgDate.getTime() === today.getTime()) {
    return `${hh}:${mm}`;
  }
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${month}-${day} ${hh}:${mm}`;
}

export const MessageBubble: React.FC<MessageBubbleProps> = ({
  message,
  streaming = false,
  showAvatar = true,
  isGrouped = false,
}) => {
  const isUser = message.role === 'user';
  const isSystem = message.role === 'system';

  if (isSystem) {
    return (
      <div className={styles.systemMessage}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          {message.content}
        </Text>
      </div>
    );
  }

  return (
    <div
      className={`${styles.bubble} ${isUser ? styles.bubbleUser : styles.bubbleAssistant} ${isGrouped ? styles.bubbleGrouped : ''}`}
    >
      {!isUser && showAvatar && (
        <Avatar
          size={32}
          icon={<UserOutlined />}
          style={{ backgroundColor: '#1677ff', flexShrink: 0, marginRight: 10, marginTop: 2 }}
        >
          {message.content?.charAt(0)?.toUpperCase() ?? ''}
        </Avatar>
      )}
      {!isUser && !showAvatar && <div style={{ width: 42, flexShrink: 0 }} />}
      <div className={`${styles.content} ${isUser ? styles.contentUser : styles.contentAssistant}`}>
        {!isUser && showAvatar && (
          <div className={styles.meta}>
            <Text type="secondary" style={{ fontSize: 12, fontWeight: 500 }}>Agent</Text>
          </div>
        )}
        <div
          className={`${styles.inner} ${isUser ? styles.innerUser : styles.innerAssistant}`}
        >
          {renderContent(message.content)}
          {streaming && <span className={styles.streamingCursor} />}
        </div>
        {showAvatar && (
          <div
            className={`${styles.timestamp} ${isUser ? styles.timestampUser : styles.timestampAssistant}`}
          >
            <Text type="secondary" style={{ fontSize: 11 }}>
              {formatTimestamp(message.created_at)}
            </Text>
          </div>
        )}
      </div>
    </div>
  );
};
