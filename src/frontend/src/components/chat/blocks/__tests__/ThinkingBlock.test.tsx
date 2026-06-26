// @vitest-environment jsdom
import { describe, it, expect } from 'vitest';
import React from 'react';
import { createRoot } from 'react-dom/client';
// eslint-disable-next-line @typescript-eslint/no-deprecated
import { act } from 'react-dom/test-utils';
import { ThinkingBlock } from '../ThinkingBlock';
import type { MessageBlock } from '@/types/message';

/**
 * ThinkingBlock 测试。
 *
 * 测试策略（不依赖 @testing-library/react，只用 react-dom/client 原生 render）：
 *   1. 折叠态：渲染 label "思考过程" + body wrap 不带 expanded class
 *   2. streaming=true：自动展开（useEffect 触发），header label 切到"思考中"
 *   3. aria-label 可访问性
 *   4. streaming 时光标元素存在
 *
 * flushSync + act 配合：
 *   - flushSync：强制同步 commit（render 内部使用）
 *   - act：包裹 setState 触发器，确保 useEffect 在 act 退出前 flush
 */

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const actAny = act as any;

function renderToDOM(ui: React.ReactElement): HTMLDivElement {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  actAny(() => {
    root.render(ui);
  });
  return container;
}

const baseBlock = (overrides: Partial<MessageBlock> = {}): MessageBlock => ({
  kind: 'thinking',
  index: 0,
  text: '我在思考',
  ...overrides,
});

describe('ThinkingBlock', () => {
  it('1. 折叠态渲染 aria-label + body wrap 折叠', () => {
    const block = baseBlock({ text: '分析问题' });
    const container = renderToDOM(<ThinkingBlock block={block} />);

    // outer 容器带 aria-label
    const outer = container.querySelector('[aria-label="AI 思考过程"]');
    expect(outer).not.toBeNull();

    // 折叠态 bodyWrap 没有 thinkingBodyExpanded class
    const bodyWrap = container.querySelector('[class*="thinkingBodyWrap"]') as HTMLElement;
    expect(bodyWrap).not.toBeNull();
    expect(bodyWrap.className).not.toContain('thinkingBodyExpanded');
  });

  it('2. streaming=true → outer 带 thinkingStreaming + body 自动展开 + label 为"思考中"', () => {
    const block = baseBlock({ text: '流式思考…' });
    const container = renderToDOM(<ThinkingBlock block={block} streaming={true} />);

    const outer = container.querySelector('[class*="thinkingStreaming"]');
    expect(outer).not.toBeNull();

    // streaming 自动展开在 useEffect 中触发，act 已 flush
    const bodyWrap = container.querySelector('[class*="thinkingBodyWrap"]') as HTMLElement;
    expect(bodyWrap.className).toContain('thinkingBodyExpanded');

    const label = container.querySelector('[class*="thinkingLabel"]');
    expect(label?.textContent).toBe('思考中');
  });

  it('3. 折叠态 + 无 streaming → header label 为"思考过程"', () => {
    const block = baseBlock({ text: '某个想法' });
    const container = renderToDOM(<ThinkingBlock block={block} />);

    const label = container.querySelector('[class*="thinkingLabel"]');
    expect(label?.textContent).toBe('思考过程');
  });

  it('4. 空 text → 不渲染 preview', () => {
    const block = baseBlock({ text: '' });
    const container = renderToDOM(<ThinkingBlock block={block} />);

    const preview = container.querySelector('[class*="thinkingPreview"]');
    expect(preview).toBeNull();
  });

  it('5. streaming 时光标元素存在', () => {
    const block = baseBlock({ text: '思考中…' });
    const container = renderToDOM(<ThinkingBlock block={block} streaming={true} />);

    const cursor = container.querySelector('[class*="streamingCursor"]');
    expect(cursor).not.toBeNull();
  });

  it('6. 点击 header 切换展开状态', () => {
    const block = baseBlock({ text: '短文本' });
    const container = renderToDOM(<ThinkingBlock block={block} />);

    const header = container.querySelector('[class*="thinkingHeader"]') as HTMLElement;
    expect(header).not.toBeNull();

    const bodyWrap = () => container.querySelector('[class*="thinkingBodyWrap"]') as HTMLElement;

    // 初始折叠
    expect(bodyWrap().className).not.toContain('thinkingBodyExpanded');

    // 模拟 click，act 让 setState + useEffect 同步生效
    actAny(() => {
      header.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    // 展开后应有 expanded class
    expect(bodyWrap().className).toContain('thinkingBodyExpanded');
  });

  it('7. 长 text → preview span 存在且带省略号', () => {
    const longText = 'a'.repeat(100);
    const block = baseBlock({ text: longText });
    const container = renderToDOM(<ThinkingBlock block={block} />);

    const preview = container.querySelector('[class*="thinkingPreview"]');
    expect(preview).not.toBeNull();
    expect(preview?.textContent).toContain('…');
  });

  it('8. keyboard 支持：Enter / Space 触发展开', () => {
    const block = baseBlock({ text: '键盘可访问' });
    const container = renderToDOM(<ThinkingBlock block={block} />);
    const header = container.querySelector('[class*="thinkingHeader"]') as HTMLElement;

    const bodyWrap = () => container.querySelector('[class*="thinkingBodyWrap"]') as HTMLElement;

    // Enter 展开
    actAny(() => {
      header.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    });
    expect(bodyWrap().className).toContain('thinkingBodyExpanded');

    // Space 收起
    actAny(() => {
      header.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true }));
    });
    expect(bodyWrap().className).not.toContain('thinkingBodyExpanded');
  });
});

