import type { Extension } from '@codemirror/state';
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

/**
 * 把项目里出现的 language 标识映射到对应的 CodeMirror language extension。
 * 未知语言降级为纯文本（返回 null，仍保留行号/暗色主题等基础能力）。
 * CodeEditor 与 DiffView 共用，避免语言映射重复。
 */
export function languageExtension(language?: string): Extension | null {
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
