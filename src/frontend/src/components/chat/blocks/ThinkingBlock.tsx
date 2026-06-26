import React, { useState, useMemo, useEffect, useRef } from 'react';
import { LoadingOutlined } from '@ant-design/icons';
import type { MessageBlock } from '@/types/message';
import type { BlockRenderContext } from './BlockRegistry';
import styles from './Blocks.module.css';
import { registerBlock } from './BlockRegistry';

interface ThinkingBlockProps {
  block: MessageBlock;
  /** 默认折叠；点击切换 */
  defaultExpanded?: boolean;
  /** streaming 状态时显示末尾闪烁光标 + 自动展开 */
  streaming?: boolean;
  /** registry 签名要求；ThinkingBlock 不依赖上下文，忽略 */
  ctx?: BlockRenderContext;
}

/**
 * thinking block——默认折叠，streaming 时自动展开，完成后自动收起。
 *
 * UX 设计（参考 Claude Code / ChatGPT 思考 UI）：
 *   - 折叠时显示脑图标 + "已思考 Ns"（或 "思考中..." pulse）
 *   - streaming 时自动展开，展示流式 token + 末尾光标
 *   - streaming 结束后 300ms 延迟自动收起（避免立即抖动）
 *   - 用户手动 toggle 后，不再自动收起（尊重用户意图）
 *   - 平滑 max-height 过渡（CSS transition）
 *
 * 同一思考过程流式累积的 text，append 到同一 block。
 */
function ThinkingBlockInner({ block, defaultExpanded = false, streaming = false }: ThinkingBlockProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  // 用户是否手动操作过——一旦操作，自动展开/收起逻辑让位
  const userTouched = useRef(false);
  // 流式期间记录开始时间，结束后计算 duration
  const streamStartRef = useRef<number | null>(null);
  const [durationSec, setDurationSec] = useState<number | null>(null);

  const text = useMemo(() => block.text ?? '', [block.text]);
  const textLength = text.length;

  // streaming 开始：记录时间戳 + 自动展开（除非用户手动收起过）
  useEffect(() => {
    if (streaming) {
      if (streamStartRef.current === null) {
        streamStartRef.current = Date.now();
      }
      if (!userTouched.current) {
        setExpanded(true);
      }
    } else {
      // streaming 结束：计算 duration + 自动收起（除非用户手动展开过）
      if (streamStartRef.current !== null) {
        const elapsed = Math.max(1, Math.round((Date.now() - streamStartRef.current) / 1000));
        setDurationSec(elapsed);
        streamStartRef.current = null;
      }
      if (!userTouched.current) {
        // 300ms 延迟，让光标先消失再收起，避免视觉抖动
        const timer = setTimeout(() => setExpanded(false), 300);
        return () => clearTimeout(timer);
      }
    }
  }, [streaming]);

  // 持续累积的 thinking 文本长度，用于折叠态 preview 显示首行
  const preview = useMemo(() => {
    if (!textLength) return '';
    const firstLine = text.split('\n', 1)[0] ?? '';
    const trimmed = firstLine.trim();
    if (trimmed.length <= 48) return trimmed;
    return trimmed.slice(0, 48) + '…';
  }, [text, textLength]);

  const handleToggle = () => {
    userTouched.current = true;
    setExpanded((v) => !v);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      handleToggle();
    }
  };

  const statusLabel = streaming
    ? '思考中'
    : durationSec !== null
      ? `已思考 ${durationSec}s`
      : '思考过程';

  return (
    <div
      className={`${styles.thinkingBlock} ${streaming ? styles.thinkingStreaming : ''}`}
      aria-label="AI 思考过程"
    >
      <div
        className={styles.thinkingHeader}
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onClick={handleToggle}
        onKeyDown={handleKeyDown}
      >
        <span className={styles.thinkingIcon} aria-hidden>
          {streaming ? <LoadingOutlined spin /> : <ThinkingBrainIcon />}
        </span>
        <span className={styles.thinkingLabel}>{statusLabel}</span>
        {!expanded && preview && (
          <span className={styles.thinkingPreview} aria-hidden>
            {preview}
          </span>
        )}
        <span className={styles.thinkingChevron} aria-hidden>
          {expanded ? '▾' : '▸'}
        </span>
      </div>
      <div
        className={`${styles.thinkingBodyWrap} ${expanded ? styles.thinkingBodyExpanded : ''}`}
      >
        <div className={styles.thinkingBody}>
          {text}
          {streaming && <span className={styles.streamingCursor} aria-hidden />}
        </div>
      </div>
      {streaming && (
        <div className={styles.thinkingProgress} aria-hidden>
          <div className={styles.thinkingProgressBar} />
        </div>
      )}
    </div>
  );
}

/**
 * 脑图标 SVG inline——避免引入新依赖。
 * 简洁的大脑轮廓，区别于 ant-design 通用 icon。
 */
function ThinkingBrainIcon() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      {/* 简化大脑轮廓：两个对称凸起 + 中央分割线 */}
      <path d="M12 5a3 3 0 0 0-3 3v1a3 3 0 0 0-2 2.83V14a3 3 0 0 0 2 2.83V18a3 3 0 0 0 3 3" />
      <path d="M12 5a3 3 0 0 1 3 3v1a3 3 0 0 1 2 2.83V14a3 3 0 0 1-2 2.83V18a3 3 0 0 1-3 3" />
      <path d="M12 5v16" />
    </svg>
  );
}

export const ThinkingBlock = React.memo(ThinkingBlockInner);

// 自注册：思考块——默认折叠，streaming 时显示光标 + 脑图标，完成后显示已思考时长
registerBlock('thinking', { component: ThinkingBlock });
