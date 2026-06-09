import React, { useMemo, useState } from 'react';
import { Avatar, Badge, Button, Empty, Input, Modal, Tabs } from 'antd';
import { message } from '@/utils/message';
import { CheckOutlined, CloseOutlined, SendOutlined } from '@ant-design/icons';
import { useFriendStore } from '@/store/friendStore';
import styles from './FriendRequest.module.css';
import { SimpleList as List } from '@/components/common/SimpleList';

const FriendRequest: React.FC = () => {
  const pendingRequests = useFriendStore((s) => s.pendingRequests);
  const sendRequest = useFriendStore((s) => s.sendRequest);
  const acceptRequest = useFriendStore((s) => s.acceptRequest);
  const rejectRequest = useFriendStore((s) => s.rejectRequest);
  const actionLoading = useFriendStore((s) => s.actionLoading);
  const [open, setOpen] = useState(false);
  const [username, setUsername] = useState('');
  const [sending, setSending] = useState(false);

  const pendingCount = pendingRequests.length;

  const handleSend = async () => {
    const trimmed = username.trim();
    if (!trimmed) return;
    setSending(true);
    try {
      await sendRequest(trimmed);
      message.success(`已向 ${trimmed} 发送好友请求`);
      setUsername('');
    } catch {
      // sendRequest handles error state
    } finally {
      setSending(false);
    }
  };

  const handleAccept = async (id: string) => {
    try {
      await acceptRequest(id);
      message.success('已接受好友请求');
    } catch {
      message.error('操作失败');
    }
  };

  const handleReject = async (id: string) => {
    try {
      await rejectRequest(id);
      message.success('已拒绝好友请求');
    } catch {
      message.error('操作失败');
    }
  };

  const formatTime = (dateStr: string): string => {
    const d = new Date(dateStr);
    if (isNaN(d.getTime())) return '';
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${month}-${day} ${hh}:${mm}`;
  };

  const requestList = useMemo(() => {
    if (pendingRequests.length === 0) {
      return (
        <div className={styles.empty}>
          <Empty description="暂无好友申请" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        </div>
      );
    }

    return (
      <List
        header={<span className={styles.pendingHeader}>待处理请求</span>}
        dataSource={pendingRequests}
        renderItem={(req) => (
          <List.Item
            actions={[
              <Button
                key="accept"
                type="primary"
                size="small"
                icon={<CheckOutlined />}
                loading={actionLoading === req.id}
                disabled={!!actionLoading && actionLoading !== req.id}
                onClick={() => handleAccept(req.id)}
              >
                接受
              </Button>,
              <Button
                key="reject"
                size="small"
                danger
                icon={<CloseOutlined />}
                loading={actionLoading === req.id}
                disabled={!!actionLoading}
                onClick={() => handleReject(req.id)}
              >
                拒绝
              </Button>,
            ]}
          >
            <List.Item.Meta
              avatar={
                <Avatar size="small" className={styles.requestAvatar}>
                  {(req.friend_name ?? '?').charAt(0).toUpperCase()}
                </Avatar>
              }
              title={req.friend_name ?? '未知用户'}
              description={formatTime(req.created_at)}
            />
          </List.Item>
        )}
      />
    );
  }, [pendingRequests, actionLoading]);

  return (
    <div className={styles.manager}>
      <Badge count={pendingCount} size="small">
        <Button className={styles.managerButton} onClick={() => setOpen(true)}>
          管理好友申请
        </Button>
      </Badge>
      <Modal
        open={open}
        title="好友管理"
        onCancel={() => setOpen(false)}
        footer={null}
      >
        <Tabs
          size="small"
          items={[
            {
              key: 'search',
              label: '搜索好友',
              children: (
                <div className={styles.tabBody}>
                  <Input.Search
                    className={styles.searchRow}
                    placeholder="输入用户名添加好友"
                    enterButton={
                      <Button type="primary" icon={<SendOutlined />} loading={sending}>
                        添加
                      </Button>
                    }
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    onSearch={handleSend}
                  />
                </div>
              ),
            },
            {
              key: 'requests',
              label: (
                <Badge count={pendingCount} size="small" offset={[6, -2]}>
                  好友申请
                </Badge>
              ),
              children: requestList,
            },
          ]}
        />
      </Modal>
    </div>
  );
};

export default FriendRequest;
