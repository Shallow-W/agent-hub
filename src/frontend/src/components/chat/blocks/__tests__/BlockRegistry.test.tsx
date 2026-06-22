// @vitest-environment jsdom
import { describe, it, expect } from 'vitest';
import type { ReactNode } from 'react';
import type { MessageBlock, BlockKind } from '@/types/message';

/**
 * BlockRegistry 测试。
 *
 * 测试策略：
 *   1. 自注册副作用：import './index' 触发 5 个内置 block 自注册（每个测试都 re-import）
 *   2. 单独测试 BlockRegistry 纯 API（register / get / has / render）
 *   3. 测试 aliases 历史兼容路径
 *
 * 注意：registry 是模块级单例（Map），测试间共享状态。为避免相互污染：
 *   - 内置注册（'text' / 'thinking' / 等）由 index.ts 副作用完成，所有测试都看到
 *   - 自定义 kind 用唯一字符串（如 'test-kind-1'）避免冲突
 *   - 未注册 kind 用 `as BlockKind` 绕过类型检查（运行时确实可能是任意字符串）
 */

// 集中触发内置 5 个 block 的 registerBlock 副作用
import '../index';

// 必须在 import '../index' 后导入，确保 registry 已填充
import {
  registerBlock,
  getBlockSpec,
  hasBlockRenderer,
  renderBlock,
  registeredBlockKinds,
} from '../BlockRegistry';

const UNKNOWN_KIND = 'nonexistent_kind_xyz' as BlockKind;

describe('BlockRegistry', () => {
  it('1. 内置 5 个 block 已自注册（import ./index 副作用）', () => {
    expect(hasBlockRenderer('text')).toBe(true);
    expect(hasBlockRenderer('thinking')).toBe(true);
    expect(hasBlockRenderer('tool_use')).toBe(true);
    expect(hasBlockRenderer('tool_result')).toBe(true);
    expect(hasBlockRenderer('error')).toBe(true);
  });

  it('2. getBlockSpec 已注册返回 spec，未注册返回 undefined', () => {
    expect(getBlockSpec('text')).toBeDefined();
    expect(getBlockSpec(UNKNOWN_KIND)).toBeUndefined();
  });

  it('3. hasBlockRenderer 与 getBlockSpec 一致', () => {
    expect(hasBlockRenderer('text')).toBe(getBlockSpec('text') !== undefined);
    expect(hasBlockRenderer(UNKNOWN_KIND)).toBe(false);
  });

  it('4. renderBlock 未注册返回 null', () => {
    const unknown = { kind: UNKNOWN_KIND, index: 0, text: '' } as unknown as MessageBlock;
    const result = renderBlock(unknown, false);
    expect(result).toBeNull();
  });

  it('5. renderBlock 已注册返回 React 元素（非 null）', () => {
    const textBlock: MessageBlock = { kind: 'text', index: 0, text: 'hello' };
    const result = renderBlock(textBlock, true);
    expect(result).not.toBeNull();
    // 返回的是 React 元素（JSX 转译后是对象）
    expect(typeof result).toBe('object');
  });

  it('6. registerBlock 自定义 kind 可被查询（手动注册）', () => {
    const customKind = ('test-custom-kind-' + Math.random().toString(36).slice(2, 8)) as BlockKind;
    const Custom = (): ReactNode => null;
    registerBlock(customKind, { component: Custom as never });
    expect(hasBlockRenderer(customKind)).toBe(true);
    expect(getBlockSpec(customKind)?.component).toBe(Custom);
  });

  it('7. registeredBlockKinds 至少包含 5 个内置 kind', () => {
    const kinds = registeredBlockKinds();
    expect(kinds).toContain('text');
    expect(kinds).toContain('thinking');
    expect(kinds).toContain('tool_use');
    expect(kinds).toContain('tool_result');
    expect(kinds).toContain('error');
  });
});
