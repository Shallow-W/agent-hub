import React, { useState } from 'react';
import { Avatar, Menu, Switch, Tooltip } from 'antd';
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
  collapsed: boolean;
}

const wsStatusText: Record<WsStatus, string> = {
  connected: '已连接',
  connecting: '连接中',
  disconnected: '已断开',
};

const wsDotColor: Record<WsStatus, string> = {
  connected: styles.connected ?? '',
  connecting: styles.connecting ?? '',
  disconnected: styles.disconnected ?? '',
};

const SettingsPanel: React.FC<SettingsPanelProps> = ({
  username,
  onLogout,
  wsStatus,
  onNavChange,
  collapsed,
}) => {
  const [selectedKey, setSelectedKey] = useState('chat');
  const [darkMode, setDarkMode] = useState(false);
  const initial = username ? username.charAt(0).toUpperCase() : '?';

  const handleMenuClick = (info: { key: string }) => {
    setSelectedKey(info.key);
    onNavChange(info.key);
  };

  return (
    <div className={`${styles.panel} ${collapsed ? styles.collapsed : ''}`}>
      <div className={styles.brand}>
        <div className={styles.brandIcon}>A</div>
        <span className={styles.brandName}>AgentHub</span>
      </div>

      <div className={styles.profile}>
        <Avatar
          className={styles.profileAvatar}
          size={34}
        >
          {initial}
        </Avatar>
        <span className={styles.profileName}>{username}</span>
      </div>

      <Menu
        mode="inline"
        inlineCollapsed={collapsed}
        selectedKeys={[selectedKey]}
        onClick={handleMenuClick}
        className={styles.navMenu}
        items={[
          { key: 'chat', icon: <MessageOutlined />, label: '对话' },
          { key: 'friends', icon: <UserAddOutlined />, label: '好友' },
          { key: 'groups', icon: <TeamOutlined />, label: '群聊' },
          { key: 'settings', icon: <SettingOutlined />, label: '设置' },
          { key: 'about', icon: <InfoCircleOutlined />, label: '关于' },
        ]}
      />

      <div className={styles.footer}>
        <div className={`${styles.themeRow} ${collapsed ? styles.themeRowCollapsed : ''}`}>
          <div className={styles.themeLabel}>
            <BulbOutlined className={styles.themeIcon} />
            <span className={styles.themeLabelText}>暗色主题</span>
          </div>
          <Switch
            size="small"
            checked={darkMode}
            onChange={setDarkMode}
          />
        </div>
        <div className={styles.wsStatus}>
          <Tooltip title={collapsed ? wsStatusText[wsStatus] : ''}>
            <span
              className={`${styles.wsDot} ${wsDotColor[wsStatus]}`}
            />
          </Tooltip>
          <span className={styles.wsStatusText}>{wsStatusText[wsStatus]}</span>
        </div>
        <Tooltip title={collapsed ? '退出登录' : ''}>
          <button className={styles.logoutBtn} onClick={onLogout}>
            <LogoutOutlined className={styles.logoutIcon} />
            <span className={styles.logoutText}>退出登录</span>
          </button>
        </Tooltip>
      </div>
    </div>
  );
};

export default SettingsPanel;
