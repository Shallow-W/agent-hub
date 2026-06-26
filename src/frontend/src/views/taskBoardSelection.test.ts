import { describe, expect, it } from 'vitest';
import { resolveActiveGroupConversationId } from './taskBoardSelection';

describe('resolveActiveGroupConversationId', () => {
  const groups = [{ id: 'group-1' }, { id: 'group-2' }];

  it('keeps the active conversation when it is a group', () => {
    expect(resolveActiveGroupConversationId(groups, 'group-2')).toBe('group-2');
  });

  it('falls back to the first group when active conversation is not a group', () => {
    expect(resolveActiveGroupConversationId(groups, 'single-1')).toBe('group-1');
  });

  it('returns null when there are no group conversations', () => {
    expect(resolveActiveGroupConversationId([], 'single-1')).toBeNull();
  });
});
