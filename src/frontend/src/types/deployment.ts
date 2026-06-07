/**
 * 部署记录：将一个 artifact（按血缘根）落盘为可访问站点 / 可打包下载的产物。
 * 字段名严格对齐后端 model.Deployment（三层对齐真源）。
 */
export type DeploymentStatus = 'pending' | 'success' | 'failed';

export interface Deployment {
  id: string;
  artifact_root_id: string;
  conversation_id: string;
  mode: string;
  status: DeploymentStatus;
  /** 预览访问地址：默认相对路径（如 /api/sites/{id}/index.html）；配置公网基址时为绝对地址 */
  url?: string;
  /** 源码 zip 下载地址：默认相对路径；配置公网基址时为绝对地址 */
  download_url?: string;
  error?: string;
  created_at: string;
}
