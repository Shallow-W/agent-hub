import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Input, Button, Tooltip, Spin, Select, Modal, message } from 'antd';
import {
  PlusOutlined,
  SendOutlined,
  PaperClipOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import { useAgents } from '@/hooks/useAgents';
import { useConversationStore } from '@/store/conversationStore';
import styles from './ChatInput.module.css';

const { TextArea } = Input;
const EMPTY_CONVERSATION_AGENTS: ReturnType<typeof useConversationStore.getState>['conversationAgents'][string] = [];

interface ChatInputProps {
  conversationId: string;
}

export const ChatInput: React.FC<ChatInputProps> = ({ conversationId }) => {
  const [value, setValue] = useState('');
  const [agentId, setAgentId] = useState<string | undefined>();
  const [addModalOpen, setAddModalOpen] = useState(false);
  const [addAgentIds, setAddAgentIds] = useState<string[]>([]);
  const { send, streamingContent } = useMessages(conversationId);
  const { agents } = useAgents();
  const conversationAgents = useConversationStore(
    (s) => s.conversationAgents[conversationId] ?? EMPTY_CONVERSATION_AGENTS,
  );
  const addConversationAgent = useConversationStore((s) => s.addConversationAgent);
  const isStreaming = (streamingContent ?? '').length > 0;
  const selectedAgent = conversationAgents.find((agent) => agent.agent_id === agentId);
  const availableAgents = useMemo(() => {
    const joined = new Set(conversationAgents.map((agent) => agent.agent_id));
    return agents.filter((agent) => !joined.has(agent.id));
  }, [agents, conversationAgents]);

  useEffect(() => {
    const firstAgentId = conversationAgents[0]?.agent_id;
    setAgentId((current) => {
      if (current && conversationAgents.some((agent) => agent.agent_id === current)) {
        return current;
      }
      return firstAgentId;
    });
  }, [conversationAgents, conversationId]);

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    setValue('');
    try {
      await send(trimmed, agentId);
    } catch {
      // 发送失败时恢复输入，避免用户丢失刚才写的内容。
      setValue(trimmed);
    }
  }, [value, isStreaming, send, agentId]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  const handleAddRobots = useCallback(async () => {
    if (addAgentIds.length === 0) return;
    const added = await Promise.all(
      addAgentIds.map((id) => addConversationAgent(conversationId, id)),
    );
    setAgentId((current) => current ?? added[0]?.agent_id);
    setAddAgentIds([]);
    setAddModalOpen(false);
    message.success(`已加入 ${added.length} 个 Robot`);
  }, [addAgentIds, addConversationAgent, conversationId]);

  const charCount = value.length;

  return (
    <div className={styles.container}>
      {isStreaming && (
        <div className={styles.typingIndicator}>
          <Spin size="small" />
          <span>Agent 正在思考</span>
        </div>
      )}
      <div className={styles.agentRow}>
        <RobotOutlined />
        <Select
          allowClear
          aria-label="选择 Agent"
          className={styles.agentSelect}
          optionFilterProp="label"
          options={conversationAgents.map((agent) => ({
            label: `${agent.name} · ${agent.cli_tool}`,
            value: agent.agent_id,
          }))}
          placeholder={conversationAgents.length === 0 ? '当前对话未添加 Robot' : '选择本对话 Robot'}
          showSearch
          size="small"
          value={agentId}
          onChange={setAgentId}
        />
        <Tooltip title="添加 Robot 到当前对话">
          <Button
            aria-label="添加 Robot"
            className={styles.addRobotButton}
            icon={<PlusOutlined />}
            size="small"
            onClick={() => setAddModalOpen(true)}
          />
        </Tooltip>
        {selectedAgent ? (
          <span className={styles.agentHint}>
            {selectedAgent.machine_name || selectedAgent.source}
          </span>
        ) : null}
      </div>
      <div className={styles.inputRow}>
        <TextArea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
          autoSize={{ minRows: 1, maxRows: 5 }}
          className={styles.textarea}
        />
        <Button
          type="primary"
          shape="default"
          icon={<SendOutlined />}
          onClick={handleSubmit}
          disabled={!value.trim() || isStreaming}
          style={{ flexShrink: 0, height: 44, width: 44 }}
        />
      </div>
      <div className={styles.toolbar}>
        <Tooltip title="附件">
          <Button type="text" icon={<PaperClipOutlined />} size="small" />
        </Tooltip>
        <span
          className={`${styles.charCount} ${charCount > 0 ? styles.charCountVisible : ''}`}
        >
          {charCount}
        </span>
      </div>
      <Modal
        title="添加 Robot 到当前对话"
        open={addModalOpen}
        okText="添加"
        cancelText="取消"
        okButtonProps={{ disabled: addAgentIds.length === 0 }}
        onOk={handleAddRobots}
        onCancel={() => {
          setAddModalOpen(false);
          setAddAgentIds([]);
        }}
      >
        <Select
          aria-label="选择要加入的 Robot"
          className={styles.addRobotSelect}
          mode="multiple"
          optionFilterProp="label"
          options={availableAgents.map((agent) => ({
            label: `${agent.name} · ${agent.cli_tool}${agent.machine_name ? ` · ${agent.machine_name}` : ''}`,
            value: agent.id,
          }))}
          placeholder={availableAgents.length === 0 ? '所有 Robot 都已在当前对话中' : '选择一个或多个 Robot'}
          showSearch
          value={addAgentIds}
          onChange={setAddAgentIds}
        />
      </Modal>
    </div>
  );
};
