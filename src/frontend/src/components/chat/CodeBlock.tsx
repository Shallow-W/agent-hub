import React, { useState, type ReactNode } from 'react';
import {
  CheckOutlined,
  CopyOutlined,
  ExpandOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { Input } from 'antd';
import { message as antMessage } from '@/utils/message';
import type { Artifact } from '@/types/message';
import { aiEditArtifact } from '@/api/artifact';
import { ApiError } from '@/api/client';
import { LANG_DISPLAY, highlightCode } from './highlight';
import { ArtifactEditor } from './ArtifactEditor';
import styles from './CodeBlock.module.css';

const { TextArea } = Input;

const SELECTION_PREVIEW_LIMIT = 240;

export function extractText(node: ReactNode): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (React.isValidElement(node)) {
    return extractText((node.props as { children?: ReactNode }).children);
  }
  return '';
}

interface CodeBlockProps {
  className?: string;
  children?: ReactNode;
  code?: string;
  language?: string;
  filename?: string;
  expandable?: boolean;
  artifactRootId?: string;
  /** 选区变化回调（ArtifactEditor 在 document 模式复用 CodeBlock 时接通 AI 选区）。 */
  onSelectionChange?: (text: string) => void;
}

/**
 * 内联代码块——轻量职责：语法高亮 + 复制 + AI 局部修改面板 + 展开按钮。
 *
 * 展开后的全功能工作台（版本/编辑/Diff/AI编辑）已抽离到 ArtifactEditor 组件，
 * 所有文本类产物（code/webpage/document）共享。本组件展开时构造 Artifact 对象
 * 交给 ArtifactEditor，自身不再承载工作台状态。
 */
export const CodeBlock: React.FC<CodeBlockProps> = ({
  className,
  children,
  code,
  language,
  filename,
  expandable = false,
  artifactRootId,
  onSelectionChange,
}) => {
  const lang = language ?? (className?.replace('language-', '') || '');
  const sourceCodeStr = (code ?? extractText(children)).replace(/\n$/, '');
  const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';

  const [copied, setCopied] = useState(false);
  const [editorOpen, setEditorOpen] = useState(false);

  // 内联 AI 局部修改（不展开时的轻量交互）
  const [selectedCode, setSelectedCode] = useState('');
  const [aiInstruction, setAiInstruction] = useState('');
  const [aiEditing, setAiEditing] = useState(false);
  const inlineCodeRef = React.useRef<HTMLPreElement>(null);

  const highlighted = React.useMemo(() => highlightCode(sourceCodeStr, lang || undefined), [sourceCodeStr, lang]);

  const captureSelectionFrom = (container: HTMLElement | null) => {
    const selection = window.getSelection();
    if (!selection || selection.rangeCount === 0 || !container) return;
    const range = selection.getRangeAt(0);
    if (!container.contains(range.commonAncestorContainer)) return;
    const text = selection.toString();
    const trimmed = text.trim() ? text : '';
    setSelectedCode(trimmed);
    if (onSelectionChange) onSelectionChange(trimmed);
  };

  const handleAIEdit = async () => {
    if (!artifactRootId || aiEditing) return;
    const instruction = aiInstruction.trim();
    if (!instruction) {
      antMessage.warning('先写一下你希望 AI 怎么改');
      return;
    }
    setAiEditing(true);
    try {
      await aiEditArtifact(artifactRootId, {
        instruction,
        ...(selectedCode.trim() ? { selection: selectedCode } : {}),
      });
      setAiInstruction('');
      setSelectedCode('');
      antMessage.success('AI 已生成新版本，点击「展开」查看');
    } catch (err) {
      antMessage.error(err instanceof ApiError ? err.message : 'AI 修改失败');
    } finally {
      setAiEditing(false);
    }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(sourceCodeStr).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => {
      antMessage.error('复制失败');
    });
  };

  const selectionPreview = selectedCode.length > SELECTION_PREVIEW_LIMIT
    ? `${selectedCode.slice(0, SELECTION_PREVIEW_LIMIT)}…`
    : selectedCode;

  const renderInlineAIEditPanel = () => (
    <div className={`${styles.aiEditPanel} ${styles.aiEditPanelCompact}`}>
      <div className={styles.aiEditMeta}>
        <span className={styles.aiEditTitle}><RobotOutlined /> AI 局部修改</span>
        <span className={styles.aiEditMetaRight}>
          <span className={styles.aiEditSelection}>
            {selectedCode.trim() ? `已选中 ${selectedCode.length} 个字符` : '未选中代码，将按整份修改'}
          </span>
          <button
            className={styles.aiEditSelectAll}
            type="button"
            disabled={aiEditing}
            onClick={() => setSelectedCode(sourceCodeStr)}
          >
            全选
          </button>
        </span>
      </div>
      {selectedCode.trim() && (
        <pre className={styles.aiEditPreview} title="当前选中内容">{selectionPreview}</pre>
      )}
      <div className={styles.aiEditControls}>
        <TextArea
          className={styles.aiEditInput}
          value={aiInstruction}
          onChange={(e) => setAiInstruction(e.target.value)}
          placeholder="描述你想怎么改选中的代码"
          autoSize={{ minRows: 1, maxRows: 3 }}
          disabled={aiEditing}
        />
        <button
          className={`${styles.codeActionBtn} ${styles.expandedCopyBtn} ${styles.aiEditButton}`}
          type="button"
          disabled={aiEditing}
          onClick={handleAIEdit}
        >
          <span className={styles.codeCopyIcon}><RobotOutlined /></span>
          <span className={styles.codeCopyText}>{aiEditing ? '修改中...' : '让 AI 改'}</span>
        </button>
      </div>
    </div>
  );

  // 展开时构造 Artifact 对象交给 ArtifactEditor。
  // root_id 为空时表示纯 markdown 代码块（未接产物系统），ArtifactEditor 据此
  // 降级为纯预览模式——不调版本/AI 编辑 API，避免后端 UUID 校验报错。
  const editorArtifact: Artifact | null = editorOpen ? {
    root_id: artifactRootId || undefined,
    version: 1,
    type: 'code',
    language: lang || undefined,
    filename,
    content: sourceCodeStr,
    created_at: '',
  } : null;

  return (
    <div className={styles.codeBlockWrapper}>
      <div className={styles.codeHeader}>
        <span>{displayLang}</span>
        <div className={styles.codeActions}>
          {expandable && (
            <button className={styles.codeActionBtn} type="button" title="展开代码" onClick={() => setEditorOpen(true)}>
              <span className={styles.codeCopyIcon}><ExpandOutlined /></span>
              <span className={styles.codeCopyText}>展开</span>
            </button>
          )}
          <button
            className={`${styles.codeActionBtn} ${copied ? styles.codeCopyBtnCopied : ''}`}
            type="button"
            title="复制代码"
            onClick={handleCopy}
          >
            <span className={styles.codeCopyIcon}>{copied ? <CheckOutlined /> : <CopyOutlined />}</span>
            <span className={styles.codeCopyText}>{copied ? '已复制' : '复制'}</span>
          </button>
        </div>
      </div>
      <pre
        ref={inlineCodeRef}
        className={styles.codeBlock}
        onMouseUp={() => captureSelectionFrom(inlineCodeRef.current)}
        onKeyUp={() => captureSelectionFrom(inlineCodeRef.current)}
      >
        <code dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
      {artifactRootId && selectedCode.trim() && !editorOpen && renderInlineAIEditPanel()}

      {editorArtifact && (
        <ArtifactEditor
          artifact={editorArtifact}
          open={editorOpen}
          onClose={() => setEditorOpen(false)}
        />
      )}
    </div>
  );
};
