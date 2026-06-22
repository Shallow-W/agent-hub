import React, { useState } from 'react';
import { Button, Tooltip } from 'antd';
import { FolderOpenOutlined } from '@ant-design/icons';
import type { CardProps, ProjectCard as ProjectCardData } from '@/types/card';
import { FilesDrawer } from '@/components/chat/FilesDrawer';
import styles from './Cards.module.css';

/**
 * 项目目录卡片——只读。
 *
 * 解耦设计的关键一环：agent 写完文件后通过 render_card(card_type=project, work_dir=...)
 * 上报工作目录，此卡片携带 workDir 作为路径生产与文件浏览之间的唯一契约。
 * 点击卡片打开 FilesDrawer 浏览该目录（抽屉只认 workDir，不关心它怎么来的）。
 *
 * 自包含模式：卡片内部 useState 控制 FilesDrawer（同 DiffCard 控制 DiffViewer 的范本），
 * 不走 onAction / 不依赖 ChatWindow 单例——避免卡片协议承载抽屉控制逻辑。
 *
 * 需要 agentId（调 daemon RPC），从 CardProps.agentId 取（MessageBubble 透传）。
 */
export const ProjectCard: React.FC<CardProps<ProjectCardData>> = ({ card, agentId }) => {
  const [open, setOpen] = useState(false);

  // workDir 是路径生产与文件浏览的唯一契约；缺失时无法浏览，不渲染卡片避免误导。
  // （正常情况 agent 总会带 work_dir；此守卫防御 agent 漏传或数据损坏）
  if (!card.workDir) return null;

  const handleClick = () => {
    // agentId 缺失（如系统消息）时无法调 daemon RPC，禁用打开
    if (!agentId) return;
    setOpen(true);
  };

  return (
    <>
      <div
        className={styles.projectCard}
        role="button"
        tabIndex={0}
        onClick={handleClick}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            handleClick();
          }
        }}
      >
        <FolderOpenOutlined className={styles.projectIcon} />
        <div className={styles.projectMeta}>
          <div className={styles.projectTitle}>{card.title || '项目目录'}</div>
          <Tooltip title={card.workDir}>
            <div className={styles.projectPath}>{card.workDir}</div>
          </Tooltip>
          {card.summary && <div className={styles.projectSummary}>{card.summary}</div>}
        </div>
        <Button
          type="primary"
          size="small"
          ghost
          icon={<FolderOpenOutlined />}
          disabled={!agentId}
          onClick={(e) => {
            e.stopPropagation();
            handleClick();
          }}
        >
          浏览文件
        </Button>
      </div>
      {agentId && (
        <FilesDrawer
          agentId={agentId}
          workDir={card.workDir}
          open={open}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
};
