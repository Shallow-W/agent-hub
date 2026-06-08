import React, { useCallback, useState } from 'react';
import { Avatar, Button, Input, message } from 'antd';
import { UserOutlined, CameraOutlined, EditOutlined, CheckOutlined, CloseOutlined } from '@ant-design/icons';
import { UserAvatarPickerModal } from '@/components/settings/UserAvatarPickerModal';
import styles from './EditableProfileCard.module.css';

export interface EditableField {
  key: string;
  label: string;
  value: string;
  type?: 'text' | 'textarea';
  maxLength?: number;
  placeholder?: string;
}

export interface EditableProfileCardProps {
  /** Avatar URL (resolved) */
  avatarSrc?: string;
  /** Fallback character when no avatar */
  avatarFallback?: string;
  /** Whether avatar is editable */
  avatarEditable?: boolean;
  /** Callback when avatar key is selected */
  onAvatarChange?: (avatarKey: string) => Promise<void>;

  /** Editable fields */
  fields: EditableField[];
  /** Save handler for field changes */
  onFieldSave: (key: string, value: string) => Promise<void>;

  /** Whether current user can edit */
  canEdit: boolean;

  /** Optional extra content below fields */
  children?: React.ReactNode;
}

export const EditableProfileCard: React.FC<EditableProfileCardProps> = ({
  avatarSrc,
  avatarFallback,
  avatarEditable = false,
  onAvatarChange,
  fields,
  onFieldSave,
  canEdit,
  children,
}) => {
  const [avatarPickerOpen, setAvatarPickerOpen] = useState(false);
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');
  const [saving, setSaving] = useState(false);

  const startEdit = useCallback((field: EditableField) => {
    setEditValue(field.value);
    setEditingKey(field.key);
  }, []);

  const cancelEdit = useCallback(() => {
    setEditingKey(null);
    setEditValue('');
  }, []);

  const saveField = useCallback(async (field: EditableField) => {
    const trimmed = editValue.trim();
    if (field.type !== 'textarea' && !trimmed) {
      message.warning(`${field.label}不能为空`);
      return;
    }
    if (trimmed === field.value) {
      cancelEdit();
      return;
    }
    setSaving(true);
    try {
      await onFieldSave(field.key, trimmed);
      message.success(`${field.label}已更新`);
      cancelEdit();
    } catch (err) {
      const msg = err instanceof Error ? err.message : `更新${field.label}失败`;
      message.error(msg);
    } finally {
      setSaving(false);
    }
  }, [editValue, onFieldSave, cancelEdit]);

  const handleAvatarClose = useCallback(() => {
    setAvatarPickerOpen(false);
  }, []);

  return (
    <div className={styles.card}>
      {/* Avatar */}
      {(avatarSrc || avatarFallback) && (
        <div
          className={`${styles.avatarWrapper} ${canEdit && avatarEditable ? styles.avatarEditable : ''}`}
          onClick={() => canEdit && avatarEditable && setAvatarPickerOpen(true)}
          role={canEdit && avatarEditable ? 'button' : undefined}
          tabIndex={canEdit && avatarEditable ? 0 : undefined}
          aria-label={avatarEditable ? '更换头像' : undefined}
        >
          <Avatar size={52} src={avatarSrc} icon={<UserOutlined />} className={styles.avatar}>
            {avatarFallback}
          </Avatar>
          {canEdit && avatarEditable && (
            <div className={styles.avatarOverlay}>
              <CameraOutlined />
            </div>
          )}
        </div>
      )}

      {/* Fields */}
      <div className={styles.fields}>
        {fields.map((field) => (
          <div key={field.key} className={styles.fieldRow}>
            <span className={styles.fieldLabel}>{field.label}</span>
            {editingKey === field.key ? (
              <div className={styles.editRow}>
                {field.type === 'textarea' ? (
                  <Input.TextArea
                    size="small"
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    disabled={saving}
                    className={styles.editInput}
                    maxLength={field.maxLength}
                    placeholder={field.placeholder}
                    autoSize={{ minRows: 2, maxRows: 4 }}
                  />
                ) : (
                  <Input
                    size="small"
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    onPressEnter={() => void saveField(field)}
                    disabled={saving}
                    className={styles.editInput}
                    maxLength={field.maxLength}
                    placeholder={field.placeholder}
                  />
                )}
                <Button type="text" size="small" icon={<CheckOutlined />} loading={saving} onClick={() => void saveField(field)} aria-label="保存" />
                <Button type="text" size="small" icon={<CloseOutlined />} onClick={cancelEdit} disabled={saving} aria-label="取消" />
              </div>
            ) : (
              <div className={styles.displayRow}>
                <span className={styles.fieldValue}>
                  {field.value || <span className={styles.fieldPlaceholder}>{field.placeholder || '未设置'}</span>}
                </span>
                {canEdit && (
                  <Button type="text" size="small" icon={<EditOutlined />} onClick={() => startEdit(field)} aria-label={`编辑${field.label}`} className={styles.editBtn} />
                )}
              </div>
            )}
          </div>
        ))}
      </div>

      {children}

      {avatarEditable && (
        <UserAvatarPickerModal
          open={avatarPickerOpen}
          onClose={handleAvatarClose}
          onSelect={onAvatarChange}
        />
      )}
    </div>
  );
};
