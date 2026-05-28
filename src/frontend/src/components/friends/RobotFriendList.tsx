import React, { useState } from 'react';
import { Avatar, Badge, Input, List } from 'antd';
import { RobotOutlined } from '@ant-design/icons';
import type { Agent } from '@/types/agent';

interface RobotFriendListProps {
  agents: Agent[];
  onStartChat: (agent: Agent) => void;
}

export const RobotFriendList: React.FC<RobotFriendListProps> = ({
  agents,
  onStartChat,
}) => {
  const [search, setSearch] = useState('');
  const filtered = search
    ? agents.filter((agent) => (
        `${agent.name} ${agent.cli_tool} ${agent.machine_name ?? ''}`
          .toLowerCase()
          .includes(search.toLowerCase())
      ))
    : agents;

  return (
    <div>
      <Input.Search
        placeholder="搜索 Robot..."
        allowClear
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        style={{ marginBottom: 12 }}
      />
      <List
        dataSource={filtered}
        locale={{ emptyText: '暂无 Robot，请先在 Agent 页连接并创建' }}
        renderItem={(agent) => (
          <List.Item
            style={{ cursor: 'pointer', padding: '8px 12px' }}
            onClick={() => onStartChat(agent)}
          >
            <List.Item.Meta
              avatar={
                <Badge dot color="green" offset={[-4, 30]}>
                  <Avatar
                    style={{ backgroundColor: '#1677ff' }}
                    size="small"
                    icon={<RobotOutlined />}
                  />
                </Badge>
              }
              title={agent.name}
              description={`${agent.cli_tool}${agent.machine_name ? ` · ${agent.machine_name}` : ''}`}
            />
          </List.Item>
        )}
      />
    </div>
  );
};
