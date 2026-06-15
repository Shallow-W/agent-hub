import { message as fallbackMessage } from 'antd';

type MessageApi = Pick<
  typeof fallbackMessage,
  'success' | 'error' | 'warning' | 'info' | 'loading' | 'open' | 'destroy'
>;

let activeMessage: MessageApi = fallbackMessage;

export function bindMessage(instance: MessageApi | null): void {
  activeMessage = instance ?? fallbackMessage;
}

function getMessage(): MessageApi {
  return activeMessage;
}

export const message: MessageApi = {
  success: (...args) => getMessage().success(...args),
  error: (...args) => getMessage().error(...args),
  warning: (...args) => getMessage().warning(...args),
  info: (...args) => getMessage().info(...args),
  loading: (...args) => getMessage().loading(...args),
  open: (...args) => getMessage().open(...args),
  destroy: (...args) => getMessage().destroy(...args),
};
