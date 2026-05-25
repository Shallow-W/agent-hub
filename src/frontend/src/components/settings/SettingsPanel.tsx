import React from 'react';
import styles from './SettingsPanel.module.css';

type WsStatus = 'connected' | 'connecting' | 'disconnected';

interface NavItem {
  icon: string;
  label: string;
  active?: boolean;
}

interface SettingsPanelProps {
  username: string;
  onLogout: () => void;
  wsStatus: WsStatus;
}

const navItems: NavItem[] = [
  { icon: '\u{1F4AC}', label: '对话', active: true },
  { icon: '⚙️', label: '设置' },
  { icon: 'ℹ️', label: '关于' },
];

const wsStatusText: Record<WsStatus, string> = {
  connected: '已连接',
  connecting: '连接中',
  disconnected: '已断开',
};

const wsDotClassMap: Record<WsStatus, string> = {
  connected: styles.wsConnected ?? '',
  connecting: styles.wsConnecting ?? '',
  disconnected: styles.wsDisconnected ?? '',
};

const SettingsPanel: React.FC<SettingsPanelProps> = ({
  username,
  onLogout,
  wsStatus,
}) => {
  const initial = username ? username.charAt(0).toUpperCase() : '?';

  return (
    <div className={styles.panel}>
      {/* 品牌标识 */}
      <div className={styles.brand}>
        <div className={styles.brandIcon}>A</div>
        <span className={styles.brandName}>AgentHub</span>
      </div>

      {/* 用户信息 */}
      <div className={styles.profile}>
        <div className={styles.avatar}>{initial}</div>
        <span className={styles.profileName}>{username}</span>
      </div>

      {/* 导航列表 */}
      <ul className={styles.nav}>
        {navItems.map((item) => (
          <li key={item.label}>
            <button
              className={`${styles.navItem} ${item.active ? styles.navItemActive : ''}`}
              type="button"
            >
              <span className={styles.navIcon}>{item.icon}</span>
              {item.label}
            </button>
          </li>
        ))}
      </ul>

      {/* 底部：连接状态 + 退出 */}
      <div className={styles.footer}>
        <div className={styles.wsStatus}>
          <span className={`${styles.wsDot} ${wsDotClassMap[wsStatus]}`} />
          {wsStatusText[wsStatus]}
        </div>
        <button
          className={styles.logoutBtn}
          onClick={onLogout}
          type="button"
        >
          退出登录
        </button>
      </div>
    </div>
  );
};

export default SettingsPanel;
