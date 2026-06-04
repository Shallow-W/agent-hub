import React, { useState, useMemo } from 'react';
import { Modal, Input, List, Avatar, Empty, Spin, message as antMessage } from 'antd';
import { SearchOutlined, SendOutlined } from '@ant-design/icons';
import { useConversationStore } from '@/store/conversationStore';
import { useMessageStore } from '@/store/messageStore';
import { useAuthStore } from '@/store/authStore';
import type { Message } from '@/types/message';
import styles from './ForwardModal.module.css';

interface ForwardModalProps {
  open: boolean;
  onClose: () => void;
  message: Message | null;
  currentConversationId?: string;
}

export const ForwardModal: React.FC<ForwardModalProps> = ({
  open,
  onClose,
  message,
  currentConversationId,
}) => {
  const [search, setSearch] = useState('');
  const [sending, setSending] = useState<Record<string, boolean>>({});
  const conversations = useConversationStore((s) => s.conversations);
  const sendMessage = useMessageStore((s) => s.sendMessage);
  const currentUser = useAuthStore((s) => s.user);

  const candidates = useMemo(() => {
    // Exclude current conversation and sort by updated_at desc
    return conversations
      .filter((c) => c.id !== currentConversationId)
      .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
  }, [conversations, currentConversationId]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return candidates;
    return candidates.filter((c) => c.title.toLowerCase().includes(q));
  }, [candidates, search]);

  const handleForward = async (conversationId: string) => {
    if (!message) return;
    const senderName = message.username || currentUser?.username || '用户';
    const forwardContent = `[转发自 ${senderName}]: ${message.content || ''}`;

    setSending((prev) => ({ ...prev, [conversationId]: true }));
    try {
      await sendMessage(
        conversationId,
        forwardContent,
        message.attachments?.map((a) => ({
          file_name: a.file_name,
          mime_type: a.mime_type,
          file_size: a.file_size,
          file_path: a.file_path,
          thumbnail_path: a.thumbnail_path,
          width: a.width,
          height: a.height,
        })),
        undefined,
        undefined,
        undefined,
        undefined,
      );
      antMessage.success('转发成功');
    } catch {
      antMessage.error('转发失败');
    } finally {
      setSending((prev) => ({ ...prev, [conversationId]: false }));
    }
  };

  const handleClose = () => {
    setSearch('');
    onClose();
  };

  return (
    <Modal
      title="转发消息"
      open={open}
      onCancel={handleClose}
      footer={null}
      width={480}
      className={styles.modal}
      destroyOnClose
    >
      <div className={styles.preview}>
        <span className={styles.previewLabel}>转发内容：</span>
        <span className={styles.previewText}>
          {message?.content
            ? message.content.length > 80
              ? message.content.slice(0, 80) + '...'
              : message.content
            : '(空消息)'}
        </span>
      </div>

      <Input
        prefix={<SearchOutlined className={styles.searchIcon} />}
        placeholder="搜索对话..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        allowClear
        className={styles.searchInput}
      />

      <div className={styles.listWrapper}>
        {filtered.length === 0 ? (
          <Empty
            description={search ? '未找到匹配的对话' : '没有可转发的对话'}
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            className={styles.empty}
          />
        ) : (
          <List
            dataSource={filtered}
            renderItem={(conv) => {
              const isSending = sending[conv.id];
              return (
                <div
                  className={styles.item}
                  role="button"
                  tabIndex={0}
                  onClick={() => !isSending && handleForward(conv.id)}
                  onKeyDown={(e) => {
                    if ((e.key === 'Enter' || e.key === ' ') && !isSending) {
                      e.preventDefault();
                      handleForward(conv.id);
                    }
                  }}
                >
                  <Avatar size={36} className={styles.itemAvatar}>
                    {(conv.peer_name || conv.title).charAt(0).toUpperCase()}
                  </Avatar>
                  <div className={styles.itemBody}>
                    <span className={styles.itemTitle}>{conv.title}</span>
                    {conv.last_message && (
                      <span className={styles.itemSubtitle}>
                        {conv.last_message.length > 30
                          ? conv.last_message.slice(0, 30) + '...'
                          : conv.last_message}
                      </span>
                    )}
                  </div>
                  {isSending ? (
                    <Spin size="small" />
                  ) : (
                    <SendOutlined className={styles.sendIcon} />
                  )}
                </div>
              );
            }}
          />
        )}
      </div>
    </Modal>
  );
};
