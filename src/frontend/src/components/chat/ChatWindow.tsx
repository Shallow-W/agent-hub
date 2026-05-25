import React from 'react';
import { Typography, Tooltip, Button } from 'antd';
import { SearchOutlined, MoreOutlined } from '@ant-design/icons';
import { useConversation } from '@/hooks/useConversation';
import { MessageList } from './MessageList';
import { ChatInput } from './ChatInput';
import styles from './ChatWindow.module.css';

const { Title } = Typography;

export const ChatWindow: React.FC = () => {
  const { conversations, activeId } = useConversation();
  const activeConv = conversations.find((c) => c.id === activeId);

  if (!activeConv) {
    return null;
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.convIcon} role="img" aria-label="conversation">&#x1F4AC;</span>
          <Title level={5} style={{ margin: 0 }} ellipsis>
            {activeConv.title}
          </Title>
        </div>
        <div className={styles.headerActions}>
          <Tooltip title="搜索对话">
            <Button type="text" icon={<SearchOutlined />} size="small" />
          </Tooltip>
          <Tooltip title="更多选项">
            <Button type="text" icon={<MoreOutlined />} size="small" />
          </Tooltip>
        </div>
      </div>
      <MessageList conversationId={activeConv.id} />
      <ChatInput conversationId={activeConv.id} />
    </div>
  );
};
