import React, { useState, useRef, useCallback } from 'react';
import { useMessages } from '@/hooks/useMessages';
import { useMessageStore } from '@/store/messageStore';
import styles from './ChatInput.module.css';

interface ChatInputProps {
  conversationId: string;
}

export const ChatInput: React.FC<ChatInputProps> = ({ conversationId }) => {
  const [value, setValue] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const { send } = useMessages(conversationId);
  const streamingContent = useMessageStore(
    (s) => s.streamingContent[conversationId] ?? '',
  );
  const isStreaming = streamingContent.length > 0;

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    setValue('');
    try {
      await send(trimmed);
    } catch {
      // 发送失败时恢复输入内容
      setValue(trimmed);
    }
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

  // 自适应高度
  const handleInput = useCallback(() => {
    const el = textareaRef.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = `${Math.min(el.scrollHeight, 120)}px`;
    }
  }, []);

  return (
    <div className={styles.container}>
      <div className={styles.inputRow}>
        <textarea
          ref={textareaRef}
          className={styles.textarea}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onInput={handleInput}
          onKeyDown={handleKeyDown}
          placeholder="输入消息...（Shift+Enter 换行）"
          rows={1}
        />
        <button
          className={styles.sendBtn}
          onClick={handleSubmit}
          disabled={!value.trim() || isStreaming}
        >
          发送
        </button>
      </div>
      {isStreaming && (
        <div className={styles.typing}>Agent 正在输入...</div>
      )}
    </div>
  );
};
