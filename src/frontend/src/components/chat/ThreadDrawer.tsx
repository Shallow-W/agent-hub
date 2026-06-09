import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Avatar, Drawer, Spin, Input, Button } from 'antd';
import { SendOutlined, RobotOutlined, UserOutlined } from '@ant-design/icons';
import { message as antMessage } from '@/utils/message';
import type { Message } from '@/types/message';
import { getMessageReplies } from '@/api/message';
import { useMessageStore } from '@/store/messageStore';
import { useAuthStore } from '@/store/authStore';
import { useAgentStore } from '@/store/agentStore';
import { escapeHtml } from './highlight';
import { resolveAgentAvatar, resolveUserAvatar } from '@/components/agent/agentPresentation';
import styles from './ThreadDrawer.module.css';

interface ThreadDrawerProps {
  conversationId: string;
  originalMessage: Message | null;
  open: boolean;
  onClose: () => void;
}

function formatTimestamp(dateStr: string): string {
  const d = new Date(dateStr);
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  const hh = String(d.getHours()).padStart(2, '0');
  const min = String(d.getMinutes()).padStart(2, '0');
  return `${mm}-${dd} ${hh}:${min}`;
}

export const ThreadDrawer: React.FC<ThreadDrawerProps> = ({
  conversationId,
  originalMessage,
  open,
  onClose,
}) => {
  const [replies, setReplies] = useState<Message[]>([]);
  const [loading, setLoading] = useState(false);
  const [sending, setSending] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const listRef = useRef<HTMLDivElement>(null);
  const sendMessage = useMessageStore((s) => s.sendMessage);

  // Fetch replies when drawer opens
  useEffect(() => {
    if (!open || !originalMessage) return;
    setLoading(true);
    getMessageReplies(conversationId, originalMessage.id)
      .then((data) => setReplies(data))
      .catch(() => antMessage.error('加载回复失败'))
      .finally(() => setLoading(false));
  }, [open, originalMessage, conversationId]);

  // Scroll to bottom when replies change
  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [replies]);

  // Clear on close
  const handleClose = () => {
    setReplies([]);
    setInputValue('');
    onClose();
  };

  // Send reply
  const handleSend = useCallback(async () => {
    if (!originalMessage || !inputValue.trim()) return;
    setSending(true);
    try {
      const replyPreview = {
        id: originalMessage.id,
        content: (originalMessage.content ?? '').slice(0, 50),
        sender_id: originalMessage.sender_id,
        username: originalMessage.username,
      };
      await sendMessage(
        conversationId,
        inputValue.trim(),
        undefined,
        originalMessage.id,
        replyPreview,
      );
      setInputValue('');
      // Refresh replies
      const data = await getMessageReplies(conversationId, originalMessage.id);
      setReplies(data);
    } catch {
      antMessage.error('发送失败');
    } finally {
      setSending(false);
    }
  }, [originalMessage, inputValue, conversationId, sendMessage]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const title = originalMessage
    ? `回复: ${escapeHtml(originalMessage.username || (originalMessage.sender_id ? '用户' : '助手'))}`
    : '';

  return (
    <Drawer
      title={title}
      open={open}
      onClose={handleClose}
      width={420}
      placement="right"
      className={styles.drawer}
      destroyOnClose
    >
      {originalMessage && (
        <>
          <OriginalMessageBubble message={originalMessage} />
          <div className={styles.divider}>
            {replies.length} 条回复
          </div>
        </>
      )}
      <div className={styles.replyList} ref={listRef}>
        {loading ? (
          <div className={styles.loadingArea}>
            <Spin size="small" />
          </div>
        ) : replies.length === 0 ? (
          <div className={styles.emptyReplies}>
            暂无回复
          </div>
        ) : (
          replies.map((msg) => (
            <ReplyBubble key={msg.id} message={msg} />
          ))
        )}
      </div>
      <div className={styles.inputArea}>
        <Input.TextArea
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="输入回复..."
          autoSize={{ minRows: 1, maxRows: 4 }}
          disabled={sending}
        />
        <Button
          type="primary"
          icon={<SendOutlined />}
          onClick={handleSend}
          loading={sending}
          disabled={!inputValue.trim()}
          className={styles.sendBtn}
        />
      </div>
    </Drawer>
  );
};

