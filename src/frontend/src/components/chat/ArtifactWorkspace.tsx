import React, { useState, type ReactNode } from 'react';
import { Button, Modal, Tabs, Tooltip } from 'antd';
import {
  CodeOutlined,
  EyeOutlined,
  FullscreenExitOutlined,
  FullscreenOutlined,
  InfoCircleOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import type { Components } from 'react-markdown';
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

function isPreviewableDocument(artifact?: Artifact | null): boolean {
  if (!artifact) return false;
  return artifact.type === 'document' || artifact.type === 'file' || isMarkdownArtifact(artifact);
}

function markdownText(children: ReactNode): string {
  if (children == null || typeof children === 'boolean') return '';
  if (typeof children === 'string' || typeof children === 'number') return String(children);
  if (Array.isArray(children)) return children.map(markdownText).join('');
  if (React.isValidElement<{ children?: ReactNode }>(children)) {
    return markdownText(children.props.children);
  }
  return '';
}

function looksLikeMarkdownDocument(content: string): boolean {
  const src = content.trim();
  if (src.length < 40) return false;
  const headingMatches = src.match(/^#{1,3}\s+\S.+$/gm) || [];
  if (headingMatches.length === 0) return false;
  if (headingMatches.length >= 2) return true;
  return /(^|\n)(?:[-*]\s+\S|\|.+\||```)/.test(src);
}

function unwrapMarkdownDocumentFence(content: string): string {
  const normalized = content.replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  const first = lines.findIndex((line) => line.trim() !== '');
  let last = lines.length - 1;
  while (last >= 0 && (lines[last] ?? '').trim() === '') last -= 1;

  if (first < 0 || last <= first) return content;
  const firstLine = lines[first] ?? '';
  const lastLine = lines[last] ?? '';
  const opener = firstLine.match(/^ {0,3}`{3,}\s*(markdown|md)\s*$/i);
  if (!opener || !/^ {0,3}`{3,}\s*$/.test(lastLine)) return content;
  return lines.slice(first + 1, last).join('\n').replace(/\s+$/, '');
}

function markdownDocumentContent(content: string): string {
  const unwrapped = unwrapMarkdownDocumentFence(content);
  if (unwrapped !== content) return unwrapped;

  const normalized = content.replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  for (let i = 0; i < lines.length; i += 1) {
    if (!/^ {0,3}`{3,}\s*(markdown|md)\s*$/i.test(lines[i] ?? '')) continue;
    for (let j = lines.length - 1; j > i; j -= 1) {
      if (!/^ {0,3}`{3,}\s*$/.test(lines[j] ?? '')) continue;
      const candidate = lines.slice(i + 1, j).join('\n').replace(/\s+$/, '');
      if (looksLikeMarkdownDocument(candidate)) return candidate;
      break;
    }
  }
  const headingStart = normalized.search(/^#{1,3}\s+\S.+$/m);
  if (headingStart > 0) {
    const candidate = normalized.slice(headingStart).replace(/\s+$/, '');
    if (looksLikeMarkdownDocument(candidate)) return candidate;
  }
  return content;
}

const MarkdownPreview: React.FC<{ content: string }> = ({ content }) => {
  const [checkedItems, setCheckedItems] = useState<Record<number, boolean>>({});
  let checkboxIndex = 0;
  const displayContent = markdownDocumentContent(content);
  const markdownComponents: Components = {
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
    code: ({ children }) => (
      <span className={styles.documentCodeText}>{children}</span>
    ),
    pre: ({ children }) => {
      const text = markdownText(children);
      if (looksLikeMarkdownDocument(text)) {
        return (
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
            {markdownDocumentContent(text)}
          </ReactMarkdown>
        );
      }
      return (
        <div className={styles.documentPlainText}>
          {text}
        </div>
      );
    },
  };

  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={markdownComponents}
    >
      {displayContent}
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

  if (artifact.type === 'document' || artifact.type === 'file' || isMarkdownArtifact(artifact)) {
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
  const defaultTab = artifact?.type === 'webpage' || isPreviewableDocument(artifact)
    ? 'preview'
    : 'code';
  const [activeKey, setActiveKey] = useState(defaultTab);
  const [webpageExpanded, setWebpageExpanded] = useState(false);

  React.useEffect(() => {
    setActiveKey(
      artifact?.type === 'webpage' || isPreviewableDocument(artifact)
        ? 'preview'
        : 'code',
    );
    setWebpageExpanded(false);
  }, [artifact?.id, artifact?.type, artifact?.language, artifact?.filename]);

  if (!artifact) return null;

  if (artifact.type === 'webpage') {
    const bodySizeClass = webpageExpanded ? styles.webpageModalBodyExpanded : styles.webpageModalBodyCompact;

    return (
      <Modal
        open={open}
        onCancel={onClose}
        footer={null}
        width={webpageExpanded ? '94vw' : 'min(76vw, 980px)'}
        style={{ top: webpageExpanded ? 16 : 48, maxWidth: 'none' }}
        className={`${styles.workspaceModal} ${styles.webpageModal}`}
        destroyOnHidden
      >
        <div className={`${styles.modalBody} ${styles.webpageModalBody} ${bodySizeClass}`}>
          <div className={styles.webpageControls}>
            <Tooltip title={webpageExpanded ? '还原' : '全屏'}>
              <Button
                type="text"
                size="small"
                icon={webpageExpanded ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                aria-label={webpageExpanded ? '还原预览大小' : '全屏预览'}
                className={styles.webpageControlButton}
                onClick={() => setWebpageExpanded((value) => !value)}
              />
            </Tooltip>
          </div>
          <PreviewView artifact={artifact} />
        </div>
      </Modal>
    );
  }

  const isDocument = isPreviewableDocument(artifact);

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
        destroyOnHidden
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
      destroyOnHidden
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
