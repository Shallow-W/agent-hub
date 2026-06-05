import React, { useState } from 'react';
import { CodeOutlined, ExpandOutlined, GlobalOutlined } from '@ant-design/icons';
import type { Artifact } from '@/types/message';
import { CodeBlock } from './CodeBlock';
import { WebpageFrame } from './WebpageFrame';
import { ArtifactWorkspace } from './ArtifactWorkspace';
import styles from './ArtifactCard.module.css';

interface Props {
  artifacts: Artifact[];
  /** 来源 Agent 名称（群聊多 Agent 时标识产物归属） */
  agentName?: string | null;
}

function codeTitle(a: Artifact): string {
  return a.filename || (a.language ? `${a.language} 代码` : '代码产物');
}

function webTitle(a: Artifact): string {
  if (a.title) return a.title;
  if (a.url) {
    try {
      return new URL(a.url).hostname;
    } catch {
      return a.url;
    }
  }
  return '网页产物';
}

/**
 * 聊天流内联产物卡片区。
 * - code 产物复用 CodeBlock（高亮+复制+折叠由 CodeBlock 内部承担），不重造代码卡。
 * - webpage 产物展示标题/URL + 缩略 iframe 预览。
 * - 点击展开按钮 / webpage 卡片打开全屏 ArtifactWorkspace。
 */
export const ArtifactCard: React.FC<Props> = ({ artifacts, agentName }) => {
  const [active, setActive] = useState<Artifact | null>(null);

  if (!artifacts || artifacts.length === 0) return null;

  return (
    <div className={styles.container}>
      {artifacts.map((artifact, idx) => {
        const key = artifact.id ?? `artifact-${idx}`;
        if (artifact.type === 'code') {
          return (
            <div key={key} className={styles.codeCard}>
              <div className={styles.cardBar}>
                <span className={styles.cardBarLeft}>
                  <CodeOutlined />
                  <span className={styles.cardTitle}>{codeTitle(artifact)}</span>
                  <span className={styles.cardBadge}>产物</span>
                </span>
                <button
                  type="button"
                  className={styles.expandBtn}
                  onClick={() => setActive(artifact)}
                  title="全屏查看"
                >
                  <ExpandOutlined />
                  展开
                </button>
              </div>
              <CodeBlock code={artifact.content ?? ''} language={artifact.language} />
            </div>
          );
        }

        // webpage 卡片
        return (
          <div
            key={key}
            className={styles.webCard}
            role="button"
            tabIndex={0}
            onClick={() => setActive(artifact)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                setActive(artifact);
              }
            }}
          >
            <div className={styles.webPreview}>
              {artifact.url ? (
                <WebpageFrame url={artifact.url} />
              ) : artifact.content ? (
                <WebpageFrame srcDoc={artifact.content} />
              ) : null}
            </div>
            <div className={styles.webMeta}>
              <GlobalOutlined className={styles.webIcon} />
              <div className={styles.webInfo}>
                <span className={styles.webTitle}>{webTitle(artifact)}</span>
                {artifact.url && <span className={styles.webUrl}>{artifact.url}</span>}
              </div>
              <ExpandOutlined />
            </div>
          </div>
        );
      })}

      <ArtifactWorkspace
        artifact={active}
        open={active !== null}
        onClose={() => setActive(null)}
        agentName={agentName}
      />
    </div>
  );
};
