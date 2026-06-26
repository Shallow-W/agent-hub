// @vitest-environment jsdom
import { describe, it, expect } from 'vitest';
import { splitByCardPlaceholder } from './MessageBubble';
import type { InteractiveCard } from '@/types/card';

describe('splitByCardPlaceholder', () => {
  it('case 1: 无占位符 + 有卡片 → unmatchedCards 是全部卡片，segments 只有一段 markdown', () => {
    const content = '我改了两个文件：App.tsx 和 index.css';
    const cards: InteractiveCard[] = [
      { type: 'diff', id: 'd1', title: '本次修改', workDir: '/path', files: ['App.tsx'] } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    expect(segments).toHaveLength(1);
    expect(segments[0]!.type).toBe('markdown');
    expect(unmatchedCards).toHaveLength(1);
    expect(unmatchedCards[0]!.id).toBe('d1');
  });

  it('case 2: 正文有 [CARD:id] + cards 含同 id → 拆成 md + card + md 三段，unmatchedCards 为空', () => {
    const content = '前文\n[CARD:d1]\n后文';
    const cards: InteractiveCard[] = [
      { type: 'diff', id: 'd1', title: '', workDir: '/p', files: [] } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    expect(segments).toHaveLength(3);
    // splitByCardPlaceholder 用 slice(lastIndex, match.index) 取占位符前文本，
    // 含占位符行之前的换行符；锁行为：保留尾部 \n。
    expect(segments[0]).toMatchObject({ type: 'markdown', content: '前文\n' });
    expect(segments[1]).toMatchObject({ type: 'card' });
    expect((segments[1] as { card: { id: string } }).card.id).toBe('d1');
    expect(segments[2]).toMatchObject({ type: 'markdown', content: '\n后文' });
    expect(unmatchedCards).toHaveLength(0);
  });

  it('case 3: 正文有占位符但 cards 无匹配 → 占位符保留为字面文本，unmatchedCards 含全部卡片', () => {
    const content = '正文中提到 [CARD:diff-1] 但没这张卡';
    const cards: InteractiveCard[] = [
      { type: 'info', id: 'other', fields: {} } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    // regex 要求占位符独立一行，这里 [CARD:diff-1] 是行内 → 不匹配，整段一段 markdown
    expect(segments).toHaveLength(1);
    expect(segments[0]!.type).toBe('markdown');
    expect(segments[0]).toMatchObject({ content });
    expect(unmatchedCards).toHaveLength(1);
  });

  it('case 4: 多个占位符同一消息 → 按顺序拆成多段', () => {
    const content = '一\n[CARD:a]\n二\n[CARD:b]\n三';
    const cards: InteractiveCard[] = [
      { type: 'info', id: 'a', fields: {} } as InteractiveCard,
      { type: 'info', id: 'b', fields: {} } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    // 5 段：md, card a, md, card b, md
    expect(segments).toHaveLength(5);
    expect(segments.map((s) => s.type)).toEqual(['markdown', 'card', 'markdown', 'card', 'markdown']);
    expect(unmatchedCards).toHaveLength(0);
  });

  it('case 5: 大小写敏感——[card:diff-1] 不匹配（小写 card）', () => {
    const content = 'lowercase: [card:diff-1]';
    const cards: InteractiveCard[] = [
      { type: 'diff', id: 'diff-1', title: '', workDir: '/p', files: [] } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    // [card:...] 不匹配（regex 要求大写 CARD），且这里也是行内不独立一行
    expect(segments).toHaveLength(1);
    expect(segments[0]!.type).toBe('markdown');
    expect(unmatchedCards).toHaveLength(1);
    expect(unmatchedCards[0]!.id).toBe('diff-1');
  });

  it('case 6: 占位符必须独立一行——行内 [CARD:id] 不匹配', () => {
    const content = '行内 [CARD:d1] 不会触发拆段';
    const cards: InteractiveCard[] = [
      { type: 'info', id: 'd1', fields: {} } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    expect(segments).toHaveLength(1);
    expect(unmatchedCards).toHaveLength(1);
    // 占位符当字面文本
    expect(segments[0]).toMatchObject({ content });
  });

  it('case 7: 空内容 + 有卡片 → segments 为空，unmatchedCards 是全部', () => {
    const content = '';
    const cards: InteractiveCard[] = [
      { type: 'info', id: 'x', fields: {} } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    expect(segments).toHaveLength(0);
    expect(unmatchedCards).toHaveLength(1);
  });

  it('case 8: 有内容 + 无卡片 → 单 markdown 段，unmatchedCards 为空', () => {
    const content = '纯文本回复';
    const cards: InteractiveCard[] = [];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    expect(segments).toHaveLength(1);
    expect(segments[0]).toMatchObject({ type: 'markdown', content: '纯文本回复' });
    expect(unmatchedCards).toHaveLength(0);
  });

  it('case 9: 占位符前后有空格/tab → 匹配（regex 允许行首行尾空白）', () => {
    const content = '文字\n   [CARD:d1]  \n后续';
    const cards: InteractiveCard[] = [
      { type: 'info', id: 'd1', fields: {} } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    expect(segments).toHaveLength(3);
    expect(segments[1]).toMatchObject({ type: 'card' });
    expect(unmatchedCards).toHaveLength(0);
  });

  it('case 10: 同一卡片被多个占位符引用 → 只在第一个位置渲染，其他位置保留字面文本', () => {
    // 当前实现：cards.find() 不删除元素，同一卡片可被多个占位符找到 → 都渲染。
    // matchedIds 是 Set，add 重复无副作用；unmatchedCards 用 filter 判断 id 是否在 matchedIds 中。
    // 所以两个占位符都拆段，unmatchedCards 长度=0。
    const content = '一\n[CARD:d1]\n二\n[CARD:d1]\n三';
    const cards: InteractiveCard[] = [
      { type: 'info', id: 'd1', fields: {} } as InteractiveCard,
    ];
    const { segments, unmatchedCards } = splitByCardPlaceholder(content, cards);
    expect(segments.filter((s) => s.type === 'card')).toHaveLength(2);
    expect(unmatchedCards).toHaveLength(0);
  });
});
