import React, { useState } from 'react';
import { LayoutChangeEvent, Pressable, StyleSheet, View } from 'react-native';
import Svg, { Circle, ClipPath, Defs, G, Line, LinearGradient, Path, Rect, Stop } from 'react-native-svg';

import { useMountProgress } from '@/lib/anim';
import { tapFeedback } from '@/lib/haptics';
import { radius, useTheme } from '@/theme';

import { Row, Txt } from './ui';

function useWidth(initial = 300): [number, (e: LayoutChangeEvent) => void] {
  const [w, setW] = useState(initial);
  return [w, (e) => setW(Math.max(100, Math.floor(e.nativeEvent.layout.width)))];
}

export function ProgressBar({
  value,
  color,
  height = 8,
  marker,
  track,
}: {
  /** 0..1 (values above 1 are clamped visually and shown in the danger color). */
  value: number;
  color?: string;
  height?: number;
  /** Optional target marker 0..1. */
  marker?: number;
  track?: string;
}) {
  const { c } = useTheme();
  const p = useMountProgress(0, 700);
  const v = Math.max(0, Number(value) || 0);
  const over = v > 1;
  return (
    <View
      accessibilityRole="progressbar"
      accessibilityValue={{ min: 0, max: 100, now: Math.round(Math.min(1, v) * 100) }}
      style={[styles.track, { height, backgroundColor: track ?? c.cardAlt, borderRadius: height }]}>
      <View
        style={{
          width: `${Math.min(1, v) * 100 * p}%`,
          height,
          borderRadius: height,
          backgroundColor: over ? c.danger : (color ?? c.primary),
        }}
      />
      {marker !== undefined ? (
        <View
          style={[styles.marker, { left: `${Math.min(1, Math.max(0, marker)) * 100}%`, backgroundColor: c.text }]}
        />
      ) : null}
    </View>
  );
}

export interface LineSeries {
  name: string;
  color: string;
  /** NaN marks "no value" (e.g. an actual series that stops today, a forecast that starts there). */
  values: number[];
  /** Soft gradient fill under the line (use for the primary series only). */
  area?: boolean;
  /** Dashed stroke for reference series (e.g. "contributed", "last month"). */
  dashed?: boolean;
}

/** Monotone cubic (Fritsch–Carlson) path through the points: smooth, never overshoots. */
function smoothPath(pts: { x: number; y: number }[]): string {
  const n = pts.length;
  if (n === 0) return '';
  if (n === 1) return `M${pts[0].x},${pts[0].y}`;
  if (n === 2) return `M${pts[0].x},${pts[0].y} L${pts[1].x},${pts[1].y}`;
  const dx: number[] = [];
  const slope: number[] = [];
  for (let i = 0; i < n - 1; i++) {
    dx.push(pts[i + 1].x - pts[i].x);
    slope.push(dx[i] === 0 ? 0 : (pts[i + 1].y - pts[i].y) / dx[i]);
  }
  const m: number[] = [slope[0]];
  for (let i = 1; i < n - 1; i++) {
    if (slope[i - 1] * slope[i] <= 0) m.push(0);
    else {
      const w1 = 2 * dx[i] + dx[i - 1];
      const w2 = dx[i] + 2 * dx[i - 1];
      m.push((w1 + w2) / (w1 / slope[i - 1] + w2 / slope[i]));
    }
  }
  m.push(slope[n - 2]);
  let d = `M${pts[0].x},${pts[0].y}`;
  for (let i = 0; i < n - 1; i++) {
    const h = dx[i] / 3;
    d += ` C${pts[i].x + h},${pts[i].y + m[i] * h} ${pts[i + 1].x - h},${pts[i + 1].y - m[i + 1] * h} ${pts[i + 1].x},${pts[i + 1].y}`;
  }
  return d;
}

/** Short axis numbers: 1.2k, 3.4M (money axis labels pass their own formatter). */
export function compactNumber(v: number): string {
  const a = Math.abs(v);
  const f = (x: number, s: string) => `${(v < 0 ? '-' : '') + (x >= 100 ? x.toFixed(0) : x.toFixed(1).replace(/\.0$/, ''))}${s}`;
  if (a >= 1e9) return f(a / 1e9, 'B');
  if (a >= 1e6) return f(a / 1e6, 'M');
  if (a >= 1e4) return f(a / 1e3, 'k');
  return a >= 100 ? v.toFixed(0) : String(Number(v.toFixed(2)));
}

