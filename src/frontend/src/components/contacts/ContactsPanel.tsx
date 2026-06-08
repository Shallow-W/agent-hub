import React, { useEffect, useMemo, useState } from 'react';
import { Avatar, Badge, Dropdown, Empty, Input, List, Modal, Tabs, message } from 'antd';
import type { MenuProps } from 'antd';
import { DeleteOutlined, MoreOutlined, RobotOutlined, SearchOutlined, TeamOutlined } from '@ant-design/icons';
import { useFriendStore } from '@/store/friendStore';
import { useConversationStore } from '@/store/conversationStore';
import { useAgentStore } from '@/store/agentStore';
import { resolveAgentAvatar, resolveUserAvatar, avatarUrl } from '@/components/agent/agentPresentation';
import type { Conversation } from '@/types/conversation';
import type { Friend } from '@/types/friend';
import type { Agent } from '@/types/agent';
import FriendRequest from '../friends/FriendRequest';
import layoutStyles from '@/layout/AppLayout.module.css';
import styles from './ContactsPanel.module.css';

interface ContactsPanelProps {
  conversations: Conversation[];
  onStartChat: (friendId: string) => void;
  onStartAgentChat: (agent: Agent) => void;
  onSwitchChat: () => void;
}

const getFriendName = (friend: Friend): string => friend.friend_name ?? '未知用户';

