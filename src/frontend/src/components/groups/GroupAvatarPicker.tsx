import React from 'react';
import { Avatar, Modal } from 'antd';
import { TeamOutlined } from '@ant-design/icons';
import { GROUP_AVATAR_KEYS, avatarUrl } from '@/components/agent/agentPresentation';
import styles from './GroupAvatarPicker.module.css';

interface GroupAvatarPickerProps {
  open: boolean;
  onClose: () => void;
  /** Currently selected avatar key (e.g. "group-1"). */
  currentKey?: string;
  /** Called when user picks an avatar key. */
  onSelect: (key: string) => Promise<void>;
}

export const GroupAvatarPicker: React.FC<GroupAvatarPickerProps> = ({
  open,
  onClose,
  currentKey,
  onSelect,
}) => {
  const [saving, setSaving] = React.useState(false);

  const handleSelect = async (key: string) => {
    if (saving) return;
    setSaving(true);
    try {
      await onSelect(key);
      onClose();
    } catch {
      // Let the caller handle the error message
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title="选择群头像"
      open={open}
      onCancel={onClose}
      footer={null}
      width={360}
      destroyOnClose
    >
      <div className={styles.grid}>
        {GROUP_AVATAR_KEYS.map((key) => {
          const url = avatarUrl(key);
          const isActive = key === currentKey;
          return (
            <button
              key={key}
              type="button"
              className={`${styles.cell} ${isActive ? styles.cellActive : ''}`}
              disabled={saving}
              onClick={() => void handleSelect(key)}
              title={key}
            >
              <Avatar size={56} src={url} icon={<TeamOutlined />} shape="circle" />
            </button>
          );
        })}
      </div>
    </Modal>
  );
};
