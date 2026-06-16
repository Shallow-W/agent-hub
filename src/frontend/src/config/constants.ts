/**
 * 前端全局常量集中管理。
 *
 * 所有 localStorage key、超时、重试、分页等魔法数字统一在此声明，
 * 避免散落在各个 store / hook / component 中产生不一致。
 */

// ---------------------------------------------------------------------------
// localStorage keys
// ---------------------------------------------------------------------------

export const STORAGE_KEYS = {
  TOKEN: 'agenthub_token',
  USER: 'agenthub_user',
  ACTIVE_CONV: 'agenthub_active_conv',
  DIRECT_AGENT_CHATS: 'agenthub_direct_agent_chats',
  THEME: 'theme',
  NOTIFY_SOUND: 'agenthub_notify_sound',
  NOTIFY_DESKTOP: 'agenthub_notify_desktop',
} as const;

// ---------------------------------------------------------------------------
// Message pagination & cache
// ---------------------------------------------------------------------------

/** 单次拉取的消息条数（游标分页） */
export const PAGE_SIZE = 200;

/** 单个会话在内存中保留的最大消息条数 */
export const MAX_MESSAGES = 200;

/** 会话消息缓存 TTL，超过后重新拉取（毫秒） */
export const CACHE_TTL_MS = 30_000;

/** 未读消息拉取条数 */
export const UNREAD_FETCH_LIMIT = 200;

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

/** 初始重连延迟（毫秒） */
export const WS_RETRY_DELAY_MS = 1000;

/** 最大重连延迟（毫秒） */
export const WS_MAX_RETRY_DELAY_MS = 30_000;

/** 重连抖动范围（毫秒） */
export const WS_RETRY_JITTER_MS = 500;

/** 1008 关闭码的退避延迟（毫秒） */
export const WS_REJECTION_BACKOFF_MS = 30_000;

// ---------------------------------------------------------------------------
// UI timers
// ---------------------------------------------------------------------------

/** Typing 指示器自动消失时间（毫秒） */
export const TYPING_TIMEOUT_MS = 3000;

/** 消息撤回去重窗口（毫秒） */
export const RECALL_DEDUP_TTL_MS = 30_000;

/** ConnectComputerModal 轮询间隔（毫秒） */
export const CONNECT_POLL_INTERVAL_MS = 3000;

// ---------------------------------------------------------------------------
// Upload limits
// ---------------------------------------------------------------------------

/** 最大上传文件大小（字节） */
export const MAX_FILE_SIZE = 50 * 1024 * 1024;

// ---------------------------------------------------------------------------
// API client
// ---------------------------------------------------------------------------

/** 401 自动重试次数 */
export const API_MAX_RETRY = 1;

/** 重试延迟（毫秒） */
export const API_RETRY_DELAY_MS = 1000;
