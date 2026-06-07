import React, { useState } from 'react';
import { Modal, Tabs } from 'antd';
import { CodeOutlined, EyeOutlined, InfoCircleOutlined, RobotOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Artifact } from '@/types/message';
import { CodeBlock } from './CodeBlock';
import { WebpageFrame } from './WebpageFrame';
import { DeployButton } from './DeployButton';
import styles from './ArtifactWorkspace.module.css';

interface Props {
  artifact: Artifact | null;
  open: boolean;
  onClose: () => void;
  /** 来源 Agent 名称，用于群聊多 Agent 时标识产物归属。 */
  agentName?: string | null;
}

function artifactTitle(artifact: Artifact): string {
  return artifact.title || artifact.filename || (artifact.type === 'document' ? '文档产物' : '代码产物');
}

function isMarkdownArtifact(artifact: Artifact): boolean {
  const language = artifact.language?.toLowerCase();
  const filename = artifact.filename?.toLowerCase();
  return language === 'markdown' || language === 'md' || filename?.endsWith('.md') || filename?.endsWith('.markdown') || false;
}

const MarkdownPreview: React.FC<{ content: string }> = ({ content }) => {
  const [checkedItems, setCheckedItems] = useState<Record<number, boolean>>({});
  let checkboxIndex = 0;

  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        input: (props) => {
          if (props.type !== 'checkbox') {
            return <input {...props} />;
          }
          const index = checkboxIndex;
          checkboxIndex += 1;
          const checked = checkedItems[index] ?? Boolean(props.checked);

          return (
            <input
              {...props}
              disabled={false}
              checked={checked}
              className={styles.taskCheckbox}
              onChange={(event) => {
                setCheckedItems((current) => ({
                  ...current,
                  [index]: event.target.checked,
                }));
              }}
            />
          );
        },
      }}
    >
      {content}
    </ReactMarkdown>
  );
};

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

  if (artifact.type === 'document' || artifact.type === 'file') {
    if (!artifact.content) {
      return (
        <div className={styles.emptyHint}>
          这个文件暂时没有可预览内容，后续可接入下载或文件服务。
        </div>
      );
    }
    if (isMarkdownArtifact(artifact)) {
      return (
        <div className={styles.documentArea}>
          <MarkdownPreview content={artifact.content} />
        </div>
      );
    }
    return (
      <pre className={styles.textDocument}>
        {artifact.content}
      </pre>
    );
  }

  return <div className={styles.emptyHint}>暂无可预览内容</div>;
};

const CodeView: React.FC<{ artifact: Artifact }> = ({ artifact }) => {
  if (!artifact.content) {
    return <div className={styles.emptyHint}>暂无源码内容</div>;
  }
  return (
    <div className={styles.codeArea}>
      <CodeBlock code={artifact.content} language={artifact.language} filename={artifact.filename} expandable />
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

export const ArtifactWorkspace: React.FC<Props> = ({ artifact, open, onClose, agentName }) => {
  const defaultTab = artifact?.type === 'webpage' || artifact?.type === 'document' || artifact?.type === 'file'
    ? 'preview'
    : 'code';
  const [activeKey, setActiveKey] = useState(defaultTab);

  React.useEffect(() => {
    setActiveKey(
      artifact?.type === 'webpage' || artifact?.type === 'document' || artifact?.type === 'file'
        ? 'preview'
        : 'code',
    );
  }, [artifact?.id, artifact?.type]);

  if (!artifact) return null;

  if (artifact.type === 'webpage') {
    return (
      <Modal
        open={open}
        onCancel={onClose}
        footer={null}
        width="94vw"
        style={{ top: 16, maxWidth: 'none' }}
        className={`${styles.workspaceModal} ${styles.webpageModal}`}
        destroyOnClose
      >
        <div className={`${styles.modalBody} ${styles.webpageModalBody}`}>
          <PreviewView artifact={artifact} />
        </div>
      </Modal>
    );
  }

  const isDocument = artifact.type === 'document' || artifact.type === 'file';

  if (isDocument) {
    return (
      <Modal
        open={open}
        onCancel={onClose}
        footer={null}
        width="94vw"
        style={{ top: 16, maxWidth: 'none' }}
        title={artifactTitle(artifact)}
        className={`${styles.workspaceModal} ${styles.documentModal}`}
        destroyOnClose
      >
        <div className={`${styles.modalBody} ${styles.documentModalBody}`}>
          <div className={styles.documentViewArea}>
            <PreviewView artifact={artifact} />
          </div>
        </div>
      </Modal>
    );
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width="80vw"
      style={{ top: 32, maxWidth: 1100 }}
      title={artifactTitle(artifact)}
      className={styles.workspaceModal}
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
          <DeployButton artifact={artifact} />
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