function useSvgId(prefix: string): string {
  return prefix + React.useId().replace(/[^a-zA-Z0-9]/g, '');
}

const TICK_W = 64;

/**
 * Line chart for change over time: one or more series on a single y-axis,
 * smooth 2px lines, optional area fill, faint gridlines with axis values, and a
 * crosshair + readout on touch / drag (and hover on web). Draws in left to right.
 */
export function LineChart({
  series,
  labels,
  height = 170,
  formatValue,
  formatAxis,
  formatLabel = (l) => l,
  ticks = 4,
  zeroBased = true,
  summary,
  refLine,
  animateKey,
  curve = 'smooth',
}: {
  series: LineSeries[];
  /** One label per point (dates, months...). */
  labels: string[];
  height?: number;
  formatValue: (v: number) => string;
  /** Y-axis gridline labels (default: compact numbers). */
  formatAxis?: (v: number) => string;
  /** Readout / x-axis text for a label. */
  formatLabel?: (label: string) => string;
  /** Number of x-axis labels. */
  ticks?: number;
  /** Start the y-axis at zero (amounts) or fit the data (prices). */
  zeroBased?: boolean;
  /** Readout text when nothing is selected (single series). */
  summary?: string;
  /** Dashed horizontal reference value (e.g. the average). */
  refLine?: number;
  animateKey?: unknown;
  /** 'linear' for discrete per-period amounts (daily spend), 'smooth' for trends. */
  curve?: 'smooth' | 'linear';
}) {
  const { c } = useTheme();
  const [width, onLayout] = useWidth();
  const [active, setActive] = useState<number | null>(null);
  const p = useMountProgress(animateKey ?? labels.length, 900);
  const clipId = useSvgId('clip');
  const gradId = useSvgId('grad');

  const n = labels.length;
  const all = series
    .flatMap((s) => s.values)
    .concat(refLine !== undefined ? [refLine] : [])
    .filter(Number.isFinite);
  let lo = all.length ? Math.min(...all) : 0;
  let hi = all.length ? Math.max(...all) : 1;
  if (zeroBased) lo = Math.min(0, lo);
  if (hi === lo) hi = lo + (Math.abs(lo) || 1);
  if (!zeroBased) {
    const pad = (hi - lo) * 0.08;
    lo -= pad;
    hi += pad;
  }
  const labelH = 18;
  const top = 10;
  const plotH = height - labelH;
  const base = plotH;
  const inset = 6;
  const X = (i: number) => (n <= 1 ? width / 2 : inset + (i * (width - inset * 2)) / (n - 1));
  const Y = (v: number) => top + (1 - (v - lo) / (hi - lo)) * (plotH - top);
  const fmtAxis = formatAxis ?? compactNumber;

  // Faint gridlines (the zero baseline is drawn separately for amount charts).
  const gridVals = zeroBased ? [hi, (hi + lo) / 2] : [hi - (hi - lo) * 0.08, (hi + lo) / 2, lo + (hi - lo) * 0.08];

  const tickIdx: number[] = [];
  if (n > 0) {
    const k = Math.max(2, Math.min(ticks, n));
    for (let t = 0; t < k; t++) tickIdx.push(Math.round((t * (n - 1)) / (k - 1)));
  }

  const pick = (x: number) => {
    if (n === 0) return;
    const i = n <= 1 ? 0 : Math.round(((x - inset) / (width - inset * 2)) * (n - 1));
    setActive(Math.max(0, Math.min(n - 1, i)));
  };

  const single = series.length === 1;
  const sel = active;

  return (
    <View onLayout={onLayout} style={styles.gap8}>
      {single ? (
        <Row style={styles.readout}>
          <Txt variant="small" style={styles.flex1} numberOfLines={1}>
            {sel !== null ? formatLabel(labels[sel]) : (summary ?? 'Touch the chart for details')}
          </Txt>
          {sel !== null ? (
            <Txt variant="small" weight="700" color={c.text}>
              {Number.isFinite(series[0].values[sel]) ? formatValue(series[0].values[sel]) : '—'}
            </Txt>
          ) : null}
        </Row>
      ) : (
        <Row gap={14} style={styles.wrap}>
          {series.map((s) => (
            <Row key={s.name} gap={6}>
              <View style={[styles.legendLine, { backgroundColor: s.color, opacity: s.dashed ? 0.7 : 1 }]} />
              <Txt variant="small" color={c.text}>
                {s.name}
                {sel !== null && Number.isFinite(s.values[sel]) ? `: ${formatValue(s.values[sel])}` : ''}
              </Txt>
            </Row>
          ))}
          {sel !== null ? <Txt variant="small">· {formatLabel(labels[sel])}</Txt> : null}
        </Row>
      )}
      <View>
        <Svg width={width} height={height}>
          <Defs>
            <LinearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
              <Stop offset="0" stopColor={series[0]?.color ?? c.primary} stopOpacity={0.26} />
              <Stop offset="1" stopColor={series[0]?.color ?? c.primary} stopOpacity={0} />
            </LinearGradient>
            <ClipPath id={clipId}>
              <Rect x={0} y={0} width={Math.max(1, width * p)} height={height} />
            </ClipPath>
          </Defs>
          {gridVals.map((v, i) => (
            <Line key={i} x1={0} x2={width} y1={Y(v)} y2={Y(v)} stroke={c.border} strokeWidth={1} opacity={0.7} />
          ))}
          <Line x1={0} x2={width} y1={base} y2={base} stroke={c.border} strokeWidth={1} />
          {refLine !== undefined ? (
            <Line x1={0} x2={width} y1={Y(refLine)} y2={Y(refLine)} stroke={c.textMuted} strokeWidth={1} strokeDasharray="4 4" opacity={0.7} />
          ) : null}
          <G clipPath={`url(#${clipId})`}>
            {series.map((s, si) => {
              const pts = s.values
                .slice(0, n)
                .map((v, i) => ({ v, x: X(i) }))
                .filter((q) => Number.isFinite(q.v))
                .map((q) => ({ x: q.x, y: Y(q.v) }));
              const d = curve === 'smooth' ? smoothPath(pts) : pts.map((q, i) => `${i ? 'L' : 'M'}${q.x},${q.y}`).join(' ');
              if (!d) return null;
              return (
                <React.Fragment key={s.name}>
                  {s.area && pts.length > 1 ? (
                    <Path d={`${d} L${pts[pts.length - 1].x},${base} L${pts[0].x},${base} Z`} fill={`url(#${gradId})`} />
                  ) : null}
                  <Path
                    d={d}
                    stroke={s.color}
                    strokeWidth={2}
                    fill="none"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeDasharray={s.dashed ? '5 4' : undefined}
                    opacity={s.dashed ? 0.8 : 1}
                  />
                  {pts.length === 1 || (si === 0 && sel === null && p >= 1) ? (
                    <Circle cx={pts[pts.length - 1].x} cy={pts[pts.length - 1].y} r={4} fill={s.color} stroke={c.card} strokeWidth={2} />
                  ) : null}
                </React.Fragment>
              );
            })}
          </G>
          {sel !== null ? (
            <>
              <Line x1={X(sel)} x2={X(sel)} y1={top - 4} y2={base} stroke={c.textMuted} strokeWidth={1} opacity={0.6} />
              {series.map((s) =>
                Number.isFinite(s.values[sel]) ? (
                  <Circle key={s.name} cx={X(sel)} cy={Y(s.values[sel])} r={5} fill={s.color} stroke={c.card} strokeWidth={2} />
                ) : null,
              )}
            </>
          ) : null}
        </Svg>
        {/* Gridline values and x-axis labels as plain Text (SVG text gets responder props on web). */}
        <View style={StyleSheet.absoluteFill} pointerEvents="none">
          {gridVals.map((v, i) => (
            <Txt key={i} variant="small" color={c.textMuted} style={[styles.yLabel, { top: Y(v) - 15 }]}>
              {fmtAxis(v)}
            </Txt>
          ))}
          {tickIdx.map((i, k) => {
            const x = X(i);
            const first = k === 0;
            const last = k === tickIdx.length - 1;
            return (
              <Txt
                key={`${i}-${k}`}
                variant="small"
                color={c.textMuted}
                numberOfLines={1}
                style={[
                  styles.axisLabel,
                  { left: first ? 0 : last ? width - TICK_W : x - TICK_W / 2, textAlign: first ? 'left' : last ? 'right' : 'center' },
                ]}>
                {formatLabel(labels[i])}
              </Txt>
            );
          })}
        </View>
        <View
          style={[styles.overlay, { top: 0, height: plotH }]}
          accessibilityRole="adjustable"
          accessibilityLabel="Chart, touch to inspect values"
          onStartShouldSetResponder={() => true}
          onResponderTerminationRequest={() => true}
          onResponderGrant={(e) => {
            tapFeedback();
            pick(e.nativeEvent.locationX);
          }}
          onResponderMove={(e) => pick(e.nativeEvent.locationX)}
          onPointerMove={(e) => {
            if (e.nativeEvent.pointerType === 'mouse') pick(e.nativeEvent.offsetX);
          }}
          onPointerLeave={(e) => {
            if (e.nativeEvent.pointerType === 'mouse') setActive(null);
          }}
        />
      </View>
    </View>
  );
}

