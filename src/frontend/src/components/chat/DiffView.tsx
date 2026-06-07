import React, { useEffect, useRef } from 'react';
import { EditorState, type Extension } from '@codemirror/state';
import { EditorView, lineNumbers } from '@codemirror/view';
import { MergeView } from '@codemirror/merge';
import { oneDark } from '@codemirror/theme-one-dark';
import { languageExtension } from './codemirrorLang';
import styles from './DiffView.module.css';

interface DiffViewProps {
  /** 旧版本内容（左栏）。 */
  oldDoc: string;
  /** 新版本内容（右栏）。 */
  newDoc: string;
  /** 代码语言，用于两栏语法高亮（复用 CodeEditor 的 language 映射）。 */
  language?: string;
}

/**
 * 基于 @codemirror/merge 的只读 Diff 视图（split 双栏，左旧右新）。
 * 只读对比，不涉及编辑/保存。通过 React.lazy 动态加载，与 CodeEditor 一样隔离 bundle。
 */
const DiffView: React.FC<DiffViewProps> = ({ oldDoc, newDoc, language }) => {
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<MergeView | null>(null);

  useEffect(() => {
    if (!hostRef.current) return;

    const langExt = languageExtension(language);
    // 两栏共用的只读基础扩展：行号 + 暗色主题 + 只读 + 语言高亮。
    const baseExtensions: Extension[] = [
      lineNumbers(),
      oneDark,
      EditorView.editable.of(false),
      EditorState.readOnly.of(true),
      EditorView.theme({
        '&': { height: '100%', fontSize: '14px' },
        '.cm-scroller': { fontFamily: "'SF Mono', Menlo, Monaco, monospace", lineHeight: '1.6' },
      }),
    ];
    if (langExt) baseExtensions.push(langExt);

    const view = new MergeView({
      a: { doc: oldDoc, extensions: baseExtensions },
      b: { doc: newDoc, extensions: baseExtensions },
      parent: hostRef.current,
      gutter: true,
    });
    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
  }, [oldDoc, newDoc, language]);

  return <div ref={hostRef} className={styles.diffHost} />;
};

export default DiffView;
