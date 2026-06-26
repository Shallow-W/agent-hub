import { useEffect, useCallback } from 'react';
import { onWsEvent } from '@/store/wsStore';

/**
 * React hook：订阅一个 WS 事件类型，组件卸载时自动取消订阅。
 *
 * 用法：
 *   useWsEventHandler('agent.status', (data) => { ... });
 *   useWsEventHandler('my.custom.event', (data) => { ... });
 *
 * 新增 WS 事件类型只需在任意组件调用此 hook，无需修改 useWebSocket 的 switch。
 */
export function useWsEventHandler(
  eventType: string,
  handler: (data: unknown) => void,
): void {
  // useCallback 确保 handler 引用稳定，避免每次渲染都重新订阅
  const stableHandler = useCallback(
    (data: unknown) => handler(data),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [eventType],
  );

  useEffect(() => {
    return onWsEvent(eventType, stableHandler);
  }, [eventType, stableHandler]);
}
