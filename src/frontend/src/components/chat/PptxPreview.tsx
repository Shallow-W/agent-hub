import React, { useEffect, useRef, useState } from 'react';
import { init as initPptxPreviewer } from 'pptx-preview';
import { Button } from 'antd';
import { DownloadOutlined, LoadingOutlined, FileExclamationOutlined } from '@ant-design/icons';
import styles from './PptxPreview.module.css';

interface PptxPreviewProps {
  /** 已带鉴权 token 的文件下载地址，用于 fetch ArrayBuffer 与“下载”按钮。 */
  fileUrl: string;
  fileName: string;
}

type LoadState = 'loading' | 'ready' | 'error';

/** 渲染宽度上限，避免大屏下幻灯片被拉得过宽。 */
const MAX_SLIDE_WIDTH = 960;

/**
 * 全屏 PPTX 幻灯片预览。仅在打开 Modal 时通过 React.lazy 加载本组件，
 * 从而把较重的 pptx-preview 库挡在首屏 bundle 之外。
 * 流程：fetch(fileUrl) → arrayBuffer() → pptx-preview 渲染到 host div。
 *
 * pptx-preview 渲染语义（依据 dist/pptx-preview.es.js 源码核实）：
 * - scale = options.width / pptx.width；每页 slide 渲染高度 = pptx.height * scale，
 *   **比例取自 pptx 真实尺寸**（4:3 deck 即 4:3，16:9 即 16:9），库本身不会拉伸变形。
 * - list 模式下每页 slide 的 margin 是 `0 auto 10px`，**没有**垂直居中留白；
 *   黑边来源是 `_renderWrapper` 给外层 `.pptx-preview-wrapper` 写死了
 *   `background:#000` + `height = options.height` + `overflow-y:auto`：
 *   当 options.height（旧实现按 16:9 算）与真实堆叠高度不符时，
 *   要么把单页裁切，要么露出 #000 黑底。
 *
 * 因此本实现：
 * 1. 不再假设 16:9——init 时给一个足够高的占位 height，避免渲染瞬间裁切；
 * 2. preview() 完成后从 previewer.pptx 读真实 width/height 算真实 aspect，
 *    把库写死的 wrapper 改成 height:auto / overflow:visible / 透明背景，
 *    让“大背景 + 居中 + 滚动”交给我们自己的 host 容器，从而 4:3 / 16:9 都不裁切、无黑边。
 */
const PptxPreview: React.FC<PptxPreviewProps> = ({ fileUrl, fileName }) => {
  // measureRef 始终可见，用它量真实宽度；hostRef 是 pptx-preview 的挂载点。
  const measureRef = useRef<HTMLDivElement>(null);
  const hostRef = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<LoadState>('loading');

  useEffect(() => {
    let cancelled = false;
    // init() 返回类型化的 previewer 实例，destroy 用于卸载时清理 DOM/事件。
    let previewer: ReturnType<typeof initPptxPreviewer> | null = null;

    async function render() {
      const host = hostRef.current;
      const measure = measureRef.current;
      if (!host || !measure) return;
      setState('loading');
      try {
        const resp = await fetch(fileUrl);
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const buffer = await resp.arrayBuffer();
        if (cancelled || !hostRef.current || !measureRef.current) return;

        // measure 容器始终可见，clientWidth 即真实可视宽度（封顶 960，居中）。
        const available = measureRef.current.clientWidth;
        const width = Math.min(available > 0 ? available : MAX_SLIDE_WIDTH, MAX_SLIDE_WIDTH);
        previewer = initPptxPreviewer(hostRef.current, {
          width,
          // 给一个足够高的占位 height（最高比例 4:3 也够），避免渲染瞬间被 wrapper 裁切；
          // 真实高度在 preview() 后据 pptx 尺寸重算并接管，这里只是临时不裁切。
          height: Math.round(width * (3 / 4)),
          mode: 'list',
        });
        await previewer.preview(buffer);
        if (cancelled) return;

        // preview() 后 previewer.pptx 已填充真实 slide 尺寸（EMU），据此算真实 aspect。
        const realW = previewer.pptx?.width ?? 0;
        const realH = previewer.pptx?.height ?? 0;
        const wrapper = previewer.wrapper as HTMLElement | undefined;
        if (wrapper) {
          // 取消库写死的 #000 固定高黑盒：高度交给内容（堆叠的 slide），
          // 背景/滚动/居中交给我们自己的 host 容器，4:3 与 16:9 都不裁切、无黑边。
          wrapper.style.setProperty('height', 'auto');
          wrapper.style.setProperty('overflow-y', 'visible');
          wrapper.style.setProperty('background', 'transparent');
          if (realW > 0 && realH > 0) {
            // 真实单页高度（与库内部 scale = width/realW 一致），用于校正每页 slide，
            // 防止占位 height 残留在内联样式里造成纵向偏差。
            const slideH = Math.round(width * (realH / realW));
            wrapper.querySelectorAll<HTMLElement>('.pptx-preview-slide-wrapper').forEach((slide) => {
              slide.style.setProperty('height', `${slideH}px`);
              slide.style.setProperty('width', `${width}px`);
            });
          }
        }
        setState('ready');
      } catch {
        if (!cancelled) setState('error');
      }
    }

    render();

    return () => {
      cancelled = true;
      try {
        previewer?.destroy();
      } catch {
        /* 渲染未完成时 destroy 可能抛错，忽略即可。 */
      }
    };
  }, [fileUrl]);

  return (
    // measure 容器始终可见、占满预览区，既用于量宽度，也作为 loading/error 遮罩的定位父级。
    <div ref={measureRef} className={styles.measure}>
      {/* host：大、深色、居中的背景容器；幻灯片在其中水平居中、纵向堆叠、可滚动。 */}
      <div ref={hostRef} className={styles.host} />
      {state === 'loading' && (
        <div className={styles.overlay}>
          <LoadingOutlined className={styles.spinner} spin />
          <span>正在解析幻灯片…</span>
        </div>
      )}
      {state === 'error' && (
        <div className={styles.overlay}>
          <FileExclamationOutlined className={styles.errorIcon} />
          <span className={styles.errorText}>无法预览，请下载查看。</span>
          <Button
            type="primary"
            icon={<DownloadOutlined />}
            href={fileUrl}
            download={fileName}
            className={styles.downloadBtn}
          >
            下载文件
          </Button>
        </div>
      )}
    </div>
  );
};

export default PptxPreview;
