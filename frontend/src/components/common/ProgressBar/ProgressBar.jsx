import React, { useMemo } from 'react';
import './ProgressBar.css';

function clampPercent(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Math.max(0, Math.min(100, n));
}

export default function ProgressBar({ progress, valuePercent, className = '' }) {
  const percent = useMemo(() => {
    const fromProgress = clampPercent(progress);
    if (fromProgress !== null) return fromProgress;
    const fromValuePercent = clampPercent(valuePercent);
    if (fromValuePercent !== null) return fromValuePercent;
    return null;
  }, [progress, valuePercent]);

  const isIndeterminate = percent === null;

  return (
    <div
      className={`progress-bar ${isIndeterminate ? 'progress-bar--indeterminate' : ''} ${className}`}
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={isIndeterminate ? undefined : percent}
    >
      <div className="progress-bar__track">
        <div
          className="progress-bar__fill"
          style={isIndeterminate ? undefined : { width: `${percent}%` }}
        />
      </div>
    </div>
  );
}
