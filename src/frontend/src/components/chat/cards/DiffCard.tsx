import React, { useEffect, useRef, useState } from 'react';
import {
  FileAddOutlined,
  FileExcelOutlined,
  FileExclamationOutlined,
} from '@ant-design/icons';
import { Spin } from 'antd';
import type { CardProps, DiffCard as DiffCardData } from '@/types/card';
import { fileStatus, fileDiff, type DiffContent } from '@/api/files';
import type { ChangeStatus } from '@/api/files';
import { ApiError } from '@/api/client';
import { message as antMessage } from '@/utils/message';
import { asyncPool } from '@/utils/asyncPool';
import { DiffViewer } from '@/components/chat/DiffViewer';
import styles from './Cards.module.css';

const STATUS_ICON: Record<ChangeStatus, React.ReactNode> = {
  added: <FileAddOutlined className={styles.iconAdded} />,
  modified: <FileExclamationOutlined className={styles.iconModified} />,
  deleted: <FileExcelOutlined className={styles.iconDeleted} />,
};

const STATUS_LABEL: Record<ChangeStatus, string> = {
  added: '新增',
  modified: '修改',
  deleted: '删除',
};

/**
 * 文件变更卡片（card_type=diff）——只读。
 *
 * 数据契约（解耦设计）：agent 只上报 workDir + files（相对路径数组），
 * 不输出 status 或 diff 内容。卡片打开时由前端调 fileStatus 查 git 状态，
 * 同时预取每个文件的 fileDiff（点文件时秒开 DiffViewer，不必再等 RPC）。
 */
export const DiffCard: React.FC<CardProps<DiffCardData>> = ({ card, agentId }) => {
  const [statuses, setStatuses] = useState<{ path: string; status: ChangeStatus }[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedFile, setSelectedFile] = useState<string>('');
  const [viewerOpen, setViewerOpen] = useState(false);
  // 预取的 diff 缓存：path → 前后内容。ref 不触发重渲染；
  // 用户点击文件时 handleClickFile 会 setSelectedFile 触发渲染，那时从 ref 读最新缓存。
  const diffCacheRef = useRef<Map<string, DiffContent>>(new Map());

  // 打开卡片时查 git status（agent 只报了文件路径，status 由平台查），
  // 同时预取每个文件的 fileDiff，点文件时 DiffViewer 直接用缓存秒开。
  useEffect(() => {
    if (!agentId || !card.workDir || card.files.length === 0) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    fileStatus(agentId, card.workDir, card.files)
      .then((result) => {
        if (cancelled) return;
        // agent 报的文件可能 git 查不到 status（非 git 目录/未跟踪），兜底标为 modified
        const statusMap = new Map(result.map((s) => [s.path, s.status]));
        setStatuses(
          card.files.map((f) => ({ path: f, status: statusMap.get(f) || 'modified' })),
        );
      })
      .catch((err) => {
        if (!cancelled) {
          // 查询失败时仍展示文件列表（兜底全标 modified），不阻塞用户点击查看 diff
          setStatuses(card.files.map((f) => ({ path: f, status: 'modified' as ChangeStatus })));
          if (err instanceof ApiError && err.status !== 503) {
            antMessage.error(err.message);
          }
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    // 预取 diff：限制 3 并发，避免大改动（20+ 文件）瞬间打爆 daemon。
    // 失败静默——DiffViewer 打开时若缓存缺失会自行兜底再请求一次。
    void asyncPool(
      card.files,
      (f) => fileDiff(agentId, card.workDir, f),
      3,
    ).then((results) => {
      if (cancelled) return;
      results.forEach((r, i) => {
        if (r.status === 'fulfilled') {
          diffCacheRef.current.set(card.files[i]!, r.value);
        }
      });
    });
    return () => {
      cancelled = true;
    };
  }, [agentId, card.workDir, card.files]);

  const handleClickFile = (relPath: string) => {
    if (!agentId) return;
    setSelectedFile(relPath);
    setViewerOpen(true);
  };

  return (
    <>
      <div className={styles.diffCard}>
        <div className={styles.diffHeader}>
          <span>{card.title || '文件变更'}</span>
          <span className={styles.diffCount}>{card.files.length} 个文件</span>
        </div>
        {loading ? (
          <div className={styles.diffLoading}>
            <Spin size="small" /> 查询改动状态...
          </div>
        ) : (
          <ul className={styles.diffList}>
            {statuses.map((s, idx) => (
              <li
                key={`${s.path}-${idx}`}
                className={styles.diffItem}
                onClick={() => handleClickFile(s.path)}
                title="点击查看前后对比"
              >
                {STATUS_ICON[s.status]}
                <span className={styles.diffPath}>{s.path}</span>
                <span className={`${styles.diffBadge} ${styles[`badge_${s.status}`] || ''}`}>
                  {STATUS_LABEL[s.status]}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
      {agentId && selectedFile && (
        <DiffViewer
          open={viewerOpen}
          onClose={() => setViewerOpen(false)}
          agentId={agentId}
          workDir={card.workDir}
          filePath={selectedFile}
          title={card.title}
          initialDiff={diffCacheRef.current.get(selectedFile)}
        />
      )}
    </>
  );
};
