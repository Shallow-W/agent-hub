import React, { useCallback, useEffect, useState } from 'react';
import { Avatar, Button, Descriptions, Divider, Segmented, Switch, Tag } from 'antd';
import {
  LogoutOutlined,
  SunOutlined,
  MoonOutlined,
  DesktopOutlined,
  BellOutlined,
  SoundOutlined,
  InfoCircleOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '@/store/authStore';
import { useNavigate } from 'react-router-dom';
import styles from './SettingsView.module.css';

type ThemeMode = 'light' | 'dark' | 'system';

function getInitialTheme(): ThemeMode {
  const saved = localStorage.getItem('theme');
  if (saved === 'light' || saved === 'dark' || saved === 'system') return saved;
  return 'light';
}

function applyTheme(mode: ThemeMode) {
  if (mode === 'system') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    document.documentElement.classList.toggle('dark', prefersDark);
  } else {
    document.documentElement.classList.toggle('dark', mode === 'dark');
  }
  localStorage.setItem('theme', mode);
}

function getInitialBool(key: string, fallback: boolean): boolean {
  const v = localStorage.getItem(key);
  if (v === null) return fallback;
  return v === 'true';
}

const SettingsView: React.FC = () => {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();

  const [theme, setTheme] = useState<ThemeMode>(getInitialTheme);
  const [notifySound, setNotifySound] = useState(() => getInitialBool('agenthub_notify_sound', true));
  const [notifyDesktop, setNotifyDesktop] = useState(() => getInitialBool('agenthub_notify_desktop', false));

  const handleLogout = () => {
    logout();
    navigate('/login', { replace: true });
  };

  const handleThemeChange = useCallback((value: string | number) => {
    const mode = value as ThemeMode;
    setTheme(mode);
    applyTheme(mode);
  }, []);

  useEffect(() => {
    if (theme !== 'system') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => applyTheme('system');
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  }, [theme]);

  const handleNotifySound = useCallback((checked: boolean) => {
    setNotifySound(checked);
    localStorage.setItem('agenthub_notify_sound', String(checked));
  }, []);

  const handleNotifyDesktop = useCallback(async (checked: boolean) => {
    if (checked && 'Notification' in window) {
      const perm = await Notification.requestPermission();
      if (perm !== 'granted') {
        setNotifyDesktop(false);
        localStorage.setItem('agenthub_notify_desktop', 'false');
        return;
      }
    }
    setNotifyDesktop(checked);
    localStorage.setItem('agenthub_notify_desktop', String(checked));
  }, []);

  if (!user) {
    return (
      <div className={styles.empty}>
        <span className={styles.hint}>未登录</span>
      </div>
    );
  }

  const initial = user.username.charAt(0).toUpperCase();
  const memberSince = new Date(user.created_at).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  });

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <span className={styles.title}>设置</span>
      </div>

      <div className={styles.body}>
        <div className={styles.profileCard}>
          <Avatar size={56} className={styles.avatar} icon={<UserOutlined />}>
            {initial}
          </Avatar>
          <div className={styles.profileInfo}>
            <span className={styles.username}>{user.username}</span>
            <span className={styles.userId}>ID: {user.id}</span>
          </div>
        </div>

        <Divider style={{ margin: '16px 0' }} />

        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="用户名">{user.username}</Descriptions.Item>
          <Descriptions.Item label="用户 ID">{user.id}</Descriptions.Item>
          <Descriptions.Item label="注册时间">{memberSince}</Descriptions.Item>
        </Descriptions>

        <Divider style={{ margin: '16px 0' }} />

        <div className={styles.section}>
          <div className={styles.sectionHeader}>
            <SunOutlined className={styles.sectionIcon} />
            <h4 className={styles.sectionTitle}>外观</h4>
          </div>
          <div className={styles.settingRow}>
            <span className={styles.settingLabel}>主题</span>
            <Segmented
              value={theme}
              onChange={handleThemeChange}
              options={[
                { label: '浅色', value: 'light', icon: <SunOutlined /> },
                { label: '深色', value: 'dark', icon: <MoonOutlined /> },
                { label: '跟随系统', value: 'system', icon: <DesktopOutlined /> },
              ]}
            />
          </div>
        </div>

        <Divider style={{ margin: '16px 0' }} />

        <div className={styles.section}>
          <div className={styles.sectionHeader}>
            <BellOutlined className={styles.sectionIcon} />
            <h4 className={styles.sectionTitle}>通知</h4>
          </div>
          <div className={styles.settingRow}>
            <span className={styles.settingLabel}>
              <SoundOutlined className={styles.settingLabelIcon} />
              消息通知声音
            </span>
            <Switch checked={notifySound} onChange={handleNotifySound} />
          </div>
          <div className={styles.settingRow}>
            <span className={styles.settingLabel}>
              <DesktopOutlined className={styles.settingLabelIcon} />
              桌面通知
            </span>
            <Switch checked={notifyDesktop} onChange={handleNotifyDesktop} />
          </div>
        </div>

        <Divider style={{ margin: '16px 0' }} />

        <div className={styles.section}>
          <div className={styles.sectionHeader}>
            <InfoCircleOutlined className={styles.sectionIcon} />
            <h4 className={styles.sectionTitle}>关于</h4>
          </div>
          <div className={styles.aboutItem}>
            <span className={styles.aboutLabel}>应用名称</span>
            <span className={styles.aboutValue}>AgentHub</span>
          </div>
          <div className={styles.aboutItem}>
            <span className={styles.aboutLabel}>版本</span>
            <Tag>v0.1.0</Tag>
          </div>
          <div className={styles.aboutItem}>
            <span className={styles.aboutLabel}>技术栈</span>
            <span className={styles.aboutValue}>React 18 / Go / PostgreSQL / Redis</span>
          </div>
        </div>

        <Divider style={{ margin: '16px 0' }} />

        <Button
          danger
          icon={<LogoutOutlined />}
          onClick={handleLogout}
          block
        >
          退出登录
        </Button>
      </div>
    </div>
  );
};

export default SettingsView;
