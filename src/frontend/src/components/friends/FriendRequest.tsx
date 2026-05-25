import React, { useState } from 'react';
import { Input, Button, List, Avatar, message } from 'antd';
import { SendOutlined, CheckOutlined, CloseOutlined } from '@ant-design/icons';
import { useFriendStore } from '@/store/friendStore';

const FriendRequest: React.FC = () => {
  const { pendingRequests, sendRequest, acceptRequest, rejectRequest, loading } =
    useFriendStore();
  const [username, setUsername] = useState('');

  const handleSend = async () => {
    const trimmed = username.trim();
    if (!trimmed) return;
    try {
      await sendRequest(trimmed);
      message.success(`已向 ${trimmed} 发送好友请求`);
      setUsername('');
    } catch {
      // sendRequest 内部已设置 error state
    }
  };

  return (
    <div>
      {/* 发送好友请求 */}
      <Input.Search
        placeholder="输入用户名添加好友"
        enterButton={
          <Button type="primary" icon={<SendOutlined />} loading={loading}>
            添加
          </Button>
        }
        value={username}
        onChange={(e) => setUsername(e.target.value)}
        onSearch={handleSend}
        style={{ marginBottom: 16 }}
      />

      {/* 待处理请求列表 */}
      {pendingRequests.length > 0 && (
        <List
          header={<span style={{ fontSize: 13, color: '#666' }}>待处理请求</span>}
          dataSource={pendingRequests}
          renderItem={(req) => (
            <List.Item
              actions={[
                <Button
                  key="accept"
                  type="primary"
                  size="small"
                  icon={<CheckOutlined />}
                  onClick={() => acceptRequest(req.id)}
                >
                  接受
                </Button>,
                <Button
                  key="reject"
                  size="small"
                  danger
                  icon={<CloseOutlined />}
                  onClick={() => rejectRequest(req.id)}
                >
                  拒绝
                </Button>,
              ]}
            >
              <List.Item.Meta
                avatar={<Avatar size="small">{(req.from_username ?? '?').charAt(0).toUpperCase()}</Avatar>}
                title={req.from_username ?? '未知用户'}
              />
            </List.Item>
          )}
        />
      )}
    </div>
  );
};

export default FriendRequest;
