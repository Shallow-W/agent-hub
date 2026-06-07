import React, { useState } from 'react';
import {
  ExpandOutlined,
  FileOutlined,
  FileTextOutlined,
  GlobalOutlined,
} from '@ant-design/icons';
import type { Artifact } from '@/types/message';
import { WebpageFrame } from './WebpageFrame';
import { ArtifactWorkspace } from './ArtifactWorkspace';
import styles from './ArtifactCard.module.css';

interface Props {
  artifacts: Artifact[];
  /** 来源 Agent 名称，用于群聊多 Agent 时标识产物归属。 */
  agentName?: string | null;
}

function artifactTitle(artifact: Artifact): string {
  if (artifact.title) return artifact.title;
  if (artifact.filename) return artifact.filename;
  if (artifact.url) {
    try {
      return new URL(artifact.url).hostname;
    } catch {
      return artifact.url;
    }
  }
  if (artifact.type === 'document') return '文档产物';
  if (artifact.type === 'file') return '文件产物';
  return '网页产物';
}

function documentSummary(artifact: Artifact): string {
  if (artifact.language) return artifact.language;
  if (artifact.filename) return artifact.filename.split('.').pop()?.toUpperCase() || 'DOCUMENT';
  return artifact.content ? `${artifact.content.length} 字符` : '暂不可预览';
}

export const ArtifactCard: React.FC<Props> = ({ artifacts, agentName }) => {
  const [active, setActive] = useState<Artifact | null>(null);

  if (!artifacts || artifacts.length === 0) return null;

  return (
    <div className={styles.container}>
      {artifacts.map((artifact, idx) => {
        const key = artifact.id ?? `artifact-${idx}`;

        if (artifact.type === 'webpage') {
          return (
            <div
              key={key}
              className={styles.webCard}
              role="button"
              tabIndex={0}
              onClick={() => setActive(artifact)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
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
                  <span className={styles.webTitle}>{artifactTitle(artifact)}</span>
                  {artifact.url && <span className={styles.webUrl}>{artifact.url}</span>}
                </div>
                <ExpandOutlined />
              </div>
            </div>
          );
        }

        const Icon = artifact.type === 'document' ? FileTextOutlined : FileOutlined;
        return (
          <div
            key={key}
            className={styles.documentCard}
            role="button"
            tabIndex={0}
            onClick={() => setActive(artifact)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                setActive(artifact);
              }
            }}
          >
            <Icon className={styles.documentIcon} />
            <div className={styles.documentInfo}>
              <span className={styles.documentTitle}>{artifactTitle(artifact)}</span>
              <span className={styles.documentSummary}>{documentSummary(artifact)}</span>
            </div>
            <ExpandOutlined />
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
