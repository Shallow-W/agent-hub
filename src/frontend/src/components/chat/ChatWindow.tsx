import React, { useEffect, useMemo, useState } from 'react';
import { Typography, Tooltip, Button, Modal, Select, Tag, message } from 'antd';
import {
  MoreOutlined,
  PlusOutlined,
  RobotOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useConversation } from '@/hooks/useConversation';
import { useAgents } from '@/hooks/useAgents';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import styles from './ChatWindow.module.css';

const { Title } = Typography;

export const ChatWindow: React.FC = () => {
  const {
    conversations,
    activeId,
    conversationAgents,
    fetchConversationAgents,
    addConversationAgent,
    removeConversationAgent,
  } = useConversation();
  const { agents } = useAgents();
  const [modalOpen, setModalOpen] = useState(false);
  const [selectedAgentId, setSelectedAgentId] = useState<string>();
  const activeConv = conversations.find((c) => c.id === activeId);
  const robots = activeId ? (conversationAgents[activeId] ?? []) : [];

  useEffect(() => {
    if (!activeId) return;
    fetchConversationAgents(activeId);
  }, [activeId, fetchConversationAgents]);

  const availableAgents = useMemo(() => {
    const joined = new Set(robots.map((robot) => robot.agent_id));
    return agents.filter((agent) => !joined.has(agent.id));
  }, [agents, robots]);

  const handleAddRobot = async () => {
    if (!activeId || !selectedAgentId) return;
    await addConversationAgent(activeId, selectedAgentId);
    setSelectedAgentId(undefined);
    setModalOpen(false);
    message.success('Robot 已加入当前对话');
  };

  const handleRemoveRobot = async (agentId: string) => {
    if (!activeId) return;
    await removeConversationAgent(activeId, agentId);
  };

  if (!activeConv) {
    return null;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.convIcon} role="img" aria-label="conversation">&#x1F4AC;</span>
          <div className={styles.titleBlock}>
            <Title level={5} style={{ margin: 0 }} ellipsis>
              {activeConv.title}
            </Title>
            <div className={styles.robotStrip}>
              {robots.length === 0 ? (
                <span className={styles.robotHint}>未添加 Robot</span>
              ) : (
                robots.map((robot) => (
                  <Tag
                    key={robot.agent_id}
                    closable
                    icon={<RobotOutlined />}
                    onClose={(event) => {
                      event.preventDefault();
                      handleRemoveRobot(robot.agent_id);
                    }}
                  >
                    {robot.name}
                  </Tag>
                ))
              )}
            </div>
          </div>
        </div>
        <div className={styles.headerActions}>
          <Tooltip title="添加 Robot">
            <Button
              type="text"
              icon={<PlusOutlined />}
              size="small"
              onClick={() => setModalOpen(true)}
            />
          </Tooltip>
          <Tooltip title="搜索对话">
            <Button type="text" icon={<SearchOutlined />} size="small" />
          </Tooltip>
          <Tooltip title="更多选项">
            <Button type="text" icon={<MoreOutlined />} size="small" />
          </Tooltip>
        </div>
      </div>
      <MessageList key={`messages-${activeConv.id}`} conversationId={activeConv.id} />
      <ChatInput key={`input-${activeConv.id}`} conversationId={activeConv.id} />

      <Modal
        title="添加 Robot 到当前对话"
        open={modalOpen}
        okText="添加"
        cancelText="取消"
        okButtonProps={{ disabled: !selectedAgentId }}
        onOk={handleAddRobot}
        onCancel={() => setModalOpen(false)}
      >
        <Select
          aria-label="选择要加入的 Robot"
          style={{ width: '100%' }}
          placeholder={availableAgents.length === 0 ? '暂无可加入 Robot' : '选择一个 Robot'}
          value={selectedAgentId}
          onChange={setSelectedAgentId}
          options={availableAgents.map((agent) => ({
            value: agent.id,
            label: `${agent.name} · ${agent.cli_tool}${agent.machine_name ? ` · ${agent.machine_name}` : ''}`,
          }))}
          showSearch
          optionFilterProp="label"
        />
      </Modal>
    </div>
  );
};
