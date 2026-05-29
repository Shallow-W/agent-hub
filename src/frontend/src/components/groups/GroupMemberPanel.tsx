import React, { useEffect, useRef, useState, useCallback } from 'react';
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
  Tabs,
  Dropdown,
} from 'antd';
import {
  UserAddOutlined,
  DeleteOutlined,
  LogoutOutlined,
  MoreOutlined,
  TeamOutlined,
  UserOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { getGroupMembers, removeGroupMember, leaveGroup, addGroupMember, changeMemberRole } from '@/api/group';
import { getConversationAgents, addConversationAgent, removeConversationAgent } from '@/api/conversation';
import type { GroupMember } from '@/types/group';
import type { ConversationAgent } from '@/types/conversation';
import { useFriendStore } from '@/store/friendStore';
import { useAgentStore } from '@/store/agentStore';
import { searchUsers as searchUsersApi } from '@/api/friend';
import type { User } from '@/types/auth';
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
  const [userSearchResults, setUserSearchResults] = useState<User[]>([]);
  const [userSearchLoading, setUserSearchLoading] = useState(false);
  const [userSearchQuery, setUserSearchQuery] = useState('');
  const [addingUser, setAddingUser] = useState<string | null>(null);
  const userSearchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { friends } = useFriendStore();
  const agents = useAgentStore((s) => s.agents);
  const [agentMembers, setAgentMembers] = useState<ConversationAgent[]>([]);
  const [addingAgent, setAddingAgent] = useState<string | null>(null);

  const fetchMembers = useCallback(async () => {
    setLoading(true);
    try {
      const [list, agentList] = await Promise.all([
        getGroupMembers(conversationId),
        getConversationAgents(conversationId),
      ]);
      setMembers(list ?? []);
      setAgentMembers(agentList ?? []);
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

  const handleRoleChange = async (userId: string, role: string) => {
    setActionLoading(userId);
    try {
      await changeMemberRole(conversationId, userId, role);
      message.success(role === 'admin' ? '已设为管理员' : '已取消管理员');
      await fetchMembers();
    } catch {
      message.error('修改角色失败');
    } finally {
      setActionLoading(null);
    }
  };

  const buildMemberActions = (member: GroupMember): MenuProps['items'] => {
    const items: MenuProps['items'] = [];
    if (member.role === 'member') {
      items.push({
        key: 'set-admin',
        icon: <TeamOutlined />,
        label: '设为管理员',
        onClick: () => handleRoleChange(member.user_id, 'admin'),
      });
    } else if (member.role === 'admin') {
      items.push({
        key: 'set-member',
        icon: <UserOutlined />,
        label: '取消管理员',
        onClick: () => handleRoleChange(member.user_id, 'member'),
      });
    }
    items.push({
      key: 'remove',
      icon: <DeleteOutlined />,
      label: '移除成员',
      danger: true,
      onClick: () => handleRemove(member.user_id),
    });
    return items;
  };

  const handleLeave = async () => {
    setActionLoading('leave');
    try {
      await leaveGroup(conversationId);
      message.success('已退出群聊');
      setMembers([]);
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

  const handleAddUser = async (userId: string) => {
    setAddingUser(userId);
    try {
      await addGroupMember(conversationId, { user_id: userId, role: 'member' });
      message.success('已添加成员');
      setUserSearchResults((prev) => prev.filter((u) => u.id !== userId));
      await fetchMembers();
    } catch {
      message.error('添加成员失败');
    } finally {
      setAddingUser(null);
    }
  };

  const memberIdsRef = useRef(new Set<string>());
  memberIdsRef.current = new Set(members.map((m) => m.user_id));
  const availableFriends = friends.filter((f) => !memberIdsRef.current.has(f.friend_id));
  const filteredInviteFriends = inviteSearch
    ? availableFriends.filter((f) =>
        (f.friend_name ?? '').toLowerCase().includes(inviteSearch.toLowerCase()),
      )
    : availableFriends;

  const handleUserSearch = useCallback((value: string) => {
    setUserSearchQuery(value);
    if (userSearchTimer.current !== null) clearTimeout(userSearchTimer.current);
    if (!value.trim()) {
      setUserSearchResults([]);
      return;
    }
    userSearchTimer.current = setTimeout(async () => {
      setUserSearchLoading(true);
      try {
        const results = await searchUsersApi(value.trim());
        setUserSearchResults((results ?? []).filter((u) => !memberIdsRef.current.has(u.id)));
      } catch {
        setUserSearchResults([]);
      } finally {
        setUserSearchLoading(false);
      }
    }, 300);
  }, []);

  useEffect(() => {
    return () => {
      if (userSearchTimer.current !== null) clearTimeout(userSearchTimer.current);
    };
  }, []);

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
                  ...(currentUserRole === 'owner' &&
                  member.user_id !== currentUserId &&
                  member.role !== 'owner'
                    ? [
                        <Dropdown
                          key="actions"
                          menu={{ items: buildMemberActions(member) }}
                          trigger={['click']}
                        >
                          <Button
                            type="text"
                            size="small"
                            icon={<MoreOutlined />}
                            loading={actionLoading === member.user_id}
                            aria-label={`${member.username ?? '成员'} 的操作菜单`}
                          />
                        </Dropdown>,
                      ]
                    : canManage &&
                      member.user_id !== currentUserId &&
                      member.role !== 'owner'
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
                              aria-label={`移除 ${member.username ?? '成员'}`}
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

      {/* Agent 成员列表 */}
      {agentMembers.length > 0 && (
        <>
          <div style={{ marginTop: 12, marginBottom: 4, fontSize: 12, color: '#999' }}>
            <RobotOutlined /> 智能体
          </div>
          <List
            dataSource={agentMembers}
            renderItem={(agent) => (
              <List.Item
                actions={
                  canManage
                    ? [
                        <Popconfirm
                          key="remove-agent"
                          title="确定移除该智能体？"
                          onConfirm={async () => {
                            setActionLoading(agent.agent_id);
                            try {
                              await removeConversationAgent(conversationId, agent.agent_id);
                              message.success('已移除智能体');
                              await fetchMembers();
                            } catch {
                              message.error('移除智能体失败');
                            } finally {
                              setActionLoading(null);
                            }
                          }}
                          okText="确定"
                          cancelText="取消"
                        >
                          <Button
                            type="text"
                            danger
                            size="small"
                            icon={<DeleteOutlined />}
                            loading={actionLoading === agent.agent_id}
                          />
                        </Popconfirm>,
                      ]
                    : []
                }
              >
                <List.Item.Meta
                  avatar={
                    <Avatar size="small" style={{ backgroundColor: '#52c41a' }} icon={<RobotOutlined />} />
                  }
                  title={
                    <span>
                      {agent.name}
                      <Tag color="purple" style={{ fontSize: 10, marginLeft: 4 }}>智能体</Tag>
                    </span>
                  }
                  description={agent.cli_tool}
                />
              </List.Item>
            )}
          />
        </>
      )}

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
          setUserSearchQuery('');
          setUserSearchResults([]);
        }}
        width={280}
      >
        <Tabs
          size="small"
          items={[
            {
              key: 'friends',
              label: '好友',
              children: (
                <>
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
                </>
              ),
            },
            {
              key: 'search',
              label: '搜索用户',
              children: (
                <>
                  <Input.Search
                    placeholder="输入用户名搜索..."
                    allowClear
                    value={userSearchQuery}
                    onChange={(e) => handleUserSearch(e.target.value)}
                    onClear={() => {
                      setUserSearchQuery('');
                      setUserSearchResults([]);
                    }}
                    style={{ marginBottom: 12 }}
                  />
                  {userSearchLoading && (
                    <div style={{ textAlign: 'center', padding: '12px 0' }}>
                      <Spin size="small" />
                    </div>
                  )}
                  {!userSearchLoading && userSearchQuery.trim() && userSearchResults.length === 0 && (
                    <Empty description="未找到用户" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                  )}
                  {!userSearchLoading && userSearchResults.length > 0 && (
                    <List
                      dataSource={userSearchResults}
                      renderItem={(user) => (
                        <List.Item
                          actions={[
                            <Button
                              key="add"
                              type="primary"
                              size="small"
                              icon={<UserAddOutlined />}
                              loading={addingUser === user.id}
                              disabled={!!addingUser && addingUser !== user.id}
                              onClick={() => handleAddUser(user.id)}
                            >
                              添加
                            </Button>,
                          ]}
                        >
                          <List.Item.Meta
                            avatar={
                              <Avatar size="small" style={{ backgroundColor: '#1677ff' }}>
                                {user.username.charAt(0).toUpperCase()}
                              </Avatar>
                            }
                            title={user.username}
                          />
                        </List.Item>
                      )}
                    />
                  )}
                </>
              ),
            },
            {
              key: 'agents',
              label: '智能体',
              children: (() => {
                const agentIdsInGroup = new Set(agentMembers.map((a) => a.agent_id));
                const availableAgents = agents.filter((a) => !agentIdsInGroup.has(a.id));
                if (availableAgents.length === 0) {
                  return <Empty description="没有可添加的智能体" image={Empty.PRESENTED_IMAGE_SIMPLE} />;
                }
                return (
                  <List
                    dataSource={availableAgents}
                    renderItem={(agent) => (
                      <List.Item
                        actions={[
                          <Button
                            key="add"
                            type="primary"
                            size="small"
                            icon={<RobotOutlined />}
                            loading={addingAgent === agent.id}
                            disabled={!!addingAgent && addingAgent !== agent.id}
                            onClick={async () => {
                              setAddingAgent(agent.id);
                              try {
                                await addConversationAgent(conversationId, agent.id);
                                message.success('已添加智能体');
                                await fetchMembers();
                              } catch {
                                message.error('添加智能体失败');
                              } finally {
                                setAddingAgent(null);
                              }
                            }}
                          >
                            添加
                          </Button>,
                        ]}
                      >
                        <List.Item.Meta
                          avatar={
                            <Avatar size="small" style={{ backgroundColor: '#52c41a' }} icon={<RobotOutlined />} />
                          }
                          title={agent.name}
                          description={`${agent.cli_tool} · ${agent.status === 'online' ? '在线' : '离线'}`}
                        />
                      </List.Item>
                    )}
                  />
                );
              })(),
            },
          ]}
        />
      </Drawer>
    </Drawer>
  );
};

export default GroupMemberPanel;
