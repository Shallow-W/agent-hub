/**
 * 并发受限地运行 async mapper，返回每个 item 的 settled 结果（不 fail-fast）。
 *
 * 设计要点：
 * - 不 fail-fast：一个失败不影响其他。调用方按需用 Promise.allSettled 语义处理
 * - 保持输入顺序：results[i] 对应 items[i]
 * - 并发度可配：concurrency = 1 退化串行；≥ items.length 退化全并行
 * - 返回 PromiseSettledResult<R>[]：调用方按需 .status === 'fulfilled' 判断
 *
 * 应用场景：
 * - DiffCard 预取 N 个文件的 diff（限制 3 并发）
 * - 批量请求 API（防瞬间打爆后端）
 * - 批量加载图片/资源
 *
 * 示例：
 * ```ts
 * const results = await asyncPool(
 *   files,
 *   (f) => fileDiff(agentId, workDir, f),
 *   3,
 * );
 * results.forEach((r, i) => {
 *   if (r.status === 'fulfilled') cache.set(files[i], r.value);
 * });
 * ```
 */
export async function asyncPool<T, R>(
  items: readonly T[],
  mapper: (item: T, index: number) => Promise<R>,
  concurrency: number,
): Promise<PromiseSettledResult<R>[]> {
  if (items.length === 0) return [];
  const limit = Math.max(1, Math.min(concurrency, items.length));
  const results: PromiseSettledResult<R>[] = new Array(items.length);
  let cursor = 0;

  async function worker() {
    while (true) {
      const idx = cursor++;
      if (idx >= items.length) return;
      try {
        results[idx] = { status: 'fulfilled', value: await mapper(items[idx]!, idx) };
      } catch (err) {
        results[idx] = { status: 'rejected', reason: err };
      }
    }
  }

  const workers = Array.from({ length: limit }, () => worker());
  await Promise.all(workers);
  return results;
}