// ── Sub-components ──

const OriginalMessageBubble: React.FC<{ message: Message }> = ({ message }) => {
  const isOwn = message.sender_id === useAuthStore((s) => s.user?.id);
  const agents = useAgentStore((s) => s.agents);

  let displayName = message.username || '用户';
  let avatarSrc: string | undefined;

  if (message.role === 'assistant' && message.artifacts_json) {
    try {
      const meta = JSON.parse(message.artifacts_json) as { agent_name?: string; agent_id?: string };
      if (meta.agent_name) {
        displayName = meta.agent_name;
        const storeAgent = meta.agent_id ? agents.find((a) => a.id === meta.agent_id) : undefined;
        avatarSrc = storeAgent
          ? resolveAgentAvatar(storeAgent)
          : resolveAgentAvatar({ id: meta.agent_id ?? meta.agent_name, name: meta.agent_name });
      }
    } catch { /* ignore */ }
  } else if (isOwn) {
    const me = useAuthStore.getState().user;
    displayName = me?.username || '我';
    avatarSrc = me ? resolveUserAvatar(me) : undefined;
  } else if (message.sender_id) {
    avatarSrc = resolveUserAvatar({ id: message.sender_id, username: message.username });
  }

  const avatarLetter = displayName.charAt(0).toUpperCase();

  return (
    <div className={styles.originalMessage}>
      <div className={styles.originalHeader}>
        <Avatar size={32} src={avatarSrc}>
          {avatarLetter}
        </Avatar>
        <div className={styles.originalMeta}>
          <span className={styles.originalName}>{escapeHtml(displayName)}</span>
          <span className={styles.originalTime}>{formatTimestamp(message.created_at)}</span>
        </div>
      </div>
      <div className={styles.originalContent}>
        {message.content || ''}
      </div>
    </div>
  );
};

const ReplyBubble: React.FC<{ message: Message }> = ({ message }) => {
  const isOwn = message.sender_id === useAuthStore((s) => s.user?.id) || (!message.sender_id && message.role === 'user');
  const agents = useAgentStore((s) => s.agents);

  let displayName = message.username || (message.role === 'user' ? '用户' : '助手');
  let avatarSrc: string | undefined;

  if (message.role === 'assistant' && message.artifacts_json) {
    try {
      const meta = JSON.parse(message.artifacts_json) as { agent_name?: string; agent_id?: string };
      if (meta.agent_name) {
        displayName = meta.agent_name;
        const storeAgent = meta.agent_id ? agents.find((a) => a.id === meta.agent_id) : undefined;
        avatarSrc = storeAgent
          ? resolveAgentAvatar(storeAgent)
          : resolveAgentAvatar({ id: meta.agent_id ?? meta.agent_name, name: meta.agent_name });
      }
    } catch { /* ignore */ }
  } else if (isOwn) {
    const me = useAuthStore.getState().user;
    avatarSrc = me ? resolveUserAvatar(me) : undefined;
  } else {
    avatarSrc = resolveUserAvatar({ id: message.sender_id, username: message.username });
  }

  const avatarLetter = displayName ? displayName.charAt(0).toUpperCase() : '?';
  const avatarIcon = message.role === 'assistant' ? <RobotOutlined /> : <UserOutlined />;

  return (
    <div className={`${styles.replyBubble} ${isOwn ? styles.replyBubbleOwn : ''}`}>
      <div className={styles.replyHeader}>
        <Avatar size={28} src={avatarSrc} icon={avatarIcon}>
          {avatarLetter}
        </Avatar>
        <div className={styles.replyMeta}>
          <span className={styles.replyName}>{escapeHtml(displayName)}</span>
          <span className={styles.replyTime}>{formatTimestamp(message.created_at)}</span>
        </div>
      </div>
      <div className={styles.replyContent}>
        {message.content || ''}
      </div>
    </div>
  );
};
