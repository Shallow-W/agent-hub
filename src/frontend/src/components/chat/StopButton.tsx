import React, { useState, useCallback } from 'react';
import { message as antdMessage } from '@/utils/message';
import { cancelStreamingMessage } from '@/api/message';
import styles from './StopButton.module.css';

interface StopButtonProps {
  conversationId: string;
  messageId: string;
  /** 流式 task_id（从 message.streaming payload 回传，后端定位 daemon task 用） */
  taskId?: string;
  /** 可选：取消成功后调用（如更新本地 status） */
  onCanceled?: () => void;
}

/**
 * 停止生成按钮——仅当 assistant message status === 'streaming' 时渲染。
 * 点击发 HTTP POST 取消，不等 daemon 响应，UI 立即 disable。
 *
 * daemon 侧收到 task.cancel 后 SIGINT Claude 进程，触发 task.complete with error，
 * 后端 watchdog FinalizeStreaming 切到 canceled/error 状态并广播 message.complete。
 */
function StopButtonInner({ conversationId, messageId, taskId, onCanceled }: StopButtonProps) {
  const [submitting, setSubmitting] = useState(false);

  const handleClick = useCallback(async () => {
    if (submitting) return;
    setSubmitting(true);
    try {
      await cancelStreamingMessage(conversationId, messageId, taskId);
      antdMessage.success('已发送停止请求');
      onCanceled?.();
    } catch (err) {
      console.error('cancel streaming failed:', err);
      antdMessage.error('停止失败，请重试');
      setSubmitting(false);
    }
  }, [submitting, conversationId, messageId, taskId, onCanceled]);

  return (
    <button
      type="button"
      className={styles.stopBtn}
      onClick={handleClick}
      disabled={submitting}
      aria-label="停止生成"
    >
      <span className={styles.stopIcon} aria-hidden />
      {submitting ? '停止中…' : '停止生成'}
    </button>
  );
}

export const StopButton = React.memo(StopButtonInner);
