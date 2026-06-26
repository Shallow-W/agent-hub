// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import { createRoot } from 'react-dom/client';
// eslint-disable-next-line @typescript-eslint/no-deprecated
import { act } from 'react-dom/test-utils';
import type { MessageBlock } from '@/types/message';
import type { BlockRenderContext } from '../BlockRegistry';

/**
 * CardBlock 单元测试。
 *
 * 验证：
 *   1. CardBlock 已通过 './index' 自注册到 BlockRegistry
 *   2. block.kind='card' 且 block.card 存在 + ctx 完备时 → renderCards 被调用并渲染
 *   3. block.kind='card' 但 block.card 缺失 → 不调 renderCards，DOM 为空
 *   4. block.kind='card' 但 ctx 缺失 → 不调 renderCards，DOM 为空（防御性）
 *
 * 测试策略：直接渲染 CardBlock 组件（绕过 registry 的 renderBlock，
 * 后者只是薄壳），用 DOM 断言 + renderCards mock 验证调用契约。
 */

// 集中触发内置 block 自注册副作用（含 CardBlock）
import '../index';

import { hasBlockRenderer, getBlockSpec, renderBlock } from '../BlockRegistry';
import { CardBlock } from '../CardBlock';
import { renderCards } from '../../cards/CardRegistry';

vi.mock('../../cards/CardRegistry', () => ({
  renderCards: vi.fn(() => [<div key="mock" data-testid="mocked-card">MOCKED_CARD</div>]),
}));

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

const baseCtx: BlockRenderContext = {
  conversationId: 'conv-1',
  messageId: 'msg-1',
  agentId: 'agent-1',
  artifacts: [],
  onAction: () => {},
};

describe('CardBlock', () => {
  beforeEach(() => {
    vi.mocked(renderCards).mockClear();
  });

  it('1. 已自注册到 BlockRegistry', () => {
    expect(hasBlockRenderer('card')).toBe(true);
    expect(getBlockSpec('card')).toBeDefined();
  });

  it('2. kind=card 且 card 存在 + ctx 完备 → 调用 renderCards 并渲染', () => {
    const block: MessageBlock = {
      kind: 'card',
      index: 0,
      text: '',
      card: { type: 'info', id: 'c1', fields: { k: 'v' } } as never,
    };
    const container = renderToDOM(<CardBlock block={block} ctx={baseCtx} />);

    expect(renderCards).toHaveBeenCalledOnce();
    const callArgs = vi.mocked(renderCards).mock.calls[0];
    if (!callArgs) {
      throw new Error('renderCards was not called with arguments');
    }
    expect(callArgs[0]).toEqual([block.card]);
    expect(callArgs[1]).toBe(baseCtx.conversationId);
    expect(callArgs[2]).toBe(baseCtx.messageId);
    expect(callArgs[3]).toBe(baseCtx.onAction);
    expect(callArgs[4]).toBe(baseCtx.artifacts);
    expect(callArgs[5]).toBe(baseCtx.agentId);

    // mocked renderCards 返回的 div 出现在 DOM 中
    const mocked = container.querySelector('[data-testid="mocked-card"]');
    expect(mocked).not.toBeNull();
  });

  it('3. kind=card 但 block.card 缺失 → 不调 renderCards，DOM 空', () => {
    const block = { kind: 'card', index: 0, text: '' } as unknown as MessageBlock;
    const container = renderToDOM(<CardBlock block={block} ctx={baseCtx} />);
    expect(renderCards).not.toHaveBeenCalled();
    expect(container.children.length).toBe(0);
  });

  it('4. kind=card 但 ctx 缺失 → 不调 renderCards，DOM 空', () => {
    const block: MessageBlock = {
      kind: 'card',
      index: 0,
      text: '',
      card: { type: 'info', id: 'c1', fields: {} } as never,
    };
    // ctx 不传 → undefined
    const container = renderToDOM(<CardBlock block={block} />);
    expect(renderCards).not.toHaveBeenCalled();
    expect(container.children.length).toBe(0);
  });

  it('5. 经 renderBlock（registry 顶层 API）渲染 card block 也能正常工作', () => {
    // renderBlock 是 MessageBubble 实际使用的入口，本测试验证 registry 路径与
    // 直接渲染 CardBlock 一致。
    const block: MessageBlock = {
      kind: 'card',
      index: 0,
      text: '',
      card: { type: 'info', id: 'c1', fields: {} } as never,
    };
    const container = renderToDOM(
      <>{renderBlock(block, false, baseCtx)}</>,
    );
    expect(renderCards).toHaveBeenCalledOnce();
    const mocked = container.querySelector('[data-testid="mocked-card"]');
    expect(mocked).not.toBeNull();
  });
});
