// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest';
import { asyncPool } from './asyncPool';

describe('asyncPool', () => {
  it('case 1: 空数组 → 立即返回空结果', async () => {
    const results = await asyncPool([], async () => 1, 3);
    expect(results).toEqual([]);
  });

  it('case 2: concurrency=1 → 顺序串行执行，全部 fulfilled', async () => {
    const items = [1, 2, 3, 4, 5];
    const startTimes: number[] = [];
    const results = await asyncPool(
      items,
      async (n) => {
        startTimes.push(Date.now());
        await new Promise((r) => setTimeout(r, 10));
        return n * 2;
      },
      1,
    );
    expect(results).toHaveLength(5);
    results.forEach((r, i) => {
      expect(r.status).toBe('fulfilled');
      if (r.status === 'fulfilled') expect(r.value).toBe(items[i]! * 2);
    });
    // 串行：每个任务的开始时间应递增（后一个 > 前一个）
    for (let i = 1; i < startTimes.length; i++) {
      expect(startTimes[i]).toBeGreaterThanOrEqual(startTimes[i - 1]!);
    }
  });

  it('case 3: concurrency ≥ items.length → 退化为全并行（实际并发度 = items.length）', async () => {
    const items = [1, 2, 3];
    const results = await asyncPool(
      items,
      async (n) => n + 10,
      10,
    );
    expect(results).toHaveLength(3);
    expect(results.map((r) => (r.status === 'fulfilled' ? r.value : null))).toEqual([11, 12, 13]);
  });

  it('case 4: 不 fail-fast —— 中间某项抛错，其他继续执行并 settled', async () => {
    const items = [1, 2, 3, 4];
    const results = await asyncPool(
      items,
      async (n) => {
        if (n === 2) throw new Error('boom');
        return n;
      },
      2,
    );
    expect(results).toHaveLength(4);
    expect(results[0]).toMatchObject({ status: 'fulfilled', value: 1 });
    expect(results[1]).toMatchObject({ status: 'rejected' });
    expect(results[2]).toMatchObject({ status: 'fulfilled', value: 3 });
    expect(results[3]).toMatchObject({ status: 'fulfilled', value: 4 });
  });

  it('case 5: 保持输入顺序 —— results[i] 对应 items[i]，不管完成顺序', async () => {
    // 让后面的 item 先完成（sleep 反序）
    const items = ['a', 'b', 'c', 'd'];
    const results = await asyncPool(
      items,
      async (s) => {
        // a 最慢，d 最快
        const delay = (items.length - items.indexOf(s)) * 20;
        await new Promise((r) => setTimeout(r, delay));
        return s.toUpperCase();
      },
      4,
    );
    expect(results.map((r) => (r.status === 'fulfilled' ? r.value : null))).toEqual([
      'A',
      'B',
      'C',
      'D',
    ]);
  });

  it('case 6: 并发度真的被限制 —— 用 spy 计数最大并发数 ≤ concurrency', async () => {
    const items = Array.from({ length: 10 }, (_, i) => i);
    let inflight = 0;
    let maxInflight = 0;
    const spy = vi.fn(async (n: number) => {
      inflight++;
      maxInflight = Math.max(maxInflight, inflight);
      await new Promise((r) => setTimeout(r, 15));
      inflight--;
      return n;
    });
    await asyncPool(items, spy, 3);
    expect(spy).toHaveBeenCalledTimes(10);
    expect(maxInflight).toBeLessThanOrEqual(3);
    // 实际应该恰好达到 3（items 足够多）
    expect(maxInflight).toBe(3);
  });

  it('case 7: mapper 收到正确的 index 参数', async () => {
    const items = ['x', 'y', 'z'];
    const indices: number[] = [];
    await asyncPool(
      items,
      async (_s, i) => {
        indices.push(i);
        return i;
      },
      3,
    );
    expect(indices.sort((a, b) => a - b)).toEqual([0, 1, 2]);
  });
});
