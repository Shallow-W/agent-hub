import React, { useMemo, useState } from 'react';
import { Avatar, Badge, Dropdown, Empty, Input, List, Modal, Tabs, message } from 'antd';
import type { MenuProps } from 'antd';
import { DeleteOutlined, MoreOutlined } from '@ant-design/icons';
import { useFriendStore } from '@/store/friendStore';
import { useConversationStore } from '@/store/conversationStore';
import type { Conversation } from '@/types/conversation';
import type { Friend } from '@/types/friend';
import FriendRequest from '../friends/FriendRequest';
import friendStyles from '../friends/FriendList.module.css';
import layoutStyles from '@/layout/AppLayout.module.css';
import styles from './ContactsPanel.module.css';

interface ContactsPanelProps {
  conversations: Conversation[];
  onStartChat: (friendId: string) => void;
  onSwitchChat: () => void;
}

const getFriendName = (friend: Friend): string => friend.friend_name ?? '未知用户';

const ContactsPanel: React.FC<ContactsPanelProps> = ({ conversations, onStartChat, onSwitchChat }) => {
  const {
    friends,
    loading,
    deleteFriend,
  } = useFriendStore();
  const setActive = useConversationStore((s) => s.setActive);
  const [query, setQuery] = useState('');

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
        className={friendStyles.friendItem}
        onClick={() => onStartChat(friend.friend_id)}
        actions={[
          <Dropdown
            key="more"
            menu={{ items: menuItems }}
            trigger={['click']}
            placement="bottomRight"
          >
            <button
              className={friendStyles.actionBtn}
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
              <Avatar className={styles.friendAvatar} size="small">
                {friendName.charAt(0).toUpperCase()}
              </Avatar>
            </Badge>
          }
          title={friendName}
        />
      </List.Item>
    );
  };

  return (
    <div className={styles.container}>
      <div className={styles.searchBar}>
        <Input.Search
          placeholder="搜索好友或群聊..."
          allowClear
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      <div className={styles.section}>
        <div className={styles.sectionHeader}>好友申请</div>
        <div className={styles.managerCard}>
          <FriendRequest />
        </div>
      </div>

      <Tabs
        className={styles.tabs}
        size="small"
        items={[
          {
            key: 'friends',
            label: '好友列表',
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
            label: '群聊列表',
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
                        className={friendStyles.friendItem}
                        onClick={() => {
                          setActive(conv.id);
                          onSwitchChat();
                        }}
                      >
                        <List.Item.Meta
                          avatar={
                            <Avatar className={styles.friendAvatar} size="small">
                              {conv.title.charAt(0).toUpperCase()}
                            </Avatar>
                          }
                          title={conv.title}
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
  );
};

export default ContactsPanel;
