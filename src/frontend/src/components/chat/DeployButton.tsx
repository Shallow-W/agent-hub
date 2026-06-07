import React, { useState } from 'react';
import { Button, Modal, message } from 'antd';
import { CloudUploadOutlined } from '@ant-design/icons';
import type { Artifact } from '@/types/message';
import type { Deployment } from '@/types/deployment';
import { deployArtifact } from '@/api/deployment';
import { DeployStatusCard } from './DeployStatusCard';

interface Props {
  artifact: Artifact;
  size?: 'small' | 'middle';
  /** 文字按钮（无边框），用于卡片内嵌不抢视觉 */
  text?: boolean;
}

/** 部署按钮：触发产物部署，成功后弹出部署状态卡片。 */
export const DeployButton: React.FC<Props> = ({ artifact, size = 'small', text }) => {
  const rootId = artifact.root_id || artifact.id;
  const [loading, setLoading] = useState(false);
  const [deployment, setDeployment] = useState<Deployment | null>(null);
  const [open, setOpen] = useState(false);

  if (!rootId) return null;

  const handleDeploy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setLoading(true);
    try {
      const dep = await deployArtifact(rootId);
      setDeployment(dep);
      setOpen(true);
      if (dep.status === 'failed') {
        message.error(dep.error || '部署失败');
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '部署失败');
    } finally {
      setLoading(false);
    }
  };

  const stop = (e: React.MouseEvent) => e.stopPropagation();

  return (
    <>
      <Button
        size={size}
        type={text ? 'text' : 'default'}
        loading={loading}
        icon={<CloudUploadOutlined />}
        onClick={handleDeploy}
      >
        部署
      </Button>
      <div onClick={stop} role="presentation">
        <Modal
          open={open}
          onCancel={() => setOpen(false)}
          footer={null}
          width={520}
          title="部署发布"
          destroyOnClose
        >
          {deployment && <DeployStatusCard deployment={deployment} />}
        </Modal>
      </div>
    </>
  );
};
