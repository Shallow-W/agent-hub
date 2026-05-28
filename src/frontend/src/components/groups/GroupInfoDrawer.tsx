import React, { useEffect, useState, useCallback } from 'react';
import {
  Drawer,
  List,
  Avatar,
  Tag,
  Empty,
  Spin,
  Descriptions,
  message,
} from 'antd';
import { TeamOutlined } from '@ant-design/icons';
import { getGroupInfo } from '@/api/group';
import type { GroupInfo } from '@/api/group';
import styles from './GroupInfoDrawer.module.css';

interface GroupInfoDrawerProps {
  open: boolean;
  onClose: () => void;
  conversationId: string;
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

const GroupInfoDrawer: React.FC<GroupInfoDrawerProps> = ({
  open,
  onClose,
  conversationId,
}) => {
  const [info, setInfo] = useState<GroupInfo | null>(null);
  const [loading, setLoading] = useState(false);

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
    }
  }, [open, conversationId, fetchInfo]);

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
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="群名称">
                {info.conversation?.title ?? "未命名"}
              </Descriptions.Item>
              <Descriptions.Item label="创建时间">
                {info.conversation?.created_at ? new Date(info.conversation.created_at).toLocaleString() : "-"}
              </Descriptions.Item>
              <Descriptions.Item label="成员数">
                {info.members.length}
              </Descriptions.Item>
            </Descriptions>

            <div className={styles.sectionTitle}>
              <TeamOutlined /> 成员列表
            </div>

            {info.members.length === 0 ? (
              <Empty description="暂无成员" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : (
              <List
                dataSource={info.members}
                renderItem={(member) => (
                  <List.Item>
                    <List.Item.Meta
                      avatar={
                        <Avatar size="small" style={{ backgroundColor: '#1677ff' }}>
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
          </>
        ) : (
          !loading && <Empty description="暂无群聊信息" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}
      </Spin>
    </Drawer>
  );
};

export default GroupInfoDrawer;
