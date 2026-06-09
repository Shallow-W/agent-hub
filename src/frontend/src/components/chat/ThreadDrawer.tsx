import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Avatar, Drawer, Spin, Input, Button } from 'antd';
import { SendOutlined, RobotOutlined, UserOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
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

  useEffect(() => {
    if (!open || !originalMessage) return;
    setLoading(true);
    getMessageReplies(conversationId, originalMessage.id)
      .then((data) => setReplies(data))
      .catch(() => antMessage.error('加载回复失败'))
      .finally(() => setLoading(false));
  }, [open, originalMessage, conversationId]);

  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [replies]);

  const handleClose = () => {
    setReplies([]);
    setInputValue('');
    onClose();
  };

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

  return (
    <Drawer
      title={originalMessage ? `回复 ${escapeHtml(resolveName(originalMessage))}` : ''}
      open={open}
      onClose={handleClose}
      width={520}
      placement="right"
      className={styles.drawer}
      destroyOnClose
    >
      {originalMessage && (
        <>
          <ThreadBubble message={originalMessage} isOriginal />
          <div className={styles.divider}>
            <span>{replies.length} 条回复</span>
          </div>
        </>
      )}
      <div className={styles.replyList} ref={listRef}>
        {loading ? (
          <div className={styles.loadingArea}>
            <Spin size="small" />
          </div>
        ) : replies.length === 0 ? (
          <div className={styles.emptyReplies}>暂无回复</div>
        ) : (
          replies.map((msg) => <ThreadBubble key={msg.id} message={msg} />)
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

function resolveName(message: Message): string {
  if (message.role === 'assistant' && message.artifacts_json) {
    try {
      const meta = JSON.parse(message.artifacts_json) as { agent_name?: string };
      if (meta.agent_name) return meta.agent_name;
    } catch { /* ignore */ }
  }
  return message.username || (message.sender_id ? '用户' : '助手');
}

function resolveAvatar(message: Message, isOwn: boolean): { src?: string; letter: string; icon: React.ReactNode } {
  const agents = useAgentStore.getState().agents;

  if (message.role === 'assistant' && message.artifacts_json) {
    try {
      const meta = JSON.parse(message.artifacts_json) as { agent_name?: string; agent_id?: string };
      if (meta.agent_name) {
        const storeAgent = meta.agent_id ? agents.find((a) => a.id === meta.agent_id) : undefined;
        const src = storeAgent
          ? resolveAgentAvatar(storeAgent)
          : resolveAgentAvatar({ id: meta.agent_id ?? meta.agent_name, name: meta.agent_name });
        return { src, letter: meta.agent_name.charAt(0).toUpperCase(), icon: <RobotOutlined /> };
      }
    } catch { /* ignore */ }
  }

  if (isOwn) {
    const me = useAuthStore.getState().user;
    return { src: me ? resolveUserAvatar(me) : undefined, letter: (me?.username || '我').charAt(0).toUpperCase(), icon: <UserOutlined /> };
  }

  if (message.sender_id) {
    const src = resolveUserAvatar({ id: message.sender_id, username: message.username });
    return { src, letter: (message.username || '用').charAt(0).toUpperCase(), icon: <UserOutlined /> };
  }

  return { src: undefined, letter: '?', icon: <RobotOutlined /> };
}

const ThreadBubble: React.FC<{ message: Message; isOriginal?: boolean }> = ({ message, isOriginal }) => {
  const userId = useAuthStore((s) => s.user?.id);
  const isOwn = message.sender_id === userId || (!message.sender_id && message.role === 'user');
  const name = resolveName(message);
  const avatar = resolveAvatar(message, isOwn);
  const time = formatTime(message.created_at);

  return (
    <div className={`${styles.bubble} ${isOwn ? styles.bubbleRight : styles.bubbleLeft} ${isOriginal ? styles.bubbleOriginal : ''}`}>
      <Avatar
        size={isOriginal ? 34 : 30}
        src={avatar.src}
        icon={avatar.icon}
        className={styles.avatar}
      >
        {avatar.letter}
      </Avatar>
      <div className={styles.bubbleBody}>
        <div className={styles.bubbleMeta}>
          <span className={styles.bubbleName}>{escapeHtml(name)}</span>
          <span className={styles.bubbleTime}>{time}</span>
        </div>
        <div className={`${styles.bubbleInner} ${isOwn ? styles.innerOwn : styles.innerOther}`}>
          <div className={styles.markdownBody}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {message.content || ''}
            </ReactMarkdown>
          </div>
        </div>
      </div>
    </div>
  );
};

function formatTime(dateStr: string): string {
  const d = new Date(dateStr);
  const now = new Date();
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  const isToday = d.toDateString() === now.toDateString();
  if (isToday) return `${hh}:${mm}`;
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${month}-${day} ${hh}:${mm}`;
}
