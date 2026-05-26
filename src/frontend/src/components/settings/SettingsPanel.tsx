import React, { useState } from 'react';
import { Avatar, Button, Menu, Switch, Tooltip } from 'antd';
import {
  MessageOutlined,
  SettingOutlined,
  InfoCircleOutlined,
  UserAddOutlined,
  LogoutOutlined,
  TeamOutlined,
  BulbOutlined,
} from '@ant-design/icons';
import styles from './SettingsPanel.module.css';

type WsStatus = 'connected' | 'connecting' | 'disconnected';

interface SettingsPanelProps {
  username: string;
  onLogout: () => void;
  wsStatus: WsStatus;
  onNavChange: (key: string) => void;
}

const wsStatusText: Record<WsStatus, string> = {
  connected: '已连接',
  connecting: '连接中',
  disconnected: '已断开',
};

const wsDotColor: Record<WsStatus, string> = {
  connected: '#52c41a',
  connecting: '#faad14',
  disconnected: '#ff4d4f',
};

const SettingsPanel: React.FC<SettingsPanelProps> = ({
  username,
  onLogout,
  wsStatus,
  onNavChange,
}) => {
  const [selectedKey, setSelectedKey] = useState('chat');
  const [darkMode, setDarkMode] = useState(false);
  const initial = username ? username.charAt(0).toUpperCase() : '?';

  const handleMenuClick = (info: { key: string }) => {
    setSelectedKey(info.key);
    onNavChange(info.key);
  };

  return (
    <div className={styles.panel}>
      <div className={styles.brand}>
        <div className={styles.brandIcon}>A</div>
        <span className={styles.brandName}>AgentHub</span>
      </div>

      <div className={styles.profile}>
        <Avatar
          style={{ backgroundColor: '#1677ff', flexShrink: 0 }}
          size={34}
        >
          {initial}
        </Avatar>
        <span className={styles.profileName}>{username}</span>
      </div>

      <Menu
        mode="inline"
        selectedKeys={[selectedKey]}
        onClick={handleMenuClick}
        style={{ border: 'none', flex: 1 }}
        items={[
          { key: 'chat', icon: <MessageOutlined />, label: '对话' },
          { key: 'friends', icon: <UserAddOutlined />, label: '好友' },
          { key: 'groups', icon: <TeamOutlined />, label: '群聊' },
          { key: 'settings', icon: <SettingOutlined />, label: '设置' },
          { key: 'about', icon: <InfoCircleOutlined />, label: '关于' },
        ]}
      />

      <div className={styles.footer}>
        <div className={styles.themeRow}>
          <div className={styles.themeLabel}>
            <BulbOutlined style={{ marginRight: 6 }} />
            暗色主题
          </div>
          <Tooltip title={darkMode ? '切换亮色' : '切换暗色'}>
            <Switch
              size="small"
              checked={darkMode}
              onChange={setDarkMode}
            />
          </Tooltip>
        </div>
        <div className={styles.wsStatus}>
          <span
            className={styles.wsDot}
            style={{ backgroundColor: wsDotColor[wsStatus] }}
          />
          {wsStatusText[wsStatus]}
        </div>
        <Button
          block
          danger
          icon={<LogoutOutlined />}
          onClick={onLogout}
        >
          退出登录
        </Button>
      </div>
    </div>
  );
};

export default SettingsPanel;
