import React, { useMemo, useState } from 'react';
import { Typography, Radio, Button } from 'antd';
import { LeftOutlined, RightOutlined, CheckCircleFilled } from '@ant-design/icons';
import type { CardProps, PlanCard as PlanCardData } from '@/types/card';
import styles from './Cards.module.css';

/**
 * 方案选择卡片（card_type=plan）—— 支持多问题翻页。
 *
 * 交互：
 *   - 多个 question 通过 ‹ › 翻页查看，头部显示当前问题标题 + 分页指示（2/3）
 *   - 每个 question 单选，临时选择存入本地 answers，未提交前可改
 *   - 统一提交：只有所有问题都选了，最后一页的"提交全部选择"按钮才可用
 *   - 提交后（card.state === 'resolved'）整卡只读，各问题显示已选答案，仍可翻页查看
 *
 * 历史态（刷新后）：card.state === 'resolved' → 直接进入只读，从 question.selected_option 恢复。
 */
export const PlanCard: React.FC<CardProps<PlanCardData>> = ({ card, onAction }) => {
  const isResolved = card.state === 'resolved';
  const questions = card.questions ?? [];

  // 已提交态下，从 question.selected_option 恢复一份 answers（用于只读展示）
  const persistedAnswers = useMemo(() => {
    const m: Record<string, string> = {};
    for (const q of questions) {
      if (q.selected_option) m[q.id] = q.selected_option;
    }
    return m;
  }, [questions]);

  // 本地临时选择（未提交时）：questionId → optionId
  const [answers, setAnswers] = useState<Record<string, string>>(persistedAnswers);
  // 当前问题索引
  const [currentIdx, setCurrentIdx] = useState(0);

  // 边界保护：空 questions 或索引越界
  if (questions.length === 0) {
    return (
      <div className={styles.card}>
        <Typography.Text type="secondary">（无问题）</Typography.Text>
      </div>
    );
  }
  const safeIdx = Math.min(currentIdx, questions.length - 1);
  // 边界已保证 questions 非空，safeIdx ∈ [0, length-1]，取值必非 undefined
  const current = questions[safeIdx]!;
  const total = questions.length;
  const isLast = safeIdx === total - 1;
  const currentAnswer = answers[current.id];

  // 是否所有问题都已选择（提交按钮的启用条件）
  const allAnswered = questions.every((q) => Boolean(answers[q.id]));

  const handleSelect = (optionId: string) => {
    if (isResolved) return; // 只读
    setAnswers((prev) => ({ ...prev, [current.id]: optionId }));
  };

  const handleSubmit = () => {
    if (isResolved || !allAnswered) return;
    onAction(card.id, 'submit_plan', { answers });
  };

  // 只读态下用 persistedAnswers，否则用本地 answers
  const displayAnswer = isResolved ? persistedAnswers[current.id] : currentAnswer;

  return (
    <div className={styles.card}>
      {/* 分页导航行：当前问题标题 + 指示 + 翻页按钮 */}
      <div className={styles.planPager}>
        <Button
          className={styles.planNavBtn}
          type="text"
          size="small"
          shape="circle"
          icon={<LeftOutlined />}
          disabled={safeIdx === 0}
          onClick={() => setCurrentIdx(safeIdx - 1)}
          aria-label="上一题"
        />
        <div className={styles.planPagerTitle}>
          <Typography.Text strong>{current.title || `问题 ${safeIdx + 1}`}</Typography.Text>
          {total > 1 && (
            <span className={styles.planPagerInfo}>{safeIdx + 1} / {total}</span>
          )}
        </div>
        <Button
          className={styles.planNavBtn}
          type="text"
          size="small"
          shape="circle"
          icon={<RightOutlined />}
          disabled={safeIdx === total - 1}
          onClick={() => setCurrentIdx(safeIdx + 1)}
          aria-label="下一题"
        />
      </div>

      {/* 卡片总标题（若有，且与当前问题标题不同时显示） */}
      {card.title && card.title !== current.title && (
        <div className={styles.cardHeader}>
          <Typography.Text type="secondary" className={styles.cardTitle}>{card.title}</Typography.Text>
        </div>
      )}

      {/* 选项区：单选 */}
      <Radio.Group
        value={displayAnswer}
        onChange={(e) => handleSelect(e.target.value)}
        disabled={isResolved}
        className={styles.optionList}
      >
        {current.options?.map((opt) => (
          <Radio key={opt.id} value={opt.id} className={styles.optionItem}>
            <div className={styles.optionContent}>
              <span className={styles.optionLabel}>{opt.label}</span>
              {opt.recommended && <span className={styles.recommendBadge}>推荐</span>}
            </div>
            {opt.description && (
              <div className={styles.optionDesc}>{opt.description}</div>
            )}
          </Radio>
        ))}
      </Radio.Group>

      {/* 底部操作区 */}
      {!isResolved && (
        <div className={styles.cardFooter}>
          <Button
            size="small"
            disabled={safeIdx === 0}
            onClick={() => setCurrentIdx(safeIdx - 1)}
          >
            上一题
          </Button>
          {isLast ? (
            <Button
              type="primary"
              size="small"
              disabled={!allAnswered}
              onClick={handleSubmit}
            >
              提交全部选择
            </Button>
          ) : (
            <Button
              type="primary"
              size="small"
              disabled={!currentAnswer}
              onClick={() => setCurrentIdx(safeIdx + 1)}
            >
              下一题
            </Button>
          )}
        </div>
      )}
      {isResolved && (
        <div className={styles.cardFooter}>
          <Typography.Text type="success">
            <CheckCircleFilled /> 已提交 {Object.keys(persistedAnswers).length}/{total}
          </Typography.Text>
        </div>
      )}
    </div>
  );
};
