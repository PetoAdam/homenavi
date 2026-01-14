import React, { useId, useMemo, useRef, useState } from 'react';
import './HistoryChart.css';

function buildMonotoneSpline(points) {
  const pts = Array.isArray(points) ? points : [];
  if (pts.length < 2) return { dLine: '', dArea: '' };

  // Ensure strictly increasing x to avoid divide-by-zero.
  const p = pts
    .map(v => ({ x: Number(v.x), y: Number(v.y) }))
    .filter(v => Number.isFinite(v.x) && Number.isFinite(v.y))
    .sort((a, b) => a.x - b.x);

  if (p.length < 2) return { dLine: '', dArea: '' };

  const n = p.length;
  const dx = new Array(n - 1);
  const dy = new Array(n - 1);
  const m = new Array(n - 1);

  for (let i = 0; i < n - 1; i += 1) {
    dx[i] = p[i + 1].x - p[i].x;
    dy[i] = p[i + 1].y - p[i].y;
    m[i] = dx[i] !== 0 ? dy[i] / dx[i] : 0;
  }

  // Tangents (Fritsch-Carlson monotone cubic)
  const t = new Array(n);
  t[0] = m[0];
  t[n - 1] = m[n - 2];
  for (let i = 1; i < n - 1; i += 1) {
    if (m[i - 1] === 0 || m[i] === 0 || (m[i - 1] > 0) !== (m[i] > 0)) {
      t[i] = 0;
    } else {
      const w1 = 2 * dx[i] + dx[i - 1];
      const w2 = dx[i] + 2 * dx[i - 1];
      t[i] = (w1 + w2) / ((w1 / m[i - 1]) + (w2 / m[i]));
    }
  }

  let dLine = `M ${p[0].x.toFixed(2)} ${p[0].y.toFixed(2)}`;
  for (let i = 0; i < n - 1; i += 1) {
    const x0 = p[i].x;
    const y0 = p[i].y;
    const x1 = p[i + 1].x;
    const y1 = p[i + 1].y;
    const h = x1 - x0;
    const c1x = x0 + h / 3;
    const c1y = y0 + (t[i] * h) / 3;
    const c2x = x1 - h / 3;
    const c2y = y1 - (t[i + 1] * h) / 3;
    dLine += ` C ${c1x.toFixed(2)} ${c1y.toFixed(2)} ${c2x.toFixed(2)} ${c2y.toFixed(2)} ${x1.toFixed(2)} ${y1.toFixed(2)}`;
  }

  return { dLine, dArea: '' };
}

function isBooleanLikeString(value) {
  if (typeof value !== 'string') return false;
  const v = value.trim().toLowerCase();
  return [
    'true', 'false',
    'on', 'off',
    'yes', 'no',
    'enabled', 'disabled',
    'active', 'inactive',
    'open', 'closed',
    'present', 'absent',
    'detected', 'clear',
  ].includes(v);
}

function clampNumber(value, fallback = 0) {
  if (typeof value === 'boolean') return value ? 1 : 0;
  if (typeof value === 'number') return Number.isFinite(value) ? value : fallback;
  if (typeof value === 'string') {
    const v = value.trim().toLowerCase();
    if (['true', 'on', 'yes', 'enabled', 'active', 'detected', 'present', 'open'].includes(v)) return 1;
    if (['false', 'off', 'no', 'disabled', 'inactive', 'clear', 'absent', 'closed'].includes(v)) return 0;
    const n = Number(value);
    return Number.isFinite(n) ? n : fallback;
  }
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

function formatValue(value) {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'boolean') return value ? 'On' : 'Off';
  if (typeof value === 'string') {
    const v = value.trim().toLowerCase();
    if (['true', 'on', 'yes', 'enabled', 'active', 'detected', 'present', 'open'].includes(v)) return 'On';
    if (['false', 'off', 'no', 'disabled', 'inactive', 'clear', 'absent', 'closed'].includes(v)) return 'Off';
  }
  const n = Number(value);
  if (!Number.isFinite(n)) return String(value);
  if (Math.abs(n) >= 1000) return n.toFixed(0);
  if (Math.abs(n) >= 10) return n.toFixed(1);
  return n.toFixed(2);
}

