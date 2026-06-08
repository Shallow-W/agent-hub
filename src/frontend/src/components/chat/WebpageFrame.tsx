import React, { useEffect, useRef, useState } from 'react';
import { Spin } from 'antd';
import styles from './WebpageFrame.module.css';

interface Props {
  /** 网页 URL（与 srcDoc 二选一） */
  url?: string;
  /** 完整 HTML 文档内容（与 url 二选一） */
  srcDoc?: string;
}

// iframe 加载超时（毫秒）：超时仍未 onload 视为被 CSP/跨域拦截，走 error 兜底
const LOAD_TIMEOUT = 8000;

/**
 * 网页产物的 sandbox iframe 预览。
 * - sandbox 仅给 allow-scripts，且故意不与 allow-same-origin 同时给不可信内容，
 *   避免沙箱逃逸。
 * - loading spinner + error 兜底（onError / 超时 / 无法预览时提示打开原链接）。
 */
export const WebpageFrame: React.FC<Props> = ({ url, srcDoc }) => {
  const [loading, setLoading] = useState(true);
  const [errored, setErrored] = useState(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    setLoading(true);
    setErrored(false);
    // 超时兜底：CSP 拦截 / 跨域 X-Frame-Options 不会触发 onError，靠超时识别
    timerRef.current = window.setTimeout(() => {
      setLoading((stillLoading) => {
        if (stillLoading) setErrored(true);
        return false;
      });
    }, LOAD_TIMEOUT);
    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
    };
  }, [url, srcDoc]);

  const handleLoad = () => {
    if (timerRef.current) window.clearTimeout(timerRef.current);
    setLoading(false);
  };

  const handleError = () => {
    if (timerRef.current) window.clearTimeout(timerRef.current);
    setLoading(false);
    setErrored(true);
  };

  const sandbox = srcDoc
    ? 'allow-scripts allow-modals allow-popups allow-popups-to-escape-sandbox allow-forms allow-top-navigation'
    : 'allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation';

  return (
    <div className={styles.frameWrap}>
      {!errored && (
        <iframe
          className={styles.frame}
          title={url || '网页产物预览'}
          src={url}
          srcDoc={srcDoc}
          sandbox={sandbox}
          referrerPolicy="no-referrer"
          onLoad={handleLoad}
          onError={handleError}
        />
      )}
      {loading && !errored && (
        <div className={styles.overlay}>
          <Spin />
          <span>加载中…</span>
        </div>
      )}
      {errored && (
        <div className={styles.overlay}>
          <span className={styles.errorText}>无法预览此网页（可能被对方站点 CSP / 跨域限制拦截）</span>
          {url && (
            <a className={styles.openLink} href={url} target="_blank" rel="noopener noreferrer">
              打开原链接
            </a>
          )}
        </div>
      )}
    </div>
  );
};
