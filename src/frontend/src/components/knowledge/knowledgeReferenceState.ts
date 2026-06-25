export interface KnowledgeReference {
  raw: string;
  username: string;
  kbName: string;
  key: string;
}

export type KnowledgeTextToken =
  | { type: 'text'; text: string }
  | { type: 'knowledge_ref'; ref: KnowledgeReference };

export interface KnowledgeContextSummary {
  knowledgeBaseCount: number;
  fileCount: number;
  collapsedChars: number;
}

export interface KnowledgeContextSimplification {
  text: string;
  summary: KnowledgeContextSummary | null;
}

const knowledgeRefRe = /\{\{([^/{}\s][^/{}]*?)\/([^/{}\s][^/{}]*?)\}\}/g;
const knowledgeContextMarkerRe = /^\[引用的知识库\]\s*$/gm;
const knowledgeBaseHeaderRe = /^\[知识库:\s*.+?\]\s*$/gm;
const knowledgeFileLineRe = /^- .+\(file_id=[^)]+\):\s*$/gm;
const knowledgeFileBodyRe = /(^- .+\(file_id=[^)]+\):\s*\n)```\n([\s\S]*?)\n```/gm;

export function extractKnowledgeRefs(text: string): KnowledgeReference[] {
  const refs: KnowledgeReference[] = [];
  const seen = new Set<string>();
  knowledgeRefRe.lastIndex = 0;

  let match: RegExpExecArray | null;
  while ((match = knowledgeRefRe.exec(text)) !== null) {
    const username = (match[1] ?? '').trim();
    const kbName = (match[2] ?? '').trim();
    if (!username || !kbName) continue;
    const key = `${username}/${kbName}`;
    if (seen.has(key)) continue;
    seen.add(key);
    refs.push({
      raw: match[0],
      username,
      kbName,
      key,
    });
  }

  return refs;
}

export function removeKnowledgeRef(text: string, key: string): string {
  return text
    .replace(knowledgeRefRe, (raw, username: string, kbName: string) => {
      const currentKey = `${username.trim()}/${kbName.trim()}`;
      return currentKey === key ? ' ' : raw;
    })
    .replace(/[ \t]{2,}/g, ' ')
    .replace(/\s+([，。,.!?；;：:])/g, '$1')
    .trim();
}

export function splitKnowledgeRefText(text: string): KnowledgeTextToken[] {
  const tokens: KnowledgeTextToken[] = [];
  knowledgeRefRe.lastIndex = 0;

  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = knowledgeRefRe.exec(text)) !== null) {
    if (match.index > lastIndex) {
      tokens.push({ type: 'text', text: text.slice(lastIndex, match.index) });
    }

    const username = (match[1] ?? '').trim();
    const kbName = (match[2] ?? '').trim();
    if (username && kbName) {
      tokens.push({
        type: 'knowledge_ref',
        ref: {
          raw: match[0],
          username,
          kbName,
          key: `${username}/${kbName}`,
        },
      });
    } else {
      tokens.push({ type: 'text', text: match[0] });
    }
    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < text.length) {
    tokens.push({ type: 'text', text: text.slice(lastIndex) });
  }

  return tokens;
}

export function simplifyKnowledgeContextText(text: string): KnowledgeContextSimplification {
  if (!knowledgeContextMarkerRe.test(text)) {
    knowledgeContextMarkerRe.lastIndex = 0;
    return { text, summary: null };
  }
  knowledgeContextMarkerRe.lastIndex = 0;

  const knowledgeBaseCount = countMatches(text, knowledgeBaseHeaderRe);
  const fileCount = countMatches(text, knowledgeFileLineRe);
  let collapsedChars = 0;

  const simplifiedText = text
    .replace(knowledgeContextMarkerRe, '')
    .replace(knowledgeFileBodyRe, (_raw, fileLine: string, body: string) => {
      collapsedChars += body.length;
      return `${fileLine}> 知识库文件内容已折叠（${body.length} 字），完整内容仅作为智能体上下文使用。`;
    })
    .replace(/\n{3,}/g, '\n\n')
    .trimStart();

  return {
    text: simplifiedText,
    summary: {
      knowledgeBaseCount,
      fileCount,
      collapsedChars,
    },
  };
}

function countMatches(text: string, re: RegExp): number {
  re.lastIndex = 0;
  let count = 0;
  while (re.exec(text) !== null) count += 1;
  re.lastIndex = 0;
  return count;
}
