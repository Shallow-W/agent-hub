import type { StreamMessage } from '@/types/message';
import { wsURL } from './runtime';

export type WsStatus = 'connecting' | 'connected' | 'disconnected';

type MessageHandler = (msg: StreamMessage) => void;

export class WebSocketClient {
  private ws: WebSocket | null = null;
  private url = '';
  private retryDelay = 1000;
  private maxRetryDelay = 30000;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private queue: string[] = [];
  private intentionalClose = false;
  private onMessageCallback: MessageHandler | null = null;
  private statusValue: WsStatus = 'disconnected';
  private statusListeners: Set<(status: WsStatus) => void> = new Set();
  private joinedRooms: Set<string> = new Set();

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

  offMessage(): void {
    this.onMessageCallback = null;
  }

  connect(connectToken: string): void {
    // 桌面端加载本地前端文件，但 WebSocket 必须连接远程后端。
    this.url = wsURL(`/ws?token=${encodeURIComponent(connectToken)}`);
    this.doConnect();
  }

  send(message: string): void {
    if (this.ws && this.statusValue === 'connected') {
      this.ws.send(message);
    } else {
      // 断线时缓存消息，重连后发送
      this.queue.push(message);
    }
    // 追踪 join_room 以便重连后恢复
    try {
      const parsed = JSON.parse(message);
      if (parsed.type === 'join_room' && parsed.data?.conversation_id) {
        this.joinedRooms.add(parsed.data.conversation_id);
      } else if (parsed.type === 'leave_room' && parsed.data?.conversation_id) {
        this.joinedRooms.delete(parsed.data.conversation_id);
      }
    } catch {
      // non-JSON message, ignore
    }
  }

  disconnect(): void {
    this.intentionalClose = true;
    this.clearRetryTimer();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.queue.length = 0;
    this.joinedRooms.clear();
    this.setStatus('disconnected');
  }

  private doConnect(): void {
    this.intentionalClose = false;
    this.clearRetryTimer();
    this.setStatus('connecting');

    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      this.retryDelay = 1000;
      this.setStatus('connected');
      // 重连后发送缓存的消息
      this.flushQueue();
      // 重连后恢复房间订阅
      this.rejoinRooms();
    };

    this.ws.onmessage = (event: MessageEvent) => {
      try {
        const msg: StreamMessage = JSON.parse(event.data as string);
        this.onMessageCallback?.(msg);
      } catch {
        // 忽略无法解析的消息
      }
    };

    this.ws.onclose = (event: CloseEvent) => {
      this.setStatus('disconnected');
      if (!this.intentionalClose) {
        if (event.code === 1008) {
          // Server rejected (e.g. max connections) — long backoff to avoid storm
          this.retryDelay = 30_000;
        }
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = () => {
      // onclose 会紧随其后触发，重连逻辑在那里处理
    };
  }

  private scheduleReconnect(): void {
    const jitter = Math.random() * 500;
    this.retryTimer = setTimeout(() => {
      this.doConnect();
    }, this.retryDelay + jitter);
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
    const batch = this.queue.splice(0);
    for (let i = 0; i < batch.length; i++) {
      if (this.ws?.readyState !== WebSocket.OPEN) {
        this.queue.unshift(...batch.slice(i));
        break;
      }
      this.ws.send(batch[i]!);
    }
  }

  private rejoinRooms(): void {
    for (const conversationId of this.joinedRooms) {
      this.ws?.send(JSON.stringify({
        type: 'join_room',
        data: { conversation_id: conversationId },
      }));
    }
  }

  private setStatus(status: WsStatus): void {
    this.statusValue = status;
    this.statusListeners.forEach((fn) => fn(status));
  }
}