/** Tiny trend line for lists (no axes, no interaction). Color by direction. */
export function Sparkline({
  values,
  width = 72,
  height = 28,
  color,
}: {
  values: number[];
  width?: number;
  height?: number;
  color?: string;
}) {
  const { c } = useTheme();
  const gradId = useSvgId('spark');
  if (values.length < 2) return <View style={{ width, height }} />;
  const lo = Math.min(...values);
  const hi = Math.max(...values);
  const span = hi - lo || 1;
  const col = color ?? (values[values.length - 1] >= values[0] ? c.positive : c.negative);
  const pts = values.map((v, i) => ({ x: 2 + (i * (width - 4)) / (values.length - 1), y: 3 + (1 - (v - lo) / span) * (height - 6) }));
  const d = smoothPath(pts);
  return (
    <Svg width={width} height={height}>
      <Defs>
        <LinearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
          <Stop offset="0" stopColor={col} stopOpacity={0.22} />
          <Stop offset="1" stopColor={col} stopOpacity={0} />
        </LinearGradient>
      </Defs>
      <Path d={`${d} L${pts[pts.length - 1].x},${height} L${pts[0].x},${height} Z`} fill={`url(#${gradId})`} />
      <Path d={d} stroke={col} strokeWidth={1.75} fill="none" strokeLinecap="round" strokeLinejoin="round" />
    </Svg>
  );
}

