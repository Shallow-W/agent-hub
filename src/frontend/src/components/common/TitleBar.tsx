import React, { useEffect, useState } from 'react';
import {
  MinusOutlined,
  BorderOutlined,
  BlockOutlined,
  CloseOutlined,
} from '@ant-design/icons';
import styles from './TitleBar.module.css';

declare global {
  interface Window {
    agentHubDesktop?: {
      platform: string;
      isDesktop: boolean;
      minimize: () => void;
      maximize: () => void;
      close: () => void;
      onMaximizeChange: (cb: (maximized: boolean) => void) => () => void;
    };
  }
}

/* 复用 favicon.svg 的设计，缩放为标题栏尺寸 */
const AppLogo: React.FC = () => (
  <div className={styles.logoIcon}>
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
      <rect width="64" height="64" rx="14" fill="#111827" />
      <path d="M18 42V22h7v7h14v-7h7v20h-7v-7H25v7z" fill="#f9fafb" />
      <circle cx="20" cy="18" r="5" fill="#22c55e" />
      <circle cx="44" cy="46" r="5" fill="#38bdf8" />
    </svg>
  </div>
);

const TitleBar: React.FC = () => {
  const desktop = window.agentHubDesktop;
  // 仅桌面端显示自定义标题栏
  if (!desktop?.isDesktop) return null;

  return <TitleBarInner />;
};

const TitleBarInner: React.FC = () => {
  const desktop = window.agentHubDesktop!;
  const [isMaximized, setIsMaximized] = useState(false);

  useEffect(() => {
    const cleanup = desktop.onMaximizeChange((maximized) => {
      setIsMaximized(maximized);
    });
    return cleanup;
  }, [desktop]);

  // 最大化状态同步到 body，让布局层据此切换圆角
  useEffect(() => {
    document.body.classList.toggle('ah-maximized', isMaximized);
  }, [isMaximized]);

  return (
    <div className={styles.titleBar}>
      {/* 拖拽区域：占满整行，双击可切换最大化 */}
      <div
        className={styles.dragRegion}
        onDoubleClick={desktop.maximize}
      >
        <AppLogo />
        <span className={styles.titleText}>AgentHub</span>
      </div>

      {/* 窗口控制按钮 */}
      <div className={styles.controls}>
        <button
          className={styles.controlBtn}
          onClick={desktop.minimize}
          aria-label="最小化"
        >
          <MinusOutlined />
        </button>
        <button
          className={styles.controlBtn}
          onClick={desktop.maximize}
          aria-label={isMaximized ? '还原' : '最大化'}
        >
          {isMaximized ? <BlockOutlined /> : <BorderOutlined />}
        </button>
        <button
          className={`${styles.controlBtn} ${styles.closeBtn}`}
          onClick={desktop.close}
          aria-label="关闭"
        >
          <CloseOutlined />
        </button>
      </div>
    </div>
  );
};

export default TitleBar;
