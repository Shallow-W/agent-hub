import React, { useState, useCallback } from 'react';
import { Input, Button, Tooltip, Spin } from 'antd';
import { SendOutlined, PaperClipOutlined } from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import styles from './ChatInput.module.css';

const { TextArea } = Input;

interface ChatInputProps {
  conversationId: string;
}

export const ChatInput: React.FC<ChatInputProps> = ({ conversationId }) => {
  const [value, setValue] = useState('');
  const { send, streamingContent } = useMessages(conversationId);
  const isStreaming = (streamingContent ?? '').length > 0;

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    setValue('');
    await send(trimmed);
  }, [value, isStreaming, send]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  const charCount = value.length;

  return (
    <div className={styles.container}>
      {isStreaming && (
        <div className={styles.typingIndicator}>
          <Spin size="small" />
          <span>Agent 正在输入</span>
        </div>
      )}
      <div className={styles.inputRow}>
        <TextArea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
          autoSize={{ minRows: 1, maxRows: 4 }}
          className={styles.textarea}
        />
        <Button
          type="primary"
          shape="default"
          icon={<SendOutlined />}
          onClick={handleSubmit}
          disabled={!value.trim() || isStreaming}
          style={{ flexShrink: 0, height: 44, width: 44 }}
        />
      </div>
      <div className={styles.toolbar}>
        <Tooltip title="附件">
          <Button type="text" icon={<PaperClipOutlined />} size="small" />
        </Tooltip>
        <span
          className={`${styles.charCount} ${charCount > 0 ? styles.charCountVisible : ''}`}
        >
          {charCount}
        </span>
      </div>
    </div>
  );
};
