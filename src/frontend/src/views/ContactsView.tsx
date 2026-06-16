import React, { useCallback } from 'react';
import { Button } from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import ContactsPanel from '@/components/contacts/ContactsPanel';
import { useConversationStore } from '@/store/conversationStore';
import { useFriendStore } from '@/store/friendStore';
import { getOrCreatePrivateChat, getOrCreateAgentChat } from '@/api/conversation';
import { useAuthStore } from '@/store/authStore';
import type { Agent } from '@/types/agent';
import styles from '@/layout/AppLayout.module.css';

const ContactsView: React.FC = () => {
  const navigate = useNavigate();
  const conversations = useConversationStore((s) => s.conversations);
  const setActive = useConversationStore((s) => s.setActive);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const fetchFriends = useFriendStore((s) => s.fetchFriends);
  const fetchPending = useFriendStore((s) => s.fetchPending);
  const userID = useAuthStore((s) => s.user?.id);

  const handleStartChat = useCallback(async (friendId: string) => {
    const existing = conversations.find(
      (c) => c.type === 'single' && c.peer_id === friendId,
    );
    if (existing) {
      setActive(existing.id);
    } else {
      const conv = await getOrCreatePrivateChat(friendId);
      await fetchConversations();
      setActive(conv.id);
    }
    navigate('/');
  }, [conversations, setActive, fetchConversations, navigate]);

  const handleStartAgentChat = useCallback(async (agent: Agent) => {
    const existing = conversations.find(
      (c) => c.type === 'agent' && c.peer_id === agent.id,
    );
    if (existing) {
      setActive(existing.id);
    } else {
      const conv = await getOrCreateAgentChat(agent.id);
      await fetchConversations();
      setActive(conv.id);
    }
    navigate('/');
  }, [conversations, setActive, fetchConversations, navigate, userID]);

  const handleRefresh = useCallback(async () => {
    await Promise.all([fetchFriends(true), fetchPending(true), fetchConversations()]);
  }, [fetchFriends, fetchPending, fetchConversations]);

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>联系人</span>
        <div className={styles.convPanelTools}>
          <Button type="text" icon={<PlusOutlined />} aria-label="新建群聊" onClick={() => navigate('/')} />
          <Button type="text" icon={<ReloadOutlined />} aria-label="刷新" onClick={handleRefresh} />
        </div>
      </div>
      <ContactsPanel
        conversations={conversations}
        onStartChat={handleStartChat}
        onStartAgentChat={handleStartAgentChat}
        onSwitchChat={() => navigate('/')}
      />
    </>
  );
};

export default ContactsView;
