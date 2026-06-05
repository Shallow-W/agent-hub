import React, { useEffect, useRef } from 'react';
import { EditorState, type Extension } from '@codemirror/state';
import { EditorView, keymap, lineNumbers, highlightActiveLine } from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { indentUnit } from '@codemirror/language';
import { oneDark } from '@codemirror/theme-one-dark';
import { javascript } from '@codemirror/lang-javascript';
import { python } from '@codemirror/lang-python';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { json } from '@codemirror/lang-json';
import { rust } from '@codemirror/lang-rust';
import { go } from '@codemirror/lang-go';
import { sql } from '@codemirror/lang-sql';
import { markdown } from '@codemirror/lang-markdown';
import { yaml } from '@codemirror/lang-yaml';
import styles from './CodeEditor.module.css';

interface CodeEditorProps {
  value: string;
  language?: string;
  onChange: (value: string) => void;
}

/**
 * 把项目里出现的 language 标识映射到对应的 CodeMirror language extension。
 * 未知语言降级为纯文本（无 language extension，仍保留行号/暗色主题/编辑能力）。
 */
function languageExtension(language?: string): Extension | null {
  switch ((language || '').toLowerCase()) {
    case 'js':
    case 'javascript':
    case 'jsx':
      return javascript({ jsx: true });
    case 'ts':
    case 'typescript':
      return javascript({ jsx: true, typescript: true });
    case 'tsx':
      return javascript({ jsx: true, typescript: true });
    case 'py':
    case 'python':
      return python();
    case 'html':
    case 'xml':
      return html();
    case 'css':
      return css();
    case 'json':
      return json();
    case 'rust':
    case 'rs':
      return rust();
    case 'go':
    case 'golang':
      return go();
    case 'sql':
      return sql();
    case 'md':
    case 'markdown':
      return markdown();
    case 'yaml':
    case 'yml':
      return yaml();
    default:
      return null;
  }
}

/**
 * 基于 CodeMirror 6 的可编辑代码视图。仅在全屏 Modal 的“编辑”模式下使用，
 * 通过 React.lazy 动态加载，避免把 CodeMirror 打进首屏 bundle。
 */
const CodeEditor: React.FC<CodeEditorProps> = ({ value, language, onChange }) => {
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  // 用 ref 持有最新 onChange，避免它进入重建依赖导致编辑器频繁销毁重建。
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

  // language 变化时重建编辑器（语言 extension 无法热切换）。
  useEffect(() => {
    if (!hostRef.current) return;

    const langExt = languageExtension(language);
    const extensions: Extension[] = [
      lineNumbers(),
      highlightActiveLine(),
      history(),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      indentUnit.of('  '),
      oneDark,
      EditorView.theme({
        '&': { height: '100%', fontSize: '15px' },
        '.cm-scroller': { fontFamily: "'SF Mono', Menlo, Monaco, monospace", lineHeight: '1.7' },
      }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          onChangeRef.current(update.state.doc.toString());
        }
      }),
    ];
    if (langExt) extensions.push(langExt);

    const view = new EditorView({
      state: EditorState.create({ doc: value, extensions }),
      parent: hostRef.current,
    });
    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // value 仅作为初始文档，后续外部变更通过下方 effect 同步，避免重建。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language]);

  // 外部 value 变化（如“重置”）时同步到编辑器，但避免覆盖用户正在输入的内容。
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (current !== value) {
      view.dispatch({ changes: { from: 0, to: current.length, insert: value } });
    }
  }, [value]);

  return <div ref={hostRef} className={styles.editorHost} />;
};

export default CodeEditor;
