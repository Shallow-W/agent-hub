import type { StreamMessage } from '@/types/message';

export type WsStatus = 'connecting' | 'connected' | 'disconnected';

type MessageHandler = (msg: StreamMessage) => void;

export class WebSocketClient {
  private ws: WebSocket | null = null;
  private url = '';
  private retryDelay = 1000;
  private maxRetryDelay = 30000;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private queue: string[] = [];
  private onMessageCallback: MessageHandler | null = null;
  private statusValue: WsStatus = 'disconnected';
  private statusListeners: Set<(status: WsStatus) => void> = new Set();

  get status(): WsStatus {
    return this.statusValue;
  }

  onStatusChange(listener: (status: WsStatus) => void): () => void {
    this.statusListeners.add(listener);
    return () => {
      this.statusListeners.delete(listener);
    };
  }

  onMessage(handler: MessageHandler): void {
    this.onMessageCallback = handler;
  }

  connect(connectToken: string): void {
    // 开发环境通过 Vite 代理，生产环境直接连
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    this.url = `${proto}//${window.location.host}/ws?token=${encodeURIComponent(connectToken)}`;
    this.doConnect();
  }

  send(message: string): void {
    if (this.ws && this.statusValue === 'connected') {
      this.ws.send(message);
    } else {
      // 断线时缓存消息，重连后发送
      this.queue.push(message);
    }
  }

  disconnect(): void {
    this.clearRetryTimer();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.setStatus('disconnected');
  }

  private doConnect(): void {
    this.clearRetryTimer();
    this.setStatus('connecting');

    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      this.retryDelay = 1000;
      this.setStatus('connected');
      // 重连后发送缓存的消息
      this.flushQueue();
    };

    this.ws.onmessage = (event: MessageEvent) => {
      try {
        const msg: StreamMessage = JSON.parse(event.data as string);
        this.onMessageCallback?.(msg);
      } catch {
        // 忽略无法解析的消息
      }
    };

    this.ws.onclose = () => {
      this.setStatus('disconnected');
      this.scheduleReconnect();
    };

    this.ws.onerror = () => {
      // onclose 会紧随其后触发，重连逻辑在那里处理
    };
  }

  private scheduleReconnect(): void {
    this.retryTimer = setTimeout(() => {
      this.doConnect();
    }, this.retryDelay);
    // 指数退避
    this.retryDelay = Math.min(this.retryDelay * 2, this.maxRetryDelay);
  }

  private clearRetryTimer(): void {
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
  }

  private flushQueue(): void {
    while (this.queue.length > 0) {
      const msg = this.queue.shift();
      if (msg) this.ws?.send(msg);
    }
  }

  private setStatus(status: WsStatus): void {
    this.statusValue = status;
    this.statusListeners.forEach((fn) => fn(status));
  }
}
