import { ApiError } from './client';

export interface ConvErrorInterpretation {
  message: string;
  retryable: boolean;
}

/**
 * 后端 conversation handler 错误码 -> 前端语义化文案。
 * 来源：src/backend/internal/handler/conversation.go
 */
const CONV_ERROR_MAP: Record<number, ConvErrorInterpretation> = {
  40015: { message: '参数错误（缺少对话 ID）', retryable: false },
  40016: { message: '角色无效，仅支持 orchestrator 或 worker', retryable: false },
  40315: { message: '没有权限管理此会话的 Agent', retryable: false },
  40415: { message: '会话不存在', retryable: false },
  40915: { message: '并发设置 Orchestrator 冲突', retryable: true },
  50016: { message: '服务器内部错误', retryable: false },
  50017: { message: '服务器内部错误', retryable: false },
};

/**
 * 将 conversation API 抛出的错误转译为用户友好的语义化文案，
 * 同时标记是否可重试（用于 toast 提示附加"请重试"）。
 */
export function interpretConvError(err: unknown): ConvErrorInterpretation {
  if (err instanceof ApiError && err.code) {
    const mapped = CONV_ERROR_MAP[err.code];
    if (mapped) return mapped;
  }
  return { message: err instanceof Error ? err.message : '操作失败', retryable: false };
}
