import React, { useState } from 'react';
import { Avatar, Modal } from 'antd';
import { message } from '@/utils/message';
import { UserOutlined } from '@ant-design/icons';
import { useAuthStore } from '@/store/authStore';
import {
  USER_AVATAR_KEYS,
  avatarUrl,
  resolveUserAvatar,
} from '@/components/agent/agentPresentation';
import styles from '@/components/agent/AvatarPickerModal.module.css';

interface UserAvatarPickerModalProps {
  open: boolean;
  onClose: () => void;
  /** Custom save handler. When provided, the modal calls this instead of useAuthStore.updateAvatar. */
  onSelect?: (key: string) => Promise<void>;
}

export const UserAvatarPickerModal: React.FC<UserAvatarPickerModalProps> = ({ open, onClose, onSelect }) => {
  const user = useAuthStore((s) => s.user);
  const updateAvatar = useAuthStore((s) => s.updateAvatar);
  const [saving, setSaving] = useState(false);

  const currentResolved = user ? resolveUserAvatar(user) : '';

  const handleSelect = async (key: string) => {
    if (saving) return;
    setSaving(true);
    try {
      if (onSelect) {
        await onSelect(key);
        message.success('头像已更新');
      } else {
        await updateAvatar(key);
        message.success('头像已更新');
      }
      onClose();
    } catch {
      message.error('更新头像失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title="选择头像"
      open={open}
      onCancel={onClose}
      footer={null}
      width={480}
      destroyOnHidden
    >
      <div className={styles.grid}>
        {USER_AVATAR_KEYS.map((key) => {
          const url = avatarUrl(key);
          const isActive = url === currentResolved;
          return (
            <button
              key={key}
              type="button"
              className={`${styles.cell} ${isActive ? styles.cellActive : ''}`}
              disabled={saving}
              onClick={() => void handleSelect(key)}
              title={key}
            >
              <Avatar size={56} src={url} icon={<UserOutlined />} />
            </button>
          );
        })}
      </div>
    </Modal>
  );
};
