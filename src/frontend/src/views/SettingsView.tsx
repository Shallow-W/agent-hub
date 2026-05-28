import React from 'react';
import { Avatar, Button, Descriptions, Divider } from 'antd';
import { LogoutOutlined, UserOutlined } from '@ant-design/icons';
import { useAuthStore } from '@/store/authStore';
import { useNavigate } from 'react-router-dom';
import styles from './SettingsView.module.css';

const SettingsView: React.FC = () => {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    navigate('/login', { replace: true });
  };

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
          <h4 className={styles.sectionTitle}>账户</h4>
          <p className={styles.sectionDesc}>账户管理功能即将上线</p>
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
