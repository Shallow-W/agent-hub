import React, { useState, useRef, useCallback } from 'react';
import { useMessages } from '@/hooks/useMessages';
import styles from './ChatInput.module.css';

interface ChatInputProps {
  conversationId: string;
}

export const ChatInput: React.FC<ChatInputProps> = ({ conversationId }) => {
  const [value, setValue] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const { send, streamingContent } = useMessages(conversationId);
  const isStreaming = (streamingContent ?? '').length > 0;

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    setValue('');
    // 重置高度
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
    }
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

  // 自适应高度，上限150px
  const handleInput = useCallback(() => {
    const el = textareaRef.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = `${Math.min(el.scrollHeight, 150)}px`;
    }
  }, []);

  const charCount = value.length;

  return (
    <div className={styles.container}>
      {isStreaming && (
        <div className={styles.typingIndicator}>
          Agent 正在输入
          <span className={styles.typingDots}>
            <span />
            <span />
            <span />
          </span>
        </div>
      )}
      <div className={styles.inputRow}>
        <textarea
          ref={textareaRef}
          className={styles.textarea}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onInput={handleInput}
          onKeyDown={handleKeyDown}
          placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
          rows={1}
        />
        <button
          className={styles.sendBtn}
          onClick={handleSubmit}
          disabled={!value.trim() || isStreaming}
          aria-label="发送"
        >
          &#x27A4;
        </button>
      </div>
      <div className={styles.toolbar}>
        <button className={styles.attachBtn} aria-label="附件" title="附件">
          &#x1F4CE;
        </button>
        <span
          className={`${styles.charCount} ${charCount > 0 ? styles.charCountVisible : ''}`}
        >
          {charCount}
        </span>
      </div>
    </div>
  );
};
