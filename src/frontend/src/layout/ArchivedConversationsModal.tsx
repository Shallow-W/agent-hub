import React from 'react';
import { Avatar, Button, Modal } from 'antd';
import { UndoOutlined } from '@ant-design/icons';
import type { Conversation } from '@/types/conversation';
import styles from './ArchivedConversationsModal.module.css';

interface ArchivedConversationsModalProps {
  open: boolean;
  conversations: Conversation[];
  onCancel: () => void;
  onUnarchive: (conversationId: string) => void;
}

const ArchivedConversationsModal: React.FC<ArchivedConversationsModalProps> = ({
  open,
  conversations,
  onCancel,
  onUnarchive,
}) => (
  <Modal
    title="归档对话"
    open={open}
    onCancel={onCancel}
    footer={null}
    width={400}
  >
    {conversations.length === 0 ? (
      <div className={styles.empty}>暂无归档对话</div>
    ) : (
      <div className={styles.list}>
        {conversations.map((conv) => (
          <div key={conv.id} className={styles.item}>
            <Avatar size={28}>{(conv.title || '?').charAt(0).toUpperCase()}</Avatar>
            <span className={styles.title}>{conv.title || '未命名'}</span>
            <span className={styles.date}>
              {new Date(conv.created_at).toLocaleDateString('zh-CN')}
            </span>
            <Button
              type="link"
              size="small"
              icon={<UndoOutlined />}
              onClick={() => onUnarchive(conv.id)}
            >
              取消归档
            </Button>
          </div>
        ))}
      </div>
    )}
  </Modal>
);

export default ArchivedConversationsModal;