function formatAxisNumber(value, { booleanMode } = {}) {
  if (booleanMode) {
    return value >= 0.5 ? 'On' : 'Off';
  }
  if (!Number.isFinite(value)) return '—';
  const abs = Math.abs(value);
  if (abs >= 1000) return value.toFixed(0);
  if (abs >= 10) return value.toFixed(1);
  return value.toFixed(2);
}

function formatTimeLabel(ts) {
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '';
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay) {
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleString(undefined, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function formatTooltipTime(ts) {
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

export default function HistoryChart({ title, series, height = 180, unit = '' }) {
  const svgRef = useRef(null);
  const [hover, setHover] = useState(null);
  const gradientId = useId();

  const model = useMemo(() => {
    const points = Array.isArray(series) ? series : [];
    if (points.length < 2) {
      return null;
    }

    const xs = points.map(p => new Date(p.ts).getTime()).filter(Number.isFinite);
    const ys = points.map(p => clampNumber(p.value, NaN)).filter(Number.isFinite);
    if (xs.length < 2 || ys.length < 2) {
      return null;
    }

    const minX = Math.min(...xs);
    const maxX = Math.max(...xs);
    const minY = Math.min(...ys);
    const maxY = Math.max(...ys);

    // Detect boolean mode:
    // - any actual boolean values
    // - boolean-like strings
    // - or series that only contains ~0/1 values (avoid treating continuous 0..1 metrics as booleans)
    const rawValues = points.map(p => p?.value);
    const hasBooleanRaw = rawValues.some(v => typeof v === 'boolean');
    const hasBooleanString = rawValues.some(isBooleanLikeString);
    const epsilon = 1e-6;
    const unique01 = new Set();
    let onlyZeroOne = true;
    for (const y of ys) {
      if (!Number.isFinite(y) || y < -epsilon || y > 1 + epsilon) {
        onlyZeroOne = false;
        break;
      }
      const rounded = y >= 0.5 ? 1 : 0;
      if (Math.abs(y - rounded) > 0.08) {
        // if values cluster across the range, treat as numeric (e.g. 0.0..1.0).
        onlyZeroOne = false;
        break;
      }
      unique01.add(rounded);
      if (unique01.size > 2) {
        onlyZeroOne = false;
        break;
      }
    }

    const booleanMode = hasBooleanRaw || hasBooleanString || (onlyZeroOne && unique01.size <= 2);

    const xSpan = Math.max(1, maxX - minX);
    const ySpan = Math.max(1e-9, maxY - minY);

    const paddingLeft = 54;
    const paddingRight = 14;
    const paddingTop = 14;
    const paddingBottom = 34;
    const width = 600; // viewBox width
    const innerWidth = width - paddingLeft - paddingRight;
    const innerHeight = height - paddingTop - paddingBottom;

    // Keep boolean charts visually separated from the edges.
    const yMin = booleanMode ? -0.15 : (minY - (ySpan * 0.08));
    const yMax = booleanMode ? 1.15 : (maxY + (ySpan * 0.08));

    const xFor = x => paddingLeft + ((x - minX) / xSpan) * innerWidth;
    const yFor = y => paddingTop + (1 - ((y - yMin) / (yMax - yMin))) * innerHeight;

    const svgPoints = points
      .map(p => {
        const ts = new Date(p.ts).getTime();
        const yValue = clampNumber(p.value, NaN);
        return {
          x: xFor(ts),
          y: yFor(yValue),
          ts,
          yValue,
          raw: p,
        };
      })
      .filter(p => Number.isFinite(p.x) && Number.isFinite(p.y) && Number.isFinite(p.ts) && Number.isFinite(p.yValue));

    if (svgPoints.length < 2) {
      return null;
    }

    const yBase = height - paddingBottom;

    const drawPoints = (!booleanMode && svgPoints.length >= 3)
      ? svgPoints.map((p, idx) => {
        if (idx === 0 || idx === svgPoints.length - 1) return p;
        const prev = svgPoints[idx - 1];
        const next = svgPoints[idx + 1];
        // Light smoothing in SVG space: reduces jaggedness without changing tooltip/raw values.
        const y = ((prev.y + (2 * p.y) + next.y) / 4);
        return { ...p, y };
      })
      : svgPoints;

    const spline = booleanMode ? null : buildMonotoneSpline(drawPoints);

    const dLine = booleanMode
      ? svgPoints
        .map((p, idx) => {
          if (idx === 0) {
            return `M ${p.x.toFixed(2)} ${p.y.toFixed(2)}`;
          }
          const prev = svgPoints[idx - 1];
          // Step line: hold previous value until the current timestamp, then jump.
          return `L ${p.x.toFixed(2)} ${prev.y.toFixed(2)} L ${p.x.toFixed(2)} ${p.y.toFixed(2)}`;
        })
        .join(' ')
      : spline.dLine;

    const dArea = booleanMode ? '' : (() => {
      if (!drawPoints.length) return '';
      const first = drawPoints[0];
      const last = drawPoints[drawPoints.length - 1];
      const line = spline?.dLine || '';
      if (!line) return '';
      const withoutMove = line.replace(/^M [^ ]+ [^ ]+/, '');
      return `M ${first.x.toFixed(2)} ${yBase.toFixed(2)} L ${first.x.toFixed(2)} ${first.y.toFixed(2)} ${withoutMove} L ${last.x.toFixed(2)} ${yBase.toFixed(2)} Z`;
    })();

    const latest = svgPoints[svgPoints.length - 1];
    const latestValue = latest?.raw?.value;

    return {
      width,
      height,
      dLine,
      dArea,
      latestValue,
      minY: yMin,
      maxY: yMax,
      minX,
      maxX,
      paddingLeft,
      paddingRight,
      paddingTop,
      paddingBottom,
      yBase,
      booleanMode,
      svgPoints,
    };
  }, [series, height]);

  const onMouseMove = (event) => {
    if (!model || !svgRef.current) return;
    const rect = svgRef.current.getBoundingClientRect();
    if (!rect.width || !rect.height) return;

    const px = event.clientX - rect.left;
    const py = event.clientY - rect.top;
    if (px < 0 || py < 0 || px > rect.width || py > rect.height) {
      setHover(null);
      return;
    }

    const x = (px / rect.width) * model.width;

    // Find nearest point by x.
    const points = model.svgPoints;
    if (!points || points.length === 0) return;
    let lo = 0;
    let hi = points.length - 1;
    while (lo < hi) {
      const mid = Math.floor((lo + hi) / 2);
      if (points[mid].x < x) lo = mid + 1; else hi = mid;
    }
    const idx = lo;
    const left = points[Math.max(0, idx - 1)];
    const right = points[Math.min(points.length - 1, idx)];
    const nearest = (left && right)
      ? (Math.abs(left.x - x) <= Math.abs(right.x - x) ? left : right)
      : (left || right);

    if (!nearest) return;
    const valueLabel = formatValue(nearest.raw?.value);
    const suffix = unit ? ` ${unit}` : '';
    setHover({
      x: nearest.x,
      y: nearest.y,
      ts: nearest.ts,
      valueLabel: `${valueLabel}${suffix}`,
    });
  };

  const onMouseLeave = () => setHover(null);

  const yTicks = useMemo(() => {
    if (!model) return [];
    if (model.booleanMode) {
      return [0, 0.5, 1].map(v => ({
        value: v,
        y: model.paddingTop + (model.height - model.paddingTop - model.paddingBottom) * (1 - ((v - model.minY) / (model.maxY - model.minY))),
        label: formatAxisNumber(v, { booleanMode: true }),
      }));
    }
    const ticks = 5;
    const res = [];
    for (let i = 0; i < ticks; i += 1) {
      const frac = i / (ticks - 1);
      const v = model.minY + (model.maxY - model.minY) * (1 - frac);
      const y = model.paddingTop + (model.height - model.paddingTop - model.paddingBottom) * frac;
      res.push({ value: v, y, label: formatAxisNumber(v, { booleanMode: false }) });
    }
    return res;
  }, [model]);

  const xTicks = useMemo(() => {
    if (!model) return [];
    const ticks = 4;
    const res = [];
    for (let i = 0; i < ticks; i += 1) {
      const frac = i / (ticks - 1);
      const x = model.paddingLeft + (model.width - model.paddingLeft - model.paddingRight) * frac;
      const ts = model.minX + (model.maxX - model.minX) * frac;
      res.push({ x, ts, label: formatTimeLabel(ts) });
    }
    return res;
  }, [model]);

  if (!model) {
    return (
      <div className="history-chart-empty">
        <div className="history-chart-title-row">
          <div className="history-chart-title">{title}</div>
          <div className="history-chart-latest">—</div>
        </div>
        <div className="history-chart-placeholder">Not enough data to chart.</div>
      </div>
    );
  }

  return (
    <div className="history-chart">
      <div className="history-chart-title-row">
        <div className="history-chart-title">{title}</div>
        <div className="history-chart-latest">{formatValue(model.latestValue)}{unit ? ` ${unit}` : ''}</div>
      </div>
      <svg
        className="history-chart-svg"
        viewBox={`0 0 ${model.width} ${model.height}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={title}
        ref={svgRef}
        style={{ '--history-chart-height': `${height}px` }}
        onMouseMove={onMouseMove}
        onMouseLeave={onMouseLeave}
      >
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-primary)" stopOpacity="0.28" />
            <stop offset="65%" stopColor="var(--color-primary)" stopOpacity="0.08" />
            <stop offset="100%" stopColor="var(--color-primary)" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path
          className="history-chart-grid"
          d={`M ${model.paddingLeft} ${model.height - model.paddingBottom} L ${model.width - model.paddingRight} ${model.height - model.paddingBottom}`}
        />

        {/* grid + axis ticks */}
        {yTicks.map((t, idx) => (
          <React.Fragment key={`y-${idx}`}>
            <path
              className="history-chart-grid history-chart-grid-soft"
              d={`M ${model.paddingLeft} ${t.y.toFixed(2)} L ${model.width - model.paddingRight} ${t.y.toFixed(2)}`}
            />
            <text
              className="history-chart-axis"
              x={8}
              y={t.y}
              textAnchor="start"
              dominantBaseline="middle"
            >
              {t.label}
            </text>
          </React.Fragment>
        ))}

        {xTicks.map((t, idx) => (
          <React.Fragment key={`x-${idx}`}>
            <path
              className="history-chart-grid history-chart-grid-soft"
              d={`M ${t.x.toFixed(2)} ${model.paddingTop} L ${t.x.toFixed(2)} ${model.height - model.paddingBottom}`}
            />
            <text
              className="history-chart-axis history-chart-axis-x"
              x={t.x}
              y={model.height - 10}
              textAnchor={idx === 0 ? 'start' : (idx === xTicks.length - 1 ? 'end' : 'middle')}
              dominantBaseline="ideographic"
            >
              {t.label}
            </text>
          </React.Fragment>
        ))}

        {hover ? (
          <>
            <path
              className="history-chart-crosshair"
              d={`M ${hover.x.toFixed(2)} ${model.paddingTop} L ${hover.x.toFixed(2)} ${(model.height - model.paddingBottom).toFixed(2)}`}
            />
            <path
              className="history-chart-crosshair"
              d={`M ${model.paddingLeft} ${hover.y.toFixed(2)} L ${(model.width - model.paddingRight).toFixed(2)} ${hover.y.toFixed(2)}`}
            />
            <circle className="history-chart-dot" cx={hover.x} cy={hover.y} r={3.2} />
          </>
        ) : null}

        {model.dArea ? (
          <path
            className="history-chart-area"
            d={model.dArea}
            fill={`url(#${gradientId})`}
          />
        ) : null}
        <path className="history-chart-line" d={model.dLine} />
      </svg>

      {hover ? (
        <div className="history-chart-tooltip" role="status">
          <div className="history-chart-tooltip-value">{hover.valueLabel}</div>
          <div className="history-chart-tooltip-time">{formatTooltipTime(hover.ts)}</div>
        </div>
      ) : null}
    </div>
  );
}
