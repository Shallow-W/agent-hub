// TODO(backend): replace fake data with real timeseries from
// GET /api/daemon/machines/:id/metrics (endpoint not yet implemented).
// Until then we render deterministic synthetic data per metric+machine.
import React, { useMemo } from 'react';
import {
  DashboardOutlined,
  DatabaseOutlined,
  ThunderboltOutlined,
  WifiOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';
import styles from './ResourceChart.module.css';

export type ResourceMetric = 'cpu' | 'memory' | 'disk' | 'network';

interface ResourceChartProps {
  metric: ResourceMetric;
  title: string;
  unit?: string;
  color?: string;
  machineId?: string;
}

interface MetricConfig {
  icon: ReactNode;
  color: string;
  unit: string;
  min: number;
  max: number;
  drift: 'up' | 'flat' | 'jitter';
  footer: (current: number, series: number[]) => ReactNode;
}

const POINTS = 20;

// Deterministic seeded PRNG (mulberry32) so the same metric+machine produces
// the same series across re-renders — no flicker when the parent re-renders.
function seededPRNG(seed: number): () => number {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function hashSeed(input: string): number {
  let h = 2166136261 >>> 0;
  for (let i = 0; i < input.length; i += 1) {
    h ^= input.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

function buildSeries(metric: ResourceMetric, machineId: string | undefined): number[] {
  const seed = hashSeed(`${metric}:${machineId ?? 'anonymous'}`);
  const rand = seededPRNG(seed);
  const cfg = METRIC_CONFIG[metric];
  const out: number[] = [];
  const base = cfg.min + (cfg.max - cfg.min) * (0.3 + rand() * 0.4);
  let prev = base;
  for (let i = 0; i < POINTS; i += 1) {
    const noise = (rand() - 0.5) * (cfg.max - cfg.min) * 0.35;
    let target: number;
    if (cfg.drift === 'up') {
      target = prev + (cfg.max - prev) * 0.08 + noise * 0.5;
    } else if (cfg.drift === 'flat') {
      target = prev + noise * 0.6;
    } else {
      target = base + noise;
    }
    prev = Math.max(cfg.min, Math.min(cfg.max, target));
    out.push(prev);
  }
  return out;
}

const METRIC_CONFIG: Record<ResourceMetric, MetricConfig> = {
  cpu: {
    icon: <ThunderboltOutlined />,
    color: '#52c41a',
    unit: '%',
    min: 8,
    max: 32,
    drift: 'jitter',
    footer: (current) => (
      <span className={styles.footerText}>
        平均 {Math.max(5, Math.round(current * 0.8))}% · 峰值 {Math.min(100, Math.round(current * 1.4))}%
      </span>
    ),
  },
  memory: {
    icon: <DashboardOutlined />,
    color: '#8b5cf6',
    unit: '%',
    min: 40,
    max: 70,
    drift: 'up',
    footer: (current) => (
      <span className={styles.footerText}>
        已用 {(current / 100 * 16).toFixed(1)} GB / 16 GB
      </span>
    ),
  },
  disk: {
    icon: <DatabaseOutlined />,
    color: '#f59e0b',
    unit: '%',
    min: 60,
    max: 75,
    drift: 'flat',
    footer: (current) => (
      <span className={styles.footerText}>
        已用 {(current / 100 * 512).toFixed(0)} GB / 512 GB
      </span>
    ),
  },
  network: {
    icon: <WifiOutlined />,
    color: '#3b82f6',
    unit: ' KB/s',
    min: 5,
    max: 120,
    drift: 'jitter',
    footer: (current) => {
      const down = Math.round(current * 1.6);
      return (
        <span className={styles.footerText}>
          <span className={styles.arrowUp}>↑ {Math.round(current)} KB/s</span>
          <span className={styles.footerDivider}>·</span>
          <span className={styles.arrowDown}>↓ {down} KB/s</span>
        </span>
      );
    },
  },
};

function buildSparklinePath(values: number[], width: number, height: number, max: number, min: number): string {
  const stepX = width / (values.length - 1);
  const range = max - min || 1;
  const points = values.map((v, i) => {
    const x = i * stepX;
    const y = height - ((v - min) / range) * height;
    return `${x.toFixed(2)},${y.toFixed(2)}`;
  });
  return `M ${points.join(' L ')}`;
}

function buildSparklineAreaPath(values: number[], width: number, height: number, max: number, min: number): string {
  const line = buildSparklinePath(values, width, height, max, min);
  return `${line} L ${width.toFixed(2)},${height.toFixed(2)} L 0,${height.toFixed(2)} Z`;
}

export const ResourceChart: React.FC<ResourceChartProps> = ({
  metric,
  title,
  unit,
  color,
  machineId,
}) => {
  const cfg = METRIC_CONFIG[metric];
  const lineColor = color ?? cfg.color;
  const effectiveUnit = unit ?? cfg.unit;
  const series = useMemo(() => buildSeries(metric, machineId), [metric, machineId]);
  const current = series[series.length - 1] ?? 0;

  const width = 160;
  const height = 36;
  const max = cfg.max;
  const min = cfg.min;
  const linePath = useMemo(
    () => buildSparklinePath(series, width, height, max, min),
    [series, max, min],
  );
  const areaPath = useMemo(
    () => buildSparklineAreaPath(series, width, height, max, min),
    [series, max, min],
  );
  const gradientId = `rc-grad-${metric}`;

  const displayValue = metric === 'network'
    ? current.toFixed(0)
    : current.toFixed(1);

  return (
    <div className={styles.container}>
      <div className={styles.titleRow}>
        <span className={styles.icon} style={{ color: lineColor, background: `${lineColor}1a` }}>
          {cfg.icon}
        </span>
        <span className={styles.title}>{title}</span>
      </div>
      <div className={styles.valueRow}>
        <span className={styles.value}>{displayValue}</span>
        <span className={styles.unit}>{effectiveUnit}</span>
      </div>
      <svg
        className={styles.sparkline}
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={lineColor} stopOpacity="0.32" />
            <stop offset="100%" stopColor={lineColor} stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={areaPath} fill={`url(#${gradientId})`} stroke="none" />
        <path
          d={linePath}
          fill="none"
          stroke={lineColor}
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      <div className={styles.footer}>{cfg.footer(current, series)}</div>
    </div>
  );
};

export default ResourceChart;
