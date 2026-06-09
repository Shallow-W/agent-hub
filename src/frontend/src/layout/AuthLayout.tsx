import React from 'react';
import { RobotOutlined } from '@ant-design/icons';
import TitleBar from '@/components/common/TitleBar';
import styles from './AuthLayout.module.css';

interface AuthLayoutProps {
  children: React.ReactNode;
}

const AuthLayout: React.FC<AuthLayoutProps> = ({ children }) => {
  return (
    <div className={styles.container}>
      <TitleBar />
      <div className={styles.card}>
        <div className={styles.logo}>
          <div className={styles.logoIcon}>
            <RobotOutlined />
          </div>
          <h1>AgentHub</h1>
          <p>多 Agent 协作平台</p>
        </div>
        {children}
      </div>
    </div>
  );
};

export default AuthLayout;
