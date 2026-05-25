import React, { useState } from 'react';
import { Avatar, Input, List, Badge } from 'antd';
import { useFriendStore } from '@/store/friendStore';

interface FriendListProps {
  onStartChat: (friendId: string) => void;
}

const FriendList: React.FC<FriendListProps> = ({ onStartChat }) => {
  const { friends, loading } = useFriendStore();
  const [search, setSearch] = useState('');

  const filtered = search
    ? friends.filter((f) =>
        (f.friend_name ?? '').toLowerCase().includes(search.toLowerCase()),
      )
    : friends;

  return (
    <div>
      <Input.Search
        placeholder="搜索好友..."
        allowClear
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        style={{ marginBottom: 12 }}
      />
      <List
        loading={loading}
        dataSource={filtered}
        locale={{ emptyText: '暂无好友' }}
        renderItem={(friend) => (
          <List.Item
            style={{ cursor: 'pointer', padding: '8px 12px' }}
            onClick={() => onStartChat(friend.friend_id)}
          >
            <List.Item.Meta
              avatar={
                <Badge dot color="green" offset={[-4, 30]}>
                  <Avatar
                    style={{ backgroundColor: '#1677ff' }}
                    size="small"
                  >
                    {(friend.friend_name ?? '?').charAt(0).toUpperCase()}
                  </Avatar>
                </Badge>
              }
              title={friend.friend_name ?? '未知用户'}
            />
          </List.Item>
        )}
      />
    </div>
  );
};

export default FriendList;
