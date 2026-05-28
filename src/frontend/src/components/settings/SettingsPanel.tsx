import React, { useEffect } from 'react';
import { Avatar, Tooltip } from 'antd';
import {
  AppstoreOutlined,
  BgColorsOutlined,
  CodeOutlined,
  DatabaseOutlined,
  MessageOutlined,
  LogoutOutlined,
  PlusOutlined,
  RobotOutlined,
  SettingOutlined,
  TeamOutlined,
  UserAddOutlined,
} from '@ant-design/icons';
import styles from './SettingsPanel.module.css';

type WsStatus = 'connected' | 'connecting' | 'disconnected';

interface SettingsPanelProps {
  username: string;
  onLogout: () => void;
  wsStatus: WsStatus;
  onNavChange: (key: string) => void;
  activeKey: string;
  onCreate: () => void;
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

const navItems = [
  { key: 'chat', label: '消息', icon: <MessageOutlined /> },
  { key: 'friends', label: '好友', icon: <UserAddOutlined /> },
  { key: 'groups', label: '群聊', icon: <TeamOutlined /> },
  { key: 'models', label: '模型', icon: <RobotOutlined /> },
  { key: 'skills', label: '技能', icon: <CodeOutlined /> },
  { key: 'workspace', label: '任务', icon: <AppstoreOutlined /> },
  { key: 'knowledge', label: '知识', icon: <DatabaseOutlined /> },
];

const SettingsPanel: React.FC<SettingsPanelProps> = ({
  username,
  onLogout,
  wsStatus,
  onNavChange,
  activeKey,
  onCreate,
  collapsed,
}) => {
  const initial = username ? username.charAt(0).toUpperCase() : '?';

  const handleNavClick = (key: string) => {
    onNavChange(key);
  };

  useEffect(() => {
    const saved = localStorage.getItem('theme');
    if (saved === 'dark') {
      document.documentElement.classList.add('dark');
    } else if (saved === 'system' || !saved) {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      document.documentElement.classList.toggle('dark', prefersDark);
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, []);

  return (
    <div className={`${styles.panel} ${collapsed ? styles.collapsed : ''}`}>
      <div className={styles.brand}>
        <div className={styles.brandIcon}>A</div>
        <span className={styles.brandName}>AgentHub</span>
      </div>

      <div className={styles.quickCreate}>
        <Tooltip title="新建">
          <button className={styles.createBtn} type="button" onClick={onCreate}>
            <PlusOutlined />
          </button>
        </Tooltip>
      </div>

      <nav className={styles.navMenu} aria-label="主导航">
        {navItems.map((item) => (
          <Tooltip key={item.key} title={collapsed ? item.label : ''} placement="right">
            <button
              className={`${styles.navItem} ${activeKey === item.key ? styles.navItemActive : ''}`}
              type="button"
              onClick={() => handleNavClick(item.key)}
            >
              <span className={styles.navIcon}>{item.icon}</span>
              <span className={styles.navLabel}>{item.label}</span>
            </button>
          </Tooltip>
        ))}
      </nav>

      <div className={styles.footer}>
        <div className={styles.wsStatus}>
          <Tooltip title={collapsed ? wsStatusText[wsStatus] : ''}>
            <span
              className={`${styles.wsDot} ${wsDotColor[wsStatus]}`}
            />
          </Tooltip>
          <span className={styles.wsStatusText}>{wsStatusText[wsStatus]}</span>
        </div>
        <Tooltip title={username || '个人头像'} placement="right">
          <Avatar className={styles.footerAvatar} size={24}>{initial}</Avatar>
        </Tooltip>
        <Tooltip title="调色板" placement="right">
          <button className={styles.footerIconBtn} type="button" aria-label="调色板">
            <BgColorsOutlined />
          </button>
        </Tooltip>
        <Tooltip title="设置" placement="right">
          <button
            className={styles.footerIconBtn}
            type="button"
            onClick={() => handleNavClick('settings')}
          >
            <SettingOutlined />
          </button>
        </Tooltip>
        <Tooltip title="退出登录" placement="right">
          <button className={styles.footerIconBtn} type="button" onClick={onLogout}>
            <LogoutOutlined />
          </button>
        </Tooltip>
      </div>
    </div>
  );
};

export default SettingsPanel;
