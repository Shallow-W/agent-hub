import React, { useEffect, useState, useCallback } from 'react';
import {
  Drawer,
  List,
  Avatar,
  Tag,
  Empty,
  Spin,
  message,
  Input,
} from 'antd';
import {
  TeamOutlined,
  TagsOutlined,
  PlusOutlined,
  CameraOutlined,
} from '@ant-design/icons';
import { getGroupInfo, updateGroupInfo } from '@/api/group';
import type { GroupInfo } from '@/api/group';
import { resolveUserAvatar, avatarUrl } from '@/components/agent/agentPresentation';
import { EditableProfileCard } from '@/components/common/EditableProfileCard';
import { GroupAvatarPicker } from '@/components/groups/GroupAvatarPicker';
import styles from './GroupInfoDrawer.module.css';

interface GroupInfoDrawerProps {
  open: boolean;
  onClose: () => void;
  conversationId: string;
  currentUserId: string;
}

const ROLE_COLORS: Record<string, string> = {
  owner: 'gold',
  admin: 'blue',
  member: 'default',
};

const ROLE_LABELS: Record<string, string> = {
  owner: '群主',
  admin: '管理员',
  member: '成员',
};

const TAG_COLORS = [
  'blue', 'geekblue', 'purple', 'magenta', 'volcano',
  'orange', 'gold', 'green', 'cyan', 'red',
];

function parseTags(raw?: string): string[] {
  if (!raw || raw === '[]') return [];
  try {
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr.filter((t): t is string => typeof t === 'string') : [];
  } catch {
    return [];
  }
}

