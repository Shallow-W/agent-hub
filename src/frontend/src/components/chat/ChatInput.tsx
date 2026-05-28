import React, { useState, useCallback } from 'react';
import { Input, Button, Tooltip, Spin, Select } from 'antd';
import { SendOutlined, PaperClipOutlined, RobotOutlined } from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import { useAgents } from '@/hooks/useAgents';
import styles from './ChatInput.module.css';

const { TextArea } = Input;

interface ChatInputProps {
  conversationId: string;
}

export const ChatInput: React.FC<ChatInputProps> = ({ conversationId }) => {
  const [value, setValue] = useState('');
  const [agentId, setAgentId] = useState<string | undefined>();
  const { send, streamingContent } = useMessages(conversationId);
  const { agents } = useAgents();
  const isStreaming = (streamingContent ?? '').length > 0;
  const selectedAgent = agents.find((agent) => agent.id === agentId);

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    setValue('');
    try {
      await send(trimmed, agentId);
    } catch {
      // 发送失败时恢复输入内容
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

  const charCount = value.length;

  return (
    <div className={styles.container}>
      {isStreaming && (
        <div className={styles.typingIndicator}>
          <Spin size="small" />
          <span>Agent 正在输入</span>
        </div>
      )}
      <div className={styles.agentRow}>
        <RobotOutlined />
        <Select
          allowClear
          aria-label="选择 Agent"
          className={styles.agentSelect}
          optionFilterProp="label"
          options={agents.map((agent) => ({
            label: `${agent.name} · ${agent.cli_tool}`,
            value: agent.id,
          }))}
          placeholder={agents.length === 0 ? '暂无可用 Agent' : '选择 Agent 接入本次消息'}
          showSearch
          size="small"
          value={agentId}
          onChange={setAgentId}
        />
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
    </div>
  );
};
