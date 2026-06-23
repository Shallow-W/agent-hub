import React, { useState, useCallback } from 'react';
import { message as antdMessage } from '@/utils/message';
import { cancelStreamingMessage } from '@/api/message';
import { useMessageStore } from '@/store/messageStore';
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
 *
 * 点击后乐观切 status=canceled 立即卸载按钮 / cursor，不等 backend/daemon 响应。
 * 设计：
 *   - 用户语义是"我要停"，无论 daemon 是否优雅响应（SIGINT 触发的 error_during_execution
 *     会被 daemon 当作 task.error 上报），前端都应立即停止"生成中"视觉态。
 *   - 若 API 调用失败（网络/鉴权），仍乐观切换——backend 最终会通过 watchdog 广播
 *     message.complete(status=error)，前端 addMessage 据此清理 streamingTaskIds。
 */
function StopButtonInner({ conversationId, messageId, taskId, onCanceled }: StopButtonProps) {
  const [submitting, setSubmitting] = useState(false);
  const cancelStreaming = useMessageStore((s) => s.cancelStreaming);

  const handleClick = useCallback(async () => {
    if (submitting) return;
    setSubmitting(true);
    // 乐观立即把 placeholder 切 canceled——不等 backend。
    // 这保证 cursor 消失 + StopButton 卸载（status !== 'streaming'），即使用户网络慢。
    cancelStreaming(conversationId, messageId);
    onCanceled?.();
    try {
      await cancelStreamingMessage(conversationId, messageId, taskId);
      antdMessage.success('已发送停止请求');
    } catch (err) {
      // API 失败不回滚状态——backend 会通过 watchdog 或 task.complete 广播终态。
      console.warn('cancel streaming API failed (UI already optimistically canceled):', err);
    }
  }, [submitting, conversationId, messageId, taskId, onCanceled, cancelStreaming]);

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