/** Single-value ring (e.g. savings rate or goal progress) with a centered label. */
export function Ring({
  value,
  size = 84,
  stroke = 9,
  color,
  track,
  label,
  labelColor,
  sublabel,
}: {
  value: number;
  size?: number;
  stroke?: number;
  color?: string;
  track?: string;
  label: string;
  labelColor?: string;
  sublabel?: string;
}) {
  const { c } = useTheme();
  const p = useMountProgress(0, 900);
  const r = (size - stroke) / 2;
  const circ = 2 * Math.PI * r;
  const v = Math.min(1, Math.max(0, Number(value) || 0)) * p;
  return (
    <View
      accessibilityLabel={label}
      style={{ width: size, height: size, alignItems: 'center', justifyContent: 'center' }}>
      <Svg width={size} height={size} style={StyleSheet.absoluteFill}>
        <Circle cx={size / 2} cy={size / 2} r={r} stroke={track ?? c.cardAlt} strokeWidth={stroke} fill="none" />
        {v > 0.001 ? (
          <Circle
            cx={size / 2}
            cy={size / 2}
            r={r}
            stroke={color ?? c.primary}
            strokeWidth={stroke}
            fill="none"
            strokeDasharray={`${circ * v} ${circ}`}
            strokeLinecap="round"
            transform={`rotate(-90 ${size / 2} ${size / 2})`}
          />
        ) : null}
      </Svg>
      <Txt variant="small" weight="800" color={labelColor ?? c.text}>
        {label}
      </Txt>
      {sublabel ? (
        <Txt variant="small" color={labelColor ?? c.textMuted} style={styles.sublabel}>
          {sublabel}
        </Txt>
      ) : null}
    </View>
  );
}

export interface DonutSlice {
  key: string;
  label: string;
  value: number;
  color: string;
}

function arcPath(cx: number, cy: number, r: number, a0: number, a1: number): string {
  const large = a1 - a0 > Math.PI ? 1 : 0;
  const x0 = cx + r * Math.cos(a0);
  const y0 = cy + r * Math.sin(a0);
  const x1 = cx + r * Math.cos(a1);
  const y1 = cy + r * Math.sin(a1);
  return `M${x0},${y0} A${r},${r} 0 ${large} 1 ${x1},${y1}`;
}

/**
 * Donut chart with a centered total; the caller's legend rows drive `active`
 * (focused slice).
 */
