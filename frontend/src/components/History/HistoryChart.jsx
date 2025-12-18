import React, { useMemo, useRef, useState } from 'react';
import './HistoryChart.css';

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

    const d = booleanMode
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
      : svgPoints
        .map((p, idx) => `${idx === 0 ? 'M' : 'L'} ${p.x.toFixed(2)} ${p.y.toFixed(2)}`)
        .join(' ');

    const latest = svgPoints[svgPoints.length - 1];
    const latestValue = latest?.raw?.value;

    return {
      width,
      height,
      d,
      latestValue,
      minY: yMin,
      maxY: yMax,
      minX,
      maxX,
      paddingLeft,
      paddingRight,
      paddingTop,
      paddingBottom,
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
        <path
          className="history-chart-grid"
          d={`M ${model.paddingLeft} ${model.height - model.paddingBottom} L ${model.width - model.paddingRight} ${model.height - model.paddingBottom}`}
        />

        {/* grid lines */}
        {[0.25, 0.5, 0.75].map(frac => {
          const y = model.paddingTop + (model.height - model.paddingTop - model.paddingBottom) * frac;
          return (
            <path
              key={`h-${frac}`}
              className="history-chart-grid history-chart-grid-soft"
              d={`M ${model.paddingLeft} ${y.toFixed(2)} L ${model.width - model.paddingRight} ${y.toFixed(2)}`}
            />
          );
        })}
        {[0.5].map(frac => {
          const x = model.paddingLeft + (model.width - model.paddingLeft - model.paddingRight) * frac;
          return (
            <path
              key={`v-${frac}`}
              className="history-chart-grid history-chart-grid-soft"
              d={`M ${x.toFixed(2)} ${model.paddingTop} L ${x.toFixed(2)} ${model.height - model.paddingBottom}`}
            />
          );
        })}

        <text
          className="history-chart-axis"
          x={8}
          y={model.paddingTop + 8}
          textAnchor="start"
          dominantBaseline="hanging"
        >
          {formatAxisNumber(model.maxY, { booleanMode: model.booleanMode })}
        </text>
        <text
          className="history-chart-axis"
          x={8}
          y={model.height - model.paddingBottom - 2}
          textAnchor="start"
          dominantBaseline="ideographic"
        >
          {formatAxisNumber(model.minY, { booleanMode: model.booleanMode })}
        </text>
        <text
          className="history-chart-axis history-chart-axis-x"
          x={model.paddingLeft}
          y={model.height - 10}
          textAnchor="start"
          dominantBaseline="ideographic"
        >
          {formatTimeLabel(model.minX)}
        </text>
        <text
          className="history-chart-axis history-chart-axis-x"
          x={model.width - model.paddingRight}
          y={model.height - 10}
          textAnchor="end"
          dominantBaseline="ideographic"
        >
          {formatTimeLabel(model.maxX)}
        </text>

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

        <path className="history-chart-line" d={model.d} />
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
