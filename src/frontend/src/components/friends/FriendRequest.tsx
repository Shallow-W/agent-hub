import React, { useState } from 'react';
import { Input, Button, List, Avatar, message, Empty } from 'antd';
import { SendOutlined, CheckOutlined, CloseOutlined } from '@ant-design/icons';
import { useFriendStore } from '@/store/friendStore';

const FriendRequest: React.FC = () => {
  const { pendingRequests, sendRequest, acceptRequest, rejectRequest, loading } =
    useFriendStore();
  const [username, setUsername] = useState('');
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const handleSend = async () => {
    const trimmed = username.trim();
    if (!trimmed) return;
    try {
      await sendRequest(trimmed);
      message.success(`已向 ${trimmed} 发送好友请求`);
      setUsername('');
    } catch {
      // sendRequest handles error state
    }
  };

  const handleAccept = async (id: string) => {
    setActionLoading(id + '-accept');
    try {
      await acceptRequest(id);
      message.success('已接受好友请求');
    } catch {
      message.error('操作失败');
    } finally {
      setActionLoading(null);
    }
  };

  const handleReject = async (id: string) => {
    setActionLoading(id + '-reject');
    try {
      await rejectRequest(id);
      message.success('已拒绝好友请求');
    } catch {
      message.error('操作失败');
    } finally {
      setActionLoading(null);
    }
  };

  const formatTime = (dateStr: string): string => {
    const d = new Date(dateStr);
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${month}-${day} ${hh}:${mm}`;
  };

  return (
    <div>
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

      {pendingRequests.length === 0 ? (
        <Empty
          description="暂无好友申请"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      ) : (
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
                  loading={actionLoading === req.id + '-accept'}
                  onClick={() => handleAccept(req.id)}
                >
                  接受
                </Button>,
                <Button
                  key="reject"
                  size="small"
                  danger
                  icon={<CloseOutlined />}
                  loading={actionLoading === req.id + '-reject'}
                  onClick={() => handleReject(req.id)}
                >
                  拒绝
                </Button>,
              ]}
            >
              <List.Item.Meta
                avatar={
                  <Avatar size="small" style={{ backgroundColor: '#1677ff' }}>
                    {(req.friend_name ?? '?').charAt(0).toUpperCase()}
                  </Avatar>
                }
                title={req.friend_name ?? '未知用户'}
                description={
                  <span style={{ fontSize: 11, color: '#999' }}>
                    {formatTime(req.created_at)}
                  </span>
                }
              />
            </List.Item>
          )}
        />
      )}
    </div>
  );
};

export default FriendRequest;
