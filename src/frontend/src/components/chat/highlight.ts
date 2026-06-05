// 集中注册 highlight.js 语言并提供高亮工具，供 CodeBlock / Artifact 复用。
// 仅注册项目需要的语言，保持打包体积小。
import hljs from 'highlight.js/lib/core';
import javascript from 'highlight.js/lib/languages/javascript';
import typescript from 'highlight.js/lib/languages/typescript';
import go from 'highlight.js/lib/languages/go';
import python from 'highlight.js/lib/languages/python';
import rust from 'highlight.js/lib/languages/rust';
import bash from 'highlight.js/lib/languages/bash';
import json from 'highlight.js/lib/languages/json';
import yaml from 'highlight.js/lib/languages/yaml';
import markdown from 'highlight.js/lib/languages/markdown';
import xml from 'highlight.js/lib/languages/xml';
import cssLang from 'highlight.js/lib/languages/css';
import sql from 'highlight.js/lib/languages/sql';

hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('js', javascript);
hljs.registerLanguage('typescript', typescript);
hljs.registerLanguage('ts', typescript);
hljs.registerLanguage('go', go);
hljs.registerLanguage('golang', go);
hljs.registerLanguage('python', python);
hljs.registerLanguage('py', python);
hljs.registerLanguage('rust', rust);
hljs.registerLanguage('bash', bash);
hljs.registerLanguage('sh', bash);
hljs.registerLanguage('shell', bash);
hljs.registerLanguage('zsh', bash);
hljs.registerLanguage('json', json);
hljs.registerLanguage('yaml', yaml);
hljs.registerLanguage('yml', yaml);
hljs.registerLanguage('markdown', markdown);
hljs.registerLanguage('md', markdown);
hljs.registerLanguage('html', xml);
hljs.registerLanguage('xml', xml);
hljs.registerLanguage('css', cssLang);
hljs.registerLanguage('sql', sql);

// 语言标签的友好显示名
export const LANG_DISPLAY: Record<string, string> = {
  js: 'JavaScript', ts: 'TypeScript', golang: 'Go', py: 'Python',
  sh: 'Shell', shell: 'Shell', zsh: 'Shell', yml: 'YAML', md: 'Markdown',
};

// 常见 language → 文件扩展名，用于“下载”时推断文件名。
const LANG_EXTENSION: Record<string, string> = {
  javascript: 'js', js: 'js', jsx: 'jsx',
  typescript: 'ts', ts: 'ts', tsx: 'tsx',
  go: 'go', golang: 'go',
  python: 'py', py: 'py',
  rust: 'rs', rs: 'rs',
  bash: 'sh', sh: 'sh', shell: 'sh', zsh: 'sh',
  json: 'json', yaml: 'yaml', yml: 'yaml',
  markdown: 'md', md: 'md',
  html: 'html', xml: 'xml', css: 'css', sql: 'sql',
};

/** 按 filename 优先、否则按 language 推断下载文件名；都缺省时退回 .txt。 */
export function inferDownloadName(filename?: string, language?: string): string {
  if (filename && filename.trim()) return filename.trim();
  const ext = language ? LANG_EXTENSION[language.toLowerCase()] : undefined;
  return `code.${ext || 'txt'}`;
}

export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

/** 高亮代码字符串，返回高亮后的 HTML；失败时回退为转义纯文本。 */
export function highlightCode(code: string, lang?: string): string {
  const trimmed = code.replace(/\n$/, '');
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(trimmed, { language: lang }).value;
    }
    return hljs.highlightAuto(trimmed).value;
  } catch {
    return escapeHtml(trimmed);
  }
}
