import React from 'react';
import { QRCode, Tag, Button, Tooltip } from 'antd';
import { message } from '@/utils/message';
import {
  CopyOutlined,
  DownloadOutlined,
  GlobalOutlined,
  LinkOutlined,
} from '@ant-design/icons';
import type { Deployment } from '@/types/deployment';
import { absoluteDeployURL, deploymentDownloadURL } from '@/api/deployment';
import styles from './DeployStatusCard.module.css';

const STATUS_META: Record<Deployment['status'], { color: string; label: string }> = {
  pending: { color: 'processing', label: '部署中' },
  success: { color: 'success', label: '部署成功' },
  failed: { color: 'error', label: '部署失败' },
};

interface Props {
  deployment: Deployment;
}

/** 部署状态卡片：状态徽标 + 预览链接 + 二维码 + 源码下载。 */
export const DeployStatusCard: React.FC<Props> = ({ deployment }) => {
  const meta = STATUS_META[deployment.status] ?? STATUS_META.pending;
  const previewUrl = absoluteDeployURL(deployment.url);
  // 优先用后端给的 download_url（配置公网基址时为绝对地址），否则按当前来源兜底拼接。
  const downloadUrl = deployment.download_url
    ? absoluteDeployURL(deployment.download_url)
    : deploymentDownloadURL(deployment.id);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(previewUrl);
      message.success('链接已复制');
    } catch {
      message.error('复制失败');
    }
  };

  return (
    <div className={styles.card}>
      <div className={styles.header}>
        <GlobalOutlined className={styles.headerIcon} />
        <span className={styles.title}>部署发布</span>
        <Tag color={meta.color} className={styles.status}>{meta.label}</Tag>
      </div>

      {deployment.status === 'success' && (
        <div className={styles.body}>
          <div className={styles.left}>
            <div className={styles.urlRow}>
              <LinkOutlined className={styles.urlIcon} />
              <a
                className={styles.url}
                href={previewUrl}
                target="_blank"
                rel="noopener noreferrer"
              >
                {previewUrl}
              </a>
              <Tooltip title="复制链接">
                <Button size="small" type="text" icon={<CopyOutlined />} onClick={copy} />
              </Tooltip>
            </div>
            <div className={styles.actions}>
              <Button
                type="primary"
                size="small"
                href={previewUrl}
                target="_blank"
                icon={<GlobalOutlined />}
              >
                打开预览
              </Button>
              <Button size="small" href={downloadUrl} icon={<DownloadOutlined />}>
                下载源码 zip
              </Button>
            </div>
            <div className={styles.hint}>扫码可在手机上打开预览</div>
          </div>
          <div className={styles.qr}>
            <QRCode value={previewUrl} size={104} bordered={false} />
          </div>
        </div>
      )}

      {deployment.status === 'failed' && (
        <div className={styles.error}>{deployment.error || '部署失败，请重试'}</div>
      )}
    </div>
  );
};
