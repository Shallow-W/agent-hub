import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { EditorState, EditorSelection, type Extension } from '@codemirror/state';
import { EditorView, lineNumbers, drawSelection } from '@codemirror/view';
import { oneDark } from '@codemirror/theme-one-dark';
import { languageExtension } from './codemirrorLang';
import styles from './CodeEditor.module.css';

interface CodeSelectViewProps {
  value: string;
  language?: string;
  onSelectionChange?: (selection: string) => void;
}

export interface CodeSelectViewHandle {
  /** 选中全部代码：视图视觉全选高亮，并立即回写完整文本。 */
  selectAll: () => void;
}

/** 选区变化后停顿多久才提交（拖动过程中不频繁回写）。 */
const SELECTION_COMMIT_DEBOUNCE = 180;

/**
 * 只读、可选中的 CodeMirror 6 视图。专用于「AI 局部修改」的查看模式：
 * 用户在这里拖选一段代码，选区会持久、明显地高亮（失焦后依然可见）。
 * 拖动过程中视觉高亮实时显示，但「捕获到 selectedCode」这一步采用
 * 选定后捕获策略：鼠标松开（pointerup/mouseup）、键盘抬起（keyup）时读取，
 * 并对其它选区变化做短 debounce，避免拖动中频繁回写。
 * 通过 React.lazy 动态加载，与 CodeEditor 共用语言映射，不进首屏 bundle。
 */
const CodeSelectView = forwardRef<CodeSelectViewHandle, CodeSelectViewProps>(
  ({ value, language, onSelectionChange }, ref) => {
    const hostRef = useRef<HTMLDivElement>(null);
    const viewRef = useRef<EditorView | null>(null);
    const onSelectionChangeRef = useRef(onSelectionChange);
    onSelectionChangeRef.current = onSelectionChange;

    // 读取当前选区文本并回写（选区为空回写 ''）。
    const commitSelection = (view: EditorView) => {
      const selection = view.state.selection.ranges
        .map((range) => view.state.doc.sliceString(range.from, range.to))
        .filter(Boolean)
        .join('\n');
      onSelectionChangeRef.current?.(selection);
    };

    useImperativeHandle(ref, () => ({
      selectAll: () => {
        const view = viewRef.current;
        if (!view) return;
        const docLength = view.state.doc.length;
        view.dispatch({ selection: EditorSelection.single(0, docLength) });
        commitSelection(view);
      },
    }));

    // language 变化时重建编辑器（语言 extension 无法热切换）。
    useEffect(() => {
      if (!hostRef.current) return;

      let debounceTimer: ReturnType<typeof setTimeout> | null = null;

      const langExt = languageExtension(language);
      const extensions: Extension[] = [
        lineNumbers(),
        // drawSelection 让选区由 CM 自绘，失焦后仍持久显示（不依赖浏览器原生 ::selection）。
        drawSelection(),
        EditorView.editable.of(false),
        EditorState.readOnly.of(true),
        oneDark,
        EditorView.theme({
          '&': { height: '100%', fontSize: '15px' },
          '.cm-scroller': {
            fontFamily: "'SF Mono', Menlo, Monaco, monospace",
            lineHeight: '1.7',
            userSelect: 'text',
          },
          '.cm-content': {
            cursor: 'text',
            userSelect: 'text',
            caretColor: 'transparent',
          },
          // 持久、明显的选区配色：聚焦/失焦统一用高亮蓝，不淡化。
          '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
            backgroundColor: 'rgba(88, 166, 255, 0.5) !important',
          },
          '.cm-cursor': { display: 'none' },
        }),
        // 选区变化做短 debounce：拖动/连续移动过程中不提交，停下后再捕获一次。
        EditorView.updateListener.of((update) => {
          if (!update.selectionSet) return;
          if (debounceTimer) clearTimeout(debounceTimer);
          debounceTimer = setTimeout(() => {
            debounceTimer = null;
            commitSelection(update.view);
          }, SELECTION_COMMIT_DEBOUNCE);
        }),
      ];
      if (langExt) extensions.push(langExt);

      const view = new EditorView({
        state: EditorState.create({ doc: value, extensions }),
        parent: hostRef.current,
      });
      viewRef.current = view;

      // 鼠标松开 / 键盘抬起：立刻捕获一次（取消 pending debounce）。
      const captureNow = () => {
        if (debounceTimer) {
          clearTimeout(debounceTimer);
          debounceTimer = null;
        }
        commitSelection(view);
      };
      const dom = view.dom;
      dom.addEventListener('mouseup', captureNow);
      dom.addEventListener('keyup', captureNow);

      return () => {
        if (debounceTimer) clearTimeout(debounceTimer);
        dom.removeEventListener('mouseup', captureNow);
        dom.removeEventListener('keyup', captureNow);
        view.destroy();
        viewRef.current = null;
      };
      // value 仅作为初始文档，后续外部变更通过下方 effect 同步，避免重建。
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [language]);

    // 外部 value 变化（如切换版本）时同步文档，并清空选区。
    useEffect(() => {
      const view = viewRef.current;
      if (!view) return;
      const current = view.state.doc.toString();
      if (current !== value) {
        view.dispatch({
          changes: { from: 0, to: current.length, insert: value },
          selection: EditorSelection.single(0),
        });
        onSelectionChangeRef.current?.('');
      }
    }, [value]);

    return <div ref={hostRef} className={styles.editorHost} />;
  },
);

CodeSelectView.displayName = 'CodeSelectView';

export default CodeSelectView;
