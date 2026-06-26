import { describe, it, expect } from 'vitest';
import {
  streamingReducer,
  reduceEvents,
  initialStreamingState,
  type StreamingState,
} from '../streamingReducer';
import type { AgentEvent } from '@/types/agentEvent';

describe('streamingReducer', () => {
  it('1. 空事件 reduceEvents 返回 initial state（引用相等）', () => {
    const result = reduceEvents([]);
    // 空数组返回原引用，语义不变
    expect(result).toBe(initialStreamingState);
    expect(result.blocks).toEqual([]);
    expect(result.status).toBe('streaming');
  });

  it('2. 单 text 事件创建新 text block', () => {
    const result = reduceEvents([{ type: 'text', content: 'hello' }]);
    expect(result.blocks).toHaveLength(1);
    expect(result.blocks[0]).toMatchObject({
      kind: 'text',
      text: 'hello',
      index: 0,
    });
    expect(result.status).toBe('streaming');
  });

  it('3. 连续 text 事件累积到同一 block', () => {
    const events: AgentEvent[] = [
      { type: 'text', content: 'hello' },
      { type: 'text', content: ' world' },
      { type: 'text', content: '!' },
    ];
    const result = reduceEvents(events);
    expect(result.blocks).toHaveLength(1);
    expect(result.blocks[0]!.text).toBe('hello world!');
    expect(result.blocks[0]!.index).toBe(0);
  });

  it('4. text→thinking→text 产生 3 个独立 block（单调递增 index）', () => {
    const events: AgentEvent[] = [
      { type: 'text', content: 'a' },
      { type: 'thinking', content: 'b' },
      { type: 'text', content: 'c' },
    ];
    const result = reduceEvents(events);
    expect(result.blocks).toHaveLength(3);
    expect(result.blocks.map((b) => b.kind)).toEqual([
      'text',
      'thinking',
      'text',
    ]);
    expect(result.blocks.map((b) => b.index)).toEqual([0, 1, 2]);
  });

  it('5. tool_use(name) + tool_use(partial content) + tool_result 累积成 2 个 block', () => {
    // 当前线上 daemon 把 input_json_delta 的 partial_json 作为 toolUseEvent('', partial_json)
    // 发出。appendDeltas 把这部分追加到最近一个 tool_use block 的 text（同一 block），
    // 而不是产生新 block。所以最终 block 数：1 个 tool_use + 1 个 tool_result = 2。
    // reducer 保留这一行为（PR1 不改 runtime 语义）。
    const events: AgentEvent[] = [
      { type: 'tool_use', tool: 'Bash', tool_use_id: 'tu_1' },
      // 模拟线上字段（input）——reducer 兼容路径
      { type: 'tool_use', tool: '', input: '{"cmd":"ls"}' },
      { type: 'tool_result', output: 'done', tool_use_id: 'tu_1' },
    ];
    const result = reduceEvents(events);
    expect(result.blocks).toHaveLength(2);
    expect(result.blocks[0]).toMatchObject({
      kind: 'tool_use',
      tool_name: 'Bash',
      tool_use_id: 'tu_1',
    });
    // 第二个事件 partial 追加到第一个 tool_use block 的 text
    expect(result.blocks[0]!.text).toBe('{"cmd":"ls"}');
    expect(result.blocks[1]).toMatchObject({
      kind: 'tool_result',
      text: 'done',
      is_error: false,
    });
    // 验证 index 单调
    expect(result.blocks.map((b) => b.index)).toEqual([0, 1]);
  });

  it('5b. 新协议 tool.call.start + tool.call.input + tool.call.end 累积', () => {
    const events: AgentEvent[] = [
      { type: 'tool.call.start', tool: 'Read', tool_use_id: 'tu_2' },
      { type: 'tool.call.input', tool_use_id: 'tu_2', delta: '{"file_path":"/a' },
      { type: 'tool.call.input', tool_use_id: 'tu_2', delta: '.txt"}' },
      { type: 'tool.call.end', tool_use_id: 'tu_2' },
      { type: 'tool.result', tool_use_id: 'tu_2', content: 'file body' },
    ];
    const result = reduceEvents(events);
    expect(result.blocks).toHaveLength(2);
    expect(result.blocks[0]).toMatchObject({
      kind: 'tool_use',
      tool_name: 'Read',
      tool_use_id: 'tu_2',
      text: '{"file_path":"/a.txt"}',
    });
    expect(result.blocks[1]).toMatchObject({
      kind: 'tool_result',
      text: 'file body',
    });
  });

  it('6. turn_end 切 status 到 complete（不产生 block）', () => {
    const result = reduceEvents([{ type: 'turn_end' }]);
    expect(result.status).toBe('complete');
    expect(result.blocks).toHaveLength(0);
  });

  it('6b. session.end / session_end 同样切 complete（双兼容）', () => {
    expect(reduceEvents([{ type: 'session.end' }]).status).toBe('complete');
    expect(reduceEvents([{ type: 'session_end' }]).status).toBe('complete');
  });

  it('7. error 事件追加 error block + status=error', () => {
    const result = reduceEvents([
      { type: 'text', content: 'partial' },
      { type: 'error', message: 'boom' },
    ]);
    expect(result.blocks).toHaveLength(2);
    expect(result.blocks[1]).toMatchObject({
      kind: 'error',
      text: 'boom',
      is_error: true,
    });
    expect(result.status).toBe('error');
  });

  it('8. cancel 事件切 status 到 canceled', () => {
    const result = reduceEvents([{ type: 'cancel', reason: '用户取消' }]);
    expect(result.status).toBe('canceled');
    expect(result.blocks).toHaveLength(0);
  });

  it('9. 纯函数性：reduce 不修改入参 state', () => {
    const initial: StreamingState = {
      blocks: [{ index: 0, kind: 'text', text: 'pre' }],
      status: 'streaming',
    };
    const snapshot: StreamingState = JSON.parse(JSON.stringify(initial));
    const result = streamingReducer(initial, { type: 'text', content: '-post' });
    // 入参对象 / 数组 / 内部对象均未被修改
    expect(initial).toEqual(snapshot);
    expect(initial.blocks).toHaveLength(1);
    expect(initial.blocks[0]!.text).toBe('pre');
    // 返回新对象
    expect(result).not.toBe(initial);
    expect(result.blocks).not.toBe(initial.blocks);
    expect(result.blocks[0]).not.toBe(initial.blocks[0]);
    expect(result.blocks[0]!.text).toBe('pre-post');
  });

  it('10. 终态保护：status 非 streaming 时后续事件忽略', () => {
    const ended: StreamingState = {
      blocks: [],
      status: 'complete',
    };
    const result = streamingReducer(ended, { type: 'text', content: 'x' });
    // 终态后 reduce 直接返回原引用（无变化）
    expect(result).toBe(ended);
    expect(result.blocks).toHaveLength(0);
  });

  it('11. tool_result 双字段兼容：output 或 content 都能读到', () => {
    const r1 = reduceEvents([{ type: 'tool_result', output: 'via-output' }]);
    const r2 = reduceEvents([{ type: 'tool.result', content: 'via-content' }]);
    expect(r1.blocks[0]!.text).toBe('via-output');
    expect(r2.blocks[0]!.text).toBe('via-content');
  });

  it('12. is_error / isError 双字段兼容', () => {
    const r1 = reduceEvents([{ type: 'tool_result', output: 'x', is_error: true }]);
    const r2 = reduceEvents([
      { type: 'tool_result', output: 'x', isError: true },
    ]);
    expect(r1.blocks[0]!.is_error).toBe(true);
    expect(r2.blocks[0]!.is_error).toBe(true);
  });
});
