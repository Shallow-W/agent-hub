import React, { useEffect, useState, useCallback } from 'react';
import {
  Drawer,
  List,
  Avatar,
  Button,
  Badge,
  Popconfirm,
  message,
  Empty,
  Spin,
  Tag,
  Input,
} from 'antd';
import {
  UserAddOutlined,
  DeleteOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import { getGroupMembers, removeGroupMember, leaveGroup, addGroupMember } from '@/api/group';
import type { GroupMember } from '@/types/group';
import { useFriendStore } from '@/store/friendStore';
import { Checkbox } from 'antd';

interface GroupMemberPanelProps {
  open: boolean;
  onClose: () => void;
  conversationId: string;
  currentUserId: string;
  onGroupLeft?: () => void;
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

const GroupMemberPanel: React.FC<GroupMemberPanelProps> = ({
  open,
  onClose,
  conversationId,
  currentUserId,
  onGroupLeft,
}) => {
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [inviteSearch, setInviteSearch] = useState('');
  const [selectedFriends, setSelectedFriends] = useState<string[]>([]);
  const [inviteLoading, setInviteLoading] = useState(false);
  const { friends } = useFriendStore();

  const fetchMembers = useCallback(async () => {
    setLoading(true);
    try {
      const list = await getGroupMembers(conversationId);
      setMembers(list);
    } catch {
      message.error('获取群成员失败');
    } finally {
      setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => {
    if (open && conversationId) {
      fetchMembers();
    }
  }, [open, conversationId, fetchMembers]);

  const currentUserRole = members.find((m) => m.user_id === currentUserId)?.role;
  const canManage = currentUserRole === 'owner' || currentUserRole === 'admin';

  const handleRemove = async (userId: string) => {
    setActionLoading(userId);
    try {
      await removeGroupMember(conversationId, userId);
      message.success('已移除成员');
      await fetchMembers();
    } catch {
      message.error('移除成员失败');
    } finally {
      setActionLoading(null);
    }
  };

  const handleLeave = async () => {
    setActionLoading('leave');
    try {
      await leaveGroup(conversationId);
      message.success('已退出群聊');
      onGroupLeft?.();
      onClose();
    } catch {
      message.error('退出群聊失败');
    } finally {
      setActionLoading(null);
    }
  };

  const handleInvite = async () => {
    if (selectedFriends.length === 0) return;
    setInviteLoading(true);
    try {
      const results = await Promise.allSettled(
        selectedFriends.map((friendId) =>
          addGroupMember(conversationId, { user_id: friendId, role: 'member' }),
        ),
      );
      const failed = results.filter((r) => r.status === 'rejected');
      if (failed.length > 0) {
        message.warning(
          `${selectedFriends.length - failed.length} 人邀请成功，${failed.length} 人失败`,
        );
      } else {
        message.success('已邀请成员');
      }
      setSelectedFriends([]);
      setInviteOpen(false);
      await fetchMembers();
    } catch {
      message.error('邀请成员失败');
    } finally {
      setInviteLoading(false);
    }
  };

  const memberIds = new Set(members.map((m) => m.user_id));
  const availableFriends = friends.filter((f) => !memberIds.has(f.friend_id));
  const filteredInviteFriends = inviteSearch
    ? availableFriends.filter((f) =>
        (f.friend_name ?? '').toLowerCase().includes(inviteSearch.toLowerCase()),
      )
    : availableFriends;

  return (
    <Drawer
      title="群成员"
      open={open}
      onClose={onClose}
      width={320}
      extra={
        canManage ? (
          <Button
            type="primary"
            size="small"
            icon={<UserAddOutlined />}
            onClick={() => setInviteOpen(true)}
          >
            邀请成员
          </Button>
        ) : undefined
      }
    >
      <Spin spinning={loading}>
        {members.length === 0 ? (
          <Empty description="暂无成员" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <List
            dataSource={members}
            renderItem={(member) => (
              <List.Item
                actions={[
                  ...(canManage && member.user_id !== currentUserId && member.role !== 'owner'
                    ? [
                        <Popconfirm
                          key="remove"
                          title="确定移除该成员？"
                          onConfirm={() => handleRemove(member.user_id)}
                          okText="确定"
                          cancelText="取消"
                        >
                          <Button
                            type="text"
                            danger
                            size="small"
                            icon={<DeleteOutlined />}
                            loading={actionLoading === member.user_id}
                          />
                        </Popconfirm>,
                      ]
                    : []),
                ]}
              >
                <List.Item.Meta
                  avatar={
                    <Badge
                      dot
                      color="green"
                      offset={[-4, 30]}
                    >
                      <Avatar size="small" style={{ backgroundColor: '#1677ff' }}>
                        {(member.username ?? '?').charAt(0).toUpperCase()}
                      </Avatar>
                    </Badge>
                  }
                  title={
                    <span>
                      {member.username ?? '未知用户'}
                      {member.user_id === currentUserId && (
                        <span style={{ fontSize: 11, color: '#999', marginLeft: 4 }}>(我)</span>
                      )}
                    </span>
                  }
                  description={
                    <Tag color={ROLE_COLORS[member.role]} style={{ fontSize: 11 }}>
                      {ROLE_LABELS[member.role]}
                    </Tag>
                  }
                />
              </List.Item>
            )}
          />
        )}
      </Spin>

      {currentUserRole !== 'owner' && (
        <div style={{ marginTop: 16, borderTop: '1px solid var(--color-border)', paddingTop: 16 }}>
          <Popconfirm
            title="确定退出该群聊？"
            onConfirm={handleLeave}
            okText="确定"
            cancelText="取消"
          >
            <Button danger block icon={<LogoutOutlined />} loading={actionLoading === 'leave'}>
              退出群聊
            </Button>
          </Popconfirm>
        </div>
      )}

      {/* Invite members drawer */}
      <Drawer
        title="邀请成员"
        open={inviteOpen}
        onClose={() => {
          setInviteOpen(false);
          setSelectedFriends([]);
          setInviteSearch('');
        }}
        width={280}
      >
        <Input.Search
          placeholder="搜索好友..."
          allowClear
          value={inviteSearch}
          onChange={(e) => setInviteSearch(e.target.value)}
          onClear={() => setInviteSearch('')}
          style={{ marginBottom: 12 }}
        />
        {filteredInviteFriends.length === 0 ? (
          <Empty description="没有可邀请的好友" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <Checkbox.Group
            value={selectedFriends}
            onChange={(vals) => setSelectedFriends(vals as string[])}
            style={{ display: 'flex', flexDirection: 'column', gap: 8 }}
          >
            {filteredInviteFriends.map((f) => (
              <Checkbox key={f.friend_id} value={f.friend_id}>
                {f.friend_name ?? '未知用户'}
              </Checkbox>
            ))}
          </Checkbox.Group>
        )}
        {selectedFriends.length > 0 && (
          <Button
            type="primary"
            block
            loading={inviteLoading}
            onClick={handleInvite}
            style={{ marginTop: 16 }}
          >
            邀请 ({selectedFriends.length})
          </Button>
        )}
      </Drawer>
    </Drawer>
  );
};

export default GroupMemberPanel;
