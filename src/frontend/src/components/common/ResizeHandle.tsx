import React, { useCallback, useRef, useState } from 'react';
import styles from './ResizeHandle.module.css';

interface ResizeHandleProps {
  onResize: (deltaX: number) => void;
}

const ResizeHandle: React.FC<ResizeHandleProps> = ({ onResize }) => {
  const startXRef = useRef(0);
  const activeRef = useRef(false);
  const [dragging, setDragging] = useState(false);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      startXRef.current = e.clientX;
      activeRef.current = true;
      setDragging(true);

      const handleMove = (ev: MouseEvent) => {
        if (!activeRef.current) return;
        const deltaX = ev.clientX - startXRef.current;
        startXRef.current = ev.clientX;
        onResize(deltaX);
      };

      const handleUp = () => {
        activeRef.current = false;
        setDragging(false);
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        document.removeEventListener('mousemove', handleMove);
        document.removeEventListener('mouseup', handleUp);
      };

      // 拖拽时禁止文本选择并显示调整光标
      document.body.style.cursor = 'col-resize';
      document.body.style.userSelect = 'none';
      document.addEventListener('mousemove', handleMove);
      document.addEventListener('mouseup', handleUp);
    },
    [onResize],
  );

  const cls = dragging
    ? `${styles.handle} ${styles.handleActive}`
    : styles.handle;

  return (
    <div
      className={cls}
      onMouseDown={handleMouseDown}
    />
  );
};

export default ResizeHandle;