export function Donut({
  slices,
  size = 176,
  stroke = 22,
  centerTop,
  centerValue,
  active,
}: {
  slices: DonutSlice[];
  size?: number;
  stroke?: number;
  centerTop: string;
  centerValue: string;
  active?: string | null;
}) {
  const { c } = useTheme();
  const p = useMountProgress(slices.length, 900);
  const total = slices.reduce((s, x) => s + Math.max(0, x.value), 0);
  const r = (size - stroke) / 2 - 3;
  const cx = size / 2;
  const cy = size / 2;
  // 2px surface gap between segments, expressed as an angle.
  const gapA = slices.length > 1 ? 2.5 / r : 0;
  const sweepTotal = Math.PI * 2 * p;
  const sweeps = slices.map((s) => (total > 0 ? (Math.max(0, s.value) / total) * sweepTotal : 0));
  const arcs = slices.map((s, i) => {
    const sweep = sweeps[i];
    const start = -Math.PI / 2 + sweeps.slice(0, i).reduce((x, y) => x + y, 0);
    const a0 = start + gapA / 2;
    const a1 = start + Math.max(sweep - gapA / 2, gapA / 2 + 0.0001);
    return { s, a0, a1, sweep };
  });
  return (
    <View style={{ width: size, height: size, alignItems: 'center', justifyContent: 'center' }}>
      <Svg width={size} height={size} style={StyleSheet.absoluteFill}>
        <Circle cx={cx} cy={cy} r={r} stroke={c.cardAlt} strokeWidth={stroke} fill="none" />
        {arcs.map(({ s, a0, a1, sweep }) =>
          sweep > gapA ? (
            <Path
              key={s.key}
              d={sweep >= Math.PI * 2 - 0.001 ? arcPath(cx, cy, r, a0, a0 + Math.PI * 1.9999) : arcPath(cx, cy, r, a0, a1)}
              stroke={s.color}
              strokeWidth={active === s.key ? stroke + 6 : stroke}
              strokeLinecap="butt"
              fill="none"
              opacity={active && active !== s.key ? 0.3 : 1}
            />
          ) : null,
        )}
      </Svg>
      <View pointerEvents="none" style={styles.donutCenter}>
        <Txt variant="small" numberOfLines={1}>
          {centerTop}
        </Txt>
        <Txt variant="h2" weight="800" numberOfLines={1} adjustsFontSizeToFit style={{ maxWidth: size - stroke * 2 - 16 }}>
          {centerValue}
        </Txt>
      </View>
    </View>
  );
}

/** Pressable legend row for the donut (identity = color dot + text, never color alone). */
export function LegendRow({
  color,
  label,
  value,
  share,
  icon,
  active,
  onPress,
}: {
  color: string;
  label: string;
  value: string;
  share: string;
  icon?: React.ReactNode;
  active?: boolean;
  onPress?: () => void;
}) {
  const { c } = useTheme();
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      style={({ pressed }) => [
        styles.legendRow,
        (active || pressed) && { backgroundColor: c.cardAlt },
      ]}>
      {icon ?? <View style={[styles.legendDot, { backgroundColor: color }]} />}
      <Txt weight="600" style={styles.flex1} numberOfLines={1}>
        {label}
      </Txt>
      <Txt variant="small" style={styles.share}>
        {share}
      </Txt>
      <Txt weight="700" style={styles.legendValue} numberOfLines={1}>
        {value}
      </Txt>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  track: { width: '100%', overflow: 'hidden', position: 'relative' },
  marker: { position: 'absolute', top: -2, bottom: -2, width: 3, borderRadius: radius.sm },
  readout: { minHeight: 17 },
  legendLine: { width: 14, height: 3, borderRadius: 2 },
  yLabel: { position: 'absolute', left: 0, fontSize: 10, lineHeight: 14 },
  legendDot: { width: 10, height: 10, borderRadius: 5 },
  gap8: { gap: 8 },
  wrap: { flexWrap: 'wrap' },
  flex1: { flex: 1 },
  overlay: { position: 'absolute', left: 0, right: 0, flexDirection: 'row' },
  axisLabel: { position: 'absolute', bottom: 0, width: TICK_W, fontSize: 10, lineHeight: 14 },
  sublabel: { fontSize: 10, lineHeight: 12 },
  donutCenter: { alignItems: 'center', justifyContent: 'center' },
  legendRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
    minHeight: 44,
    paddingHorizontal: 8,
    borderRadius: radius.sm,
  },
  share: { width: 40, textAlign: 'right' },
  legendValue: { minWidth: 76, textAlign: 'right', fontVariant: ['tabular-nums'] },
});
