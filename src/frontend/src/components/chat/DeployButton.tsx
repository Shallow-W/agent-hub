import React, { useEffect, useState } from 'react';
import { Dropdown, Button, Modal } from 'antd';
import type { MenuProps } from 'antd';
import { message } from '@/utils/message';
import { CloudUploadOutlined, GlobalOutlined, GithubOutlined } from '@ant-design/icons';
import type { Artifact } from '@/types/message';
import type { Deployment } from '@/types/deployment';
import { deployArtifact, getDeploymentCapabilities, publishToGitHub } from '@/api/deployment';
import { DeployStatusCard } from './DeployStatusCard';

interface Props {
  artifact: Artifact;
  size?: 'small' | 'middle';
  text?: boolean;
}

export const DeployButton: React.FC<Props> = ({ artifact, size = 'small', text }) => {
  const rootId = artifact.root_id || artifact.id;
  const [loading, setLoading] = useState(false);
  const [deployment, setDeployment] = useState<Deployment | null>(null);
  const [open, setOpen] = useState(false);
  const [githubEnabled, setGithubEnabled] = useState(false);

  useEffect(() => {
    let alive = true;
    getDeploymentCapabilities()
      .then((capabilities) => {
        if (alive) setGithubEnabled(capabilities.github_enabled);
      })
      .catch(() => {
        if (alive) setGithubEnabled(false);
      });
    return () => {
      alive = false;
    };
  }, []);

  if (!rootId) return null;

  const run = async (target: 'tunnel' | 'github') => {
    setLoading(true);
    try {
      const dep = target === 'github' ? await publishToGitHub(rootId) : await deployArtifact(rootId);
      setDeployment(dep);
      setOpen(true);
      if (dep.status === 'failed') {
        message.error(dep.error || '部署失败');
      } else if (target === 'github') {
        message.success('GitHub Pages 已发布并验证可访问');
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '部署失败');
    } finally {
      setLoading(false);
    }
  };

  const items: MenuProps['items'] = [
    {
      key: 'tunnel',
      icon: <GlobalOutlined />,
      label: '即时预览（内网穿透）',
    },
    ...(githubEnabled
      ? [
          {
            key: 'github',
            icon: <GithubOutlined />,
            label: '永久发布到 GitHub Pages（需等待可访问）',
          },
        ]
      : []),
  ];

  const onClick: MenuProps['onClick'] = ({ key, domEvent }) => {
    domEvent.stopPropagation();
    void run(key as 'tunnel' | 'github');
  };

  const stop = (e: React.MouseEvent) => e.stopPropagation();

  return (
    <>
      <Dropdown menu={{ items, onClick }} trigger={['click']}>
        <Button
          size={size}
          type={text ? 'text' : 'default'}
          loading={loading}
          icon={<CloudUploadOutlined />}
          onClick={stop}
        >
          部署
        </Button>
      </Dropdown>
      <div onClick={stop} role="presentation">
        <Modal
          open={open}
          onCancel={() => setOpen(false)}
          footer={null}
          width={520}
          title="部署发布"
          destroyOnHidden
        >
          {deployment && <DeployStatusCard deployment={deployment} />}
        </Modal>
      </div>
    </>
  );
};
