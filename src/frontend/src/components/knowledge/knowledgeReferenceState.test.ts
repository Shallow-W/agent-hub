import { describe, expect, it } from 'vitest';
import {
  extractKnowledgeRefs,
  removeKnowledgeRef,
  simplifyKnowledgeContextText,
  splitKnowledgeRefText,
} from './knowledgeReferenceState';

describe('knowledgeReferenceState', () => {
  it('deduplicates valid knowledge references in message text', () => {
    expect(extractKnowledgeRefs('请参考 {{alice/API文档}} 和 {{alice/API文档}}、{{bob/设计稿}}')).toEqual([
      { raw: '{{alice/API文档}}', username: 'alice', kbName: 'API文档', key: 'alice/API文档' },
      { raw: '{{bob/设计稿}}', username: 'bob', kbName: '设计稿', key: 'bob/设计稿' },
    ]);
  });

  it('ignores malformed and empty knowledge references', () => {
    expect(extractKnowledgeRefs('{{}} {{alice/}} {{/docs}} {{alice/docs}}')).toEqual([
      { raw: '{{alice/docs}}', username: 'alice', kbName: 'docs', key: 'alice/docs' },
    ]);
  });

  it('removes one knowledge reference and keeps surrounding text readable', () => {
    expect(removeKnowledgeRef('A {{alice/API文档}}  B', 'alice/API文档')).toBe('A B');
  });

  it('splits text into plain text and knowledge reference tokens', () => {
    expect(splitKnowledgeRefText('参考 {{alice/API文档}} 后回答')).toEqual([
      { type: 'text', text: '参考 ' },
      {
        type: 'knowledge_ref',
        ref: { raw: '{{alice/API文档}}', username: 'alice', kbName: 'API文档', key: 'alice/API文档' },
      },
      { type: 'text', text: ' 后回答' },
    ]);
  });

  it('collapses inline knowledge file bodies but keeps file metadata and answer text', () => {
    const result = simplifyKnowledgeContextText([
      '前文',
      '[引用的知识库]',
      '[知识库: alice/API文档 (public)]',
      '- guide.md (file_id=file-1, 12 KB):',
      '```',
      '第一行知识库正文',
      '第二行知识库正文',
      '```',
      '后续回答',
    ].join('\n'));

    expect(result.summary).toMatchObject({
      knowledgeBaseCount: 1,
      fileCount: 1,
    });
    expect(result.summary?.collapsedChars).toBeGreaterThan(0);
    expect(result.text).toContain('[知识库: alice/API文档 (public)]');
    expect(result.text).toContain('- guide.md (file_id=file-1, 12 KB):');
    expect(result.text).toContain('知识库文件内容已折叠');
    expect(result.text).toContain('后续回答');
    expect(result.text).not.toContain('第一行知识库正文');
  });
});
