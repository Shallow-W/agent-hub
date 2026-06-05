import React, { useState } from 'react';
import { Modal, Tabs } from 'antd';
import { CodeOutlined, EyeOutlined, InfoCircleOutlined, RobotOutlined } from '@ant-design/icons';
import type { Artifact } from '@/types/message';
import { CodeBlock } from './CodeBlock';
import { WebpageFrame } from './WebpageFrame';
import styles from './ArtifactWorkspace.module.css';

interface Props {
  artifact: Artifact | null;
  open: boolean;
  onClose: () => void;
  /** 来源 Agent 名称（群聊多 Agent 时标识产物归属） */
  agentName?: string | null;
}

function artifactTitle(artifact: Artifact): string {
  return artifact.title || artifact.filename || (artifact.type === 'webpage' ? '网页产物' : '代码产物');
}

const PreviewView: React.FC<{ artifact: Artifact }> = ({ artifact }) => {
  if (artifact.type === 'webpage') {
    if (artifact.url) {
      return (
        <div className={styles.previewArea}>
          <WebpageFrame url={artifact.url} />
        </div>
      );
    }
    if (artifact.content) {
      return (
        <div className={styles.previewArea}>
          <WebpageFrame srcDoc={artifact.content} />
        </div>
      );
    }
  }
  return <div className={styles.emptyHint}>暂无可预览内容</div>;
};

const CodeView: React.FC<{ artifact: Artifact }> = ({ artifact }) => {
  if (!artifact.content) {
    return <div className={styles.emptyHint}>暂无源码内容</div>;
  }
  return (
    <div className={styles.codeArea}>
      <CodeBlock code={artifact.content} language={artifact.language} />
    </div>
  );
};

const MetaView: React.FC<{ artifact: Artifact; agentName?: string | null }> = ({ artifact, agentName }) => {
  const rows: Array<[string, React.ReactNode]> = [
    ['类型', artifact.type],
    ['版本', `v${artifact.version}`],
  ];
  if (artifact.language) rows.push(['语言', artifact.language]);
  if (artifact.filename) rows.push(['文件名', artifact.filename]);
  if (artifact.title) rows.push(['标题', artifact.title]);
  if (artifact.url) {
    rows.push([
      'URL',
      <a className={styles.metaLink} href={artifact.url} target="_blank" rel="noopener noreferrer">
        {artifact.url}
      </a>,
    ]);
  }
  if (agentName) rows.push(['来源 Agent', agentName]);

  return (
    <table className={styles.metaTable}>
      <tbody>
        {rows.map(([label, value]) => (
          <tr key={label}>
            <th>{label}</th>
            <td>{value}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
};

/**
 * 全屏产物工作区：点击内联卡片打开，提供 Preview / Code / Meta 三视图。
 * - code 产物默认 Code 视图；webpage 产物默认 Preview 视图。
 */
export const ArtifactWorkspace: React.FC<Props> = ({ artifact, open, onClose, agentName }) => {
  const defaultTab = artifact?.type === 'webpage' ? 'preview' : 'code';
  const [activeKey, setActiveKey] = useState(defaultTab);

  // 切换产物时重置默认视图
  React.useEffect(() => {
    setActiveKey(artifact?.type === 'webpage' ? 'preview' : 'code');
  }, [artifact?.id, artifact?.type]);

  if (!artifact) return null;

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width="80vw"
      style={{ top: 32, maxWidth: 1100 }}
      title={artifactTitle(artifact)}
      destroyOnClose
    >
      <div className={styles.modalBody}>
        <div className={styles.toolbar}>
          <span className={styles.source}>
            {agentName && (
              <>
                <RobotOutlined />
                <span className={styles.sourceAgent}>{agentName}</span>
              </>
            )}
          </span>
        </div>
        <Tabs
          activeKey={activeKey}
          onChange={setActiveKey}
          items={[
            {
              key: 'preview',
              label: (<span><EyeOutlined /> Preview</span>),
              children: (
                <div className={styles.viewArea}>
                  <PreviewView artifact={artifact} />
                </div>
              ),
            },
            {
              key: 'code',
              label: (<span><CodeOutlined /> Code</span>),
              children: (
                <div className={styles.viewArea}>
                  <CodeView artifact={artifact} />
                </div>
              ),
            },
            {
              key: 'meta',
              label: (<span><InfoCircleOutlined /> Meta</span>),
              children: (
                <div className={styles.viewArea}>
                  <MetaView artifact={artifact} agentName={agentName} />
                </div>
              ),
            },
          ]}
        />
      </div>
    </Modal>
  );
};