const GroupInfoDrawer: React.FC<GroupInfoDrawerProps> = ({
  open,
  onClose,
  conversationId,
  currentUserId,
}) => {
  const [info, setInfo] = useState<GroupInfo | null>(null);
  const [loading, setLoading] = useState(false);

  // Tag editing state
  const [tagInputVisible, setTagInputVisible] = useState(false);
  const [tagInputValue, setTagInputValue] = useState('');

  // Group avatar picker state
  const [avatarPickerOpen, setAvatarPickerOpen] = useState(false);

  const fetchInfo = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getGroupInfo(conversationId);
      setInfo(data);
    } catch {
      message.error('获取群聊信息失败');
    } finally {
      setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => {
    if (open && conversationId) {
      fetchInfo();
      setTagInputVisible(false);
    }
  }, [open, conversationId, fetchInfo]);

  const currentMember = info?.members?.find((m) => m.user_id === currentUserId);
  const canEdit = currentMember?.role === 'owner' || currentMember?.role === 'admin';

  const handleFieldSave = useCallback(async (key: string, value: string) => {
    await updateGroupInfo(conversationId, { [key]: value });
    await fetchInfo();
  }, [conversationId, fetchInfo]);

  const handleAvatarChange = useCallback(async (avatarKey: string) => {
    await updateGroupInfo(conversationId, { avatar: avatarKey });
    message.success('群头像已更新');
    await fetchInfo();
  }, [conversationId, fetchInfo]);

  // Tag handlers
  const handleAddTag = async () => {
    const val = tagInputValue.trim();
    if (!val) return;
    if (val.length > 20) {
      message.warning('标签不能超过 20 个字符');
      return;
    }
    const current = parseTags(info?.conversation?.tags);
    if (current.length >= 10) {
      message.warning('标签数量不能超过 10 个');
      return;
    }
    if (current.includes(val)) {
      message.warning('标签已存在');
      return;
    }
    const next = [...current, val];
    try {
      await updateGroupInfo(conversationId, { tags: JSON.stringify(next) });
      setTagInputValue('');
      setTagInputVisible(false);
      await fetchInfo();
    } catch {
      message.error('添加标签失败');
    }
  };

  const handleRemoveTag = async (tag: string) => {
    const current = parseTags(info?.conversation?.tags);
    const next = current.filter((t) => t !== tag);
    try {
      await updateGroupInfo(conversationId, { tags: JSON.stringify(next) });
      await fetchInfo();
    } catch {
      message.error('移除标签失败');
    }
  };

  const groupAvatarSrc = info?.conversation?.avatar
    ? (() => {
        const av = info.conversation.avatar;
        if (/^(https?:|data:|\/)/i.test(av)) return av;
        return avatarUrl(av);
      })()
    : undefined;

  /** Extract the group avatar key (e.g. "group-3") from the raw avatar value, or undefined. */
  const currentGroupAvatarKey = (() => {
    const raw = info?.conversation?.avatar?.trim();
    if (!raw) return undefined;
    if (/^group-\d+$/.test(raw)) return raw;
    return undefined;
  })();

  const tags = parseTags(info?.conversation?.tags);

  return (
    <Drawer
      title="群聊信息"
      open={open}
      onClose={onClose}
      width={340}
    >
      <Spin spinning={loading}>
        {info ? (
          <>
            {/* 群资料卡片 */}
            <div className={styles.card}>
              {/* Group avatar — dedicated picker */}
              <div
                className={`${styles.groupAvatarWrapper} ${canEdit ? styles.groupAvatarEditable : ''}`}
                onClick={() => canEdit && setAvatarPickerOpen(true)}
                role={canEdit ? 'button' : undefined}
                tabIndex={canEdit ? 0 : undefined}
                aria-label="更换群头像"
              >
                <Avatar
                  size={64}
                  src={groupAvatarSrc}
                  icon={<TeamOutlined />}
                  className={styles.groupAvatar}
                >
                  {info.conversation?.title?.charAt(0) || 'G'}
                </Avatar>
                {canEdit && (
                  <div className={styles.groupAvatarOverlay}>
                    <CameraOutlined />
                  </div>
                )}
              </div>

              <EditableProfileCard
                avatarSrc={undefined}
                avatarFallback={undefined}
                avatarEditable={false}
                fields={[
                  { key: 'title', label: '群名', value: info.conversation?.title || '', maxLength: 50, placeholder: '输入群名称' },
                  { key: 'description', label: '简介', value: info.conversation?.description || '', type: 'textarea', maxLength: 200, placeholder: '添加群简介...' },
                  { key: 'announcement', label: '群公告', value: info.conversation?.announcement || '', type: 'textarea', maxLength: 500, placeholder: '发布群公告...' },
                ]}
                onFieldSave={handleFieldSave}
                canEdit={canEdit}
              />
            </div>

            <GroupAvatarPicker
              open={avatarPickerOpen}
              onClose={() => setAvatarPickerOpen(false)}
              currentKey={currentGroupAvatarKey}
              onSelect={handleAvatarChange}
            />

            {/* 群标签卡片 */}
            <div className={styles.card}>
              <div className={styles.sectionTitle}>
                <TagsOutlined /> 群标签
              </div>
              <div className={styles.tagsList}>
                {tags.map((tag, i) => (
                  <Tag
                    key={tag}
                    color={TAG_COLORS[i % TAG_COLORS.length]}
                    closable={canEdit}
                    onClose={(e) => { e.preventDefault(); handleRemoveTag(tag); }}
                  >
                    {tag}
                  </Tag>
                ))}
                {canEdit && !tagInputVisible && tags.length < 10 && (
                  <Tag
                    style={{ borderStyle: 'dashed', cursor: 'pointer' }}
                    onClick={() => setTagInputVisible(true)}
                  >
                    <PlusOutlined /> 添加
                  </Tag>
                )}
                {canEdit && tagInputVisible && (
                  <Input
                    className={styles.tagInput}
                    size="small"
                    autoFocus
                    value={tagInputValue}
                    onChange={(e) => setTagInputValue(e.target.value)}
                    onBlur={() => { setTagInputVisible(false); setTagInputValue(''); }}
                    onPressEnter={handleAddTag}
                    maxLength={20}
                  />
                )}
              </div>
            </div>

            {/* 成员列表卡片 */}
            <div className={styles.card}>
              <div className={styles.sectionTitle}>
                <TeamOutlined /> 成员列表
              </div>

              {!info.members?.length ? (
                <Empty description="暂无成员" image={Empty.PRESENTED_IMAGE_SIMPLE} />
              ) : (
                <List
                  dataSource={info.members}
                  renderItem={(member) => (
                    <List.Item>
                      <List.Item.Meta
                        avatar={
                          <Avatar
                            size="small"
                            src={resolveUserAvatar({ id: member.user_id, username: member.username })}
                            style={{ backgroundColor: '#1677ff' }}
                          >
                            {(member.username ?? '?').charAt(0).toUpperCase()}
                          </Avatar>
                        }
                        title={
                          <span>
                            {member.username ?? '未知用户'}
                          </span>
                        }
                        description={
                          <Tag color={ROLE_COLORS[member.role] ?? 'default'} style={{ fontSize: 11 }}>
                            {ROLE_LABELS[member.role] ?? member.role}
                          </Tag>
                        }
                      />
                    </List.Item>
                  )}
                />
              )}
            </div>
          </>
        ) : (
          !loading && <Empty description="暂无群聊信息" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}
      </Spin>
    </Drawer>
  );
};

export default GroupInfoDrawer;
