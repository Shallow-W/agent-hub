import { post } from './client';
import type { Deployment } from '@/types/deployment';

/** 部署某血缘根的最新产物，返回部署记录（url 为相对路径）。 */
export async function deployArtifact(rootId: string): Promise<Deployment> {
  return post<Deployment>(`/api/artifacts/${rootId}/deploy`, {});
}

/** 把产物发布到 GitHub Pages（永久公网地址），返回部署记录（url 为绝对 github.io 地址）。 */
export async function publishToGitHub(rootId: string): Promise<Deployment> {
  return post<Deployment>(`/api/artifacts/${rootId}/deploy-github`, {});
}

/** 把后端返回的相对地址拼成当前来源下的绝对地址（适配二维码扫码 / 局域网 / 生产同源）。 */
export function absoluteDeployURL(relative?: string): string {
  if (!relative) return '';
  if (/^https?:\/\//.test(relative)) return relative;
  return `${window.location.origin}${relative}`;
}

/** 部署产物的打包下载直链（公开，凭 deployment id 访问）。
 *  末段带 .zip 文件名，确保浏览器（含跨域下载）存成正确的 zip 而非无扩展名裸 UUID。 */
export function deploymentDownloadURL(id: string): string {
  return `${window.location.origin}/api/deployments/${id}/download/deployment-${id}.zip`;
}