const ContactsPanel: React.FC<ContactsPanelProps> = ({
  conversations,
  onStartChat,
  onStartAgentChat,
  onSwitchChat,
}) => {
  const {
    friends,
    loading,
    deleteFriend,
  } = useFriendStore();
  const setActive = useConversationStore((s) => s.setActive);
  const agents = useAgentStore((s) => s.agents);
  const fetchAgents = useAgentStore((s) => s.fetchAgents);
  const [query, setQuery] = useState('');

  useEffect(() => { fetchAgents(); }, [fetchAgents]);

  const normalizedQuery = query.trim().toLowerCase();
  const filteredFriends = useMemo(() => {
    const base = normalizedQuery
      ? friends.filter((f) => getFriendName(f).toLowerCase().includes(normalizedQuery))
      : friends;
    return [...base].sort((a, b) =>
      getFriendName(a).localeCompare(getFriendName(b), 'zh-Hans-CN', { sensitivity: 'base' }),
    );
  }, [friends, normalizedQuery]);

  const groupConvs = useMemo(
    () => conversations.filter((c) => c.type === 'group'),
    [conversations],
  );

  const filteredGroups = useMemo(() => {
    if (!normalizedQuery) return groupConvs;
    return groupConvs.filter((c) => (c.title ?? '').toLowerCase().includes(normalizedQuery));
  }, [groupConvs, normalizedQuery]);

  const filteredAgents = useMemo(() => {
    if (!normalizedQuery) return agents;
    return agents.filter((a) => a.name.toLowerCase().includes(normalizedQuery));
  }, [agents, normalizedQuery]);

  const hasFriends = filteredFriends.length > 0;

  const handleDeleteFriend = (friendId: string, friendName: string) => {
    Modal.confirm({
      title: '确认删除好友',
      content: `确定要删除好友「${friendName}」吗？`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteFriend(friendId);
          message.success('已删除好友');
        } catch {
          // error handled in store
        }
      },
    });
  };

  const renderFriendItem = (friend: Friend) => {
    const friendName = getFriendName(friend);
    const menuItems: MenuProps['items'] = [
      {
        key: 'delete',
        icon: <DeleteOutlined />,
        label: '删除好友',
        danger: true,
        onClick: (info) => {
          info.domEvent.stopPropagation();
          handleDeleteFriend(friend.friend_id, friendName);
        },
      },
    ];

    return (
      <List.Item
        className={styles.contactItem}
        onClick={() => onStartChat(friend.friend_id)}
        actions={[
          <Dropdown
            key="more"
            menu={{ items: menuItems }}
            trigger={['click']}
            placement="bottomRight"
          >
            <button
              className={styles.actionBtn}
              onClick={(e) => e.stopPropagation()}
              aria-label="更多操作"
            >
              <MoreOutlined />
            </button>
          </Dropdown>,
        ]}
      >
        <List.Item.Meta
          avatar={
            <Badge dot color="green">
              <Avatar className={styles.friendAvatar} size={28} src={resolveUserAvatar({ id: friend.friend_id, username: friendName })}>
                {friendName.charAt(0).toUpperCase()}
              </Avatar>
            </Badge>
          }
          title={<span className={styles.contactName}>{friendName}</span>}
          description={<span className={styles.contactMeta}>好友</span>}
        />
      </List.Item>
    );
  };

  return (
    <div className={styles.container}>
      <div className={styles.searchBar}>
        <Input
          prefix={<SearchOutlined />}
          placeholder="搜索好友或群聊..."
          allowClear
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className={styles.searchInput}
        />
      </div>

      <div className={styles.scrollContent}>
        <div className={styles.section}>
          <div className={styles.managerCard}>
            <div className={styles.managerCopy}>
              <span className={styles.managerTitle}>好友申请</span>
              <span className={styles.managerHint}>{friends.length} 位好友 · {groupConvs.length} 个群聊</span>
            </div>
            <FriendRequest />
          </div>
        </div>

        <Tabs
        className={styles.tabs}
        size="small"
        items={[
          {
            key: 'friends',
            label: `好友 ${filteredFriends.length}`,
            children: (
              <div className={styles.section}>
                {loading && !hasFriends ? (
                  <div className={styles.empty}>
                    <Empty description="加载中" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                  </div>
                ) : !hasFriends ? (
                  <div className={styles.empty}>
                    <Empty description="暂无好友" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                  </div>
                ) : (
                  <List
                    dataSource={filteredFriends}
                    renderItem={renderFriendItem}
                    split={false}
                  />
                )}
              </div>
            ),
          },
          {
            key: 'groups',
            label: `群聊 ${filteredGroups.length}`,
            children: (
              <div className={styles.section}>
                {filteredGroups.length === 0 ? (
                  <div className={layoutStyles.emptyState}>暂无群聊</div>
                ) : (
                  <List
                    dataSource={filteredGroups}
                    split={false}
                    renderItem={(conv) => (
                      <List.Item
                        className={styles.contactItem}
                        onClick={() => {
                          setActive(conv.id);
                          onSwitchChat();
                        }}
                      >
                        <List.Item.Meta
                          avatar={
                              <Avatar className={styles.friendAvatar} size={28} src={conv.avatar ? (/^(https?:|data:|\/)/i.test(conv.avatar) ? conv.avatar : avatarUrl(conv.avatar)) : undefined} icon={!conv.avatar ? <TeamOutlined /> : undefined} />
                          }
                          title={<span className={styles.contactName}>{conv.title}</span>}
                          description={<span className={styles.contactMeta}>群聊</span>}
                        />
                      </List.Item>
                    )}
                  />
                )}
              </div>
            ),
          },
          {
            key: 'agents',
            label: `智能体 ${filteredAgents.length}`,
            children: (
              <div className={styles.section}>
                {filteredAgents.length === 0 ? (
                  <div className={layoutStyles.emptyState}>暂无智能体</div>
                ) : (
                  <List
                    dataSource={filteredAgents}
                    split={false}
                    renderItem={(agent) => (
                      <List.Item className={styles.contactItem} onClick={() => onStartAgentChat(agent)}>
                        <List.Item.Meta
                          avatar={
                            <Avatar className={styles.agentAvatar} size={28} src={resolveAgentAvatar(agent)} icon={<RobotOutlined />} />
                          }
                          title={<span className={styles.contactName}>{agent.name}</span>}
                          description={<span className={styles.contactMeta}>{agent.cli_tool}</span>}
                        />
                      </List.Item>
                    )}
                  />
                )}
              </div>
            ),
          },
        ]}
      />
      </div>
    </div>
  );
};

export default ContactsPanel;
