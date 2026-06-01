import React, { useEffect, useMemo, useState } from 'react';
import { Checkbox, Empty, Input, Modal, Tabs } from 'antd';
import { RobotOutlined, UserOutlined } from '@ant-design/icons';
import { useFriendStore } from '@/store/friendStore';
import { useAgentStore } from '@/store/agentStore';
import styles from './NewConversationModal.module.css';

interface NewConversationModalProps {
  open: boolean;
  onCancel: () => void;
  onCreate: (title: string, memberIds: string[], agentIds: string[]) => Promise<void>;
}

const NewConversationModal: React.FC<NewConversationModalProps> = ({
  open,
  onCancel,
  onCreate,
}) => {
  const [title, setTitle] = useState('');
  const [memberSearch, setMemberSearch] = useState('');
  const [agentSearch, setAgentSearch] = useState('');
  const [selectedMembers, setSelectedMembers] = useState<string[]>([]);
  const [selectedAgents, setSelectedAgents] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const friends = useFriendStore((s) => s.friends);
  const fetchFriends = useFriendStore((s) => s.fetchFriends);
  const agents = useAgentStore((s) => s.agents);
  const fetchAgents = useAgentStore((s) => s.fetchAgents);

  useEffect(() => {
    if (!open) return;
    setTitle('');
    setMemberSearch('');
    setAgentSearch('');
    setSelectedMembers([]);
    setSelectedAgents([]);
    fetchFriends().catch(() => {});
    fetchAgents().catch(() => {});
  }, [fetchAgents, fetchFriends, open]);

  const filteredFriends = useMemo(() => {
    const keyword = memberSearch.trim().toLowerCase();
    if (!keyword) return friends;
    return friends.filter((friend) =>
      (friend.friend_name ?? '').toLowerCase().includes(keyword),
    );
  }, [friends, memberSearch]);

  const filteredAgents = useMemo(() => {
    const keyword = agentSearch.trim().toLowerCase();
    if (!keyword) return agents;
    return agents.filter((agent) =>
      agent.name.toLowerCase().includes(keyword) ||
      agent.cli_tool.toLowerCase().includes(keyword),
    );
  }, [agents, agentSearch]);

  const submit = async () => {
    setSubmitting(true);
    try {
      await onCreate(title.trim() || '新对话', selectedMembers, selectedAgents);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title="新建对话"
      open={open}
      onOk={submit}
      onCancel={onCancel}
      okText="创建"
      cancelText="取消"
      confirmLoading={submitting}
      destroyOnClose
    >
      <div className={styles.content}>
        <Input
          placeholder="对话标题，可留空"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          onPressEnter={submit}
          maxLength={50}
          autoFocus
        />
        <Tabs
          size="small"
          items={[
            {
              key: 'friends',
              label: '好友',
              children: (
                <div className={styles.panel}>
                  <Input.Search
                    placeholder="搜索好友"
                    allowClear
                    value={memberSearch}
                    onChange={(e) => setMemberSearch(e.target.value)}
                  />
                  {filteredFriends.length === 0 ? (
                    <Empty description="暂无可拉入的好友" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                  ) : (
                    <Checkbox.Group
                      value={selectedMembers}
                      onChange={(values) => setSelectedMembers(values.map(String))}
                      className={styles.optionList}
                    >
                      {filteredFriends.map((friend) => (
                        <Checkbox key={friend.friend_id} value={friend.friend_id}>
                          <UserOutlined /> {friend.friend_name ?? '未知用户'}
                        </Checkbox>
                      ))}
                    </Checkbox.Group>
                  )}
                </div>
              ),
            },
            {
              key: 'agents',
              label: '智能体',
              children: (
                <div className={styles.panel}>
                  <Input.Search
                    placeholder="搜索智能体"
                    allowClear
                    value={agentSearch}
                    onChange={(e) => setAgentSearch(e.target.value)}
                  />
                  {filteredAgents.length === 0 ? (
                    <Empty description="暂无可拉入的智能体" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                  ) : (
                    <Checkbox.Group
                      value={selectedAgents}
                      onChange={(values) => setSelectedAgents(values.map(String))}
                      className={styles.optionList}
                    >
                      {filteredAgents.map((agent) => (
                        <Checkbox key={agent.id} value={agent.id}>
                          <RobotOutlined /> {agent.name}
                          <span className={styles.optionMeta}>{agent.cli_tool}</span>
                        </Checkbox>
                      ))}
                    </Checkbox.Group>
                  )}
                </div>
              ),
            },
          ]}
        />
      </div>
    </Modal>
  );
};

export default NewConversationModal;
