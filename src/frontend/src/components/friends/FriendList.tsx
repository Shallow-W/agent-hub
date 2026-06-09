import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Avatar, Input, Badge, Tabs, Skeleton, Spin, Empty, Dropdown, Modal } from 'antd';
import { message } from '@/utils/message';
import type { MenuProps } from 'antd';
import { UserAddOutlined, DeleteOutlined, MoreOutlined } from '@ant-design/icons';
import { useFriendStore } from '@/store/friendStore';
import FriendRequest from './FriendRequest';
import styles from './FriendList.module.css';
import { SimpleList as List } from '@/components/common/SimpleList';

interface FriendListProps {
  onStartChat: (friendId: string) => void;
}

const FriendList: React.FC<FriendListProps> = ({ onStartChat }) => {
  const {
    friends,
    loading,
    pendingRequests,
    searchResults,
    isSearching,
    searchUsers,
    clearSearch,
    sendRequest,
    deleteFriend,
  } = useFriendStore();

  const [search, setSearch] = useState('');
  const [activeTab, setActiveTab] = useState('friends');
  const [addingUser, setAddingUser] = useState<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSearchChange = useCallback(
    (value: string) => {
      setSearch(value);
      if (timerRef.current !== null) clearTimeout(timerRef.current);
      if (!value.trim()) {
        clearSearch();
        return;
      }
      timerRef.current = setTimeout(() => {
        searchUsers(value.trim());
      }, 300);
    },
    [searchUsers, clearSearch],
  );

  useEffect(() => {
    return () => {
      if (timerRef.current !== null) clearTimeout(timerRef.current);
    };
  }, []);

  const handleAddFriend = async (username: string) => {
    setAddingUser(username);
    try {
      await sendRequest(username);
      message.success(`已向 ${username} 发送好友请求`);
    } catch {
      // error handled in store
    } finally {
      setAddingUser(null);
    }
  };

  const pendingCount = pendingRequests.length;

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

  const filteredFriends = search
    ? friends.filter((f) =>
        (f.friend_name ?? '').toLowerCase().includes(search.toLowerCase()),
      )
    : friends;

  const showSearchResults = search.trim().length > 0 && searchResults.length > 0;

  const renderFriendContent = () => {
    if (loading && !isSearching && friends.length === 0) {
      return (
        <div style={{ padding: '8px 0' }}>
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} style={{ display: 'flex', gap: 10, padding: '8px 12px', alignItems: 'center' }}>
              <Skeleton.Avatar active size="small" />
              <Skeleton active paragraph={{ rows: 0 }} title={{ width: '40%' }} style={{ margin: 0 }} />
            </div>
          ))}
        </div>
      );
    }

    return (
      <>
        {isSearching && (
          <div style={{ textAlign: 'center', padding: '12px 0' }}>
            <Spin size="small" />
          </div>
        )}

        {showSearchResults && (
          <List
            header={<span style={{ fontSize: 13, color: '#666' }}>搜索结果</span>}
            dataSource={searchResults}
            style={{ marginBottom: 16 }}
            renderItem={(user) => (
              <List.Item
                actions={[
                  <Badge key="add" count={0} size="small">
                    <button
                      className="ant-btn ant-btn-primary ant-btn-sm"
                      onClick={() => handleAddFriend(user.username)}
                      disabled={!!addingUser && addingUser !== user.username}
                      style={{ display: 'inline-flex', alignItems: 'center', gap: 4, cursor: addingUser && addingUser !== user.username ? 'not-allowed' : 'pointer', opacity: addingUser && addingUser !== user.username ? 0.5 : 1 }}
                    >
                      {addingUser === user.username ? '...' : <><UserAddOutlined /> 添加</>}
                    </button>
                  </Badge>,
                ]}
              >
                <List.Item.Meta
                  avatar={<Avatar size="small" style={{ backgroundColor: '#1677ff' }}>{user.username.charAt(0).toUpperCase()}</Avatar>}
                  title={user.username}
                />
              </List.Item>
            )}
          />
        )}

        <List
          loading={loading && !isSearching}
          dataSource={filteredFriends}
          locale={{
            emptyText: (
              <Empty
                description="暂无好友"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            ),
          }}
          renderItem={(friend) => {
            const friendName = friend.friend_name ?? '未知用户';
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
                className={styles.friendItem}
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
                    <Badge
                      dot
                      color="green"
                      offset={[-4, 30]}
                    >
                      <Avatar
                        style={{ backgroundColor: '#1677ff' }}
                        size="small"
                      >
                        {friendName.charAt(0).toUpperCase()}
                      </Avatar>
                    </Badge>
                  }
                  title={friendName}
                />
              </List.Item>
            );
          }}
        />
      </>
    );
  };

  const tabItems = [
    {
      key: 'friends',
      label: '好友列表',
      children: (
        <>
          <Input.Search
            placeholder="搜索好友或用户名..."
            allowClear
            value={search}
            onChange={(e) => handleSearchChange(e.target.value)}
            onClear={() => {
              setSearch('');
              clearSearch();
            }}
            style={{ marginBottom: 12 }}
          />
          {renderFriendContent()}
        </>
      ),
    },
    {
      key: 'requests',
      label: (
        <Badge count={pendingCount} size="small" offset={[6, -2]}>
          好友申请
        </Badge>
      ),
      children: <FriendRequest />,
    },
  ];

  return (
    <Tabs
      activeKey={activeTab}
      onChange={setActiveTab}
      items={tabItems}
      size="small"
      style={{ padding: '0 4px' }}
    />
  );
};

export default FriendList;
