import React, { useState } from 'react';
import { Avatar, Modal, message } from 'antd';
import { RobotOutlined } from '@ant-design/icons';
import type { Agent } from '@/types/agent';
import { useAgentStore } from '@/store/agentStore';
import { ALL_AVATAR_KEYS, avatarUrl, resolveAgentAvatar } from './agentPresentation';
import styles from './AvatarPickerModal.module.css';

interface AvatarPickerModalProps {
  agent: Agent | null;
  open: boolean;
  onClose: () => void;
}

export const AvatarPickerModal: React.FC<AvatarPickerModalProps> = ({ agent, open, onClose }) => {
  const updateAgentAvatar = useAgentStore((s) => s.updateAgentAvatar);
  const [saving, setSaving] = useState(false);

  if (!agent) return null;

  const currentResolved = resolveAgentAvatar(agent);

  const handleSelect = async (key: string) => {
    if (saving) return;
    setSaving(true);
    try {
      await updateAgentAvatar(agent.id, key);
      message.success('头像已更新');
      onClose();
    } catch {
      message.error('更新头像失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title="选择 Agent 头像"
      open={open}
      onCancel={onClose}
      footer={null}
      width={480}
      destroyOnClose
    >
      <div className={styles.grid}>
        {ALL_AVATAR_KEYS.map((key) => {
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
              <Avatar size={56} src={url} icon={<RobotOutlined />} />
            </button>
          );
        })}
      </div>
    </Modal>
  );
};
