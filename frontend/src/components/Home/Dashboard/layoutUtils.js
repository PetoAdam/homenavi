const DEFAULT_BREAKPOINT_ORDER = ['xl', 'lg', 'md', 'sm', 'xxs'];

function uniqueBreakpointOrder(preferredBreakpoint, layoutsByBp) {
  const extra = Object.keys(layoutsByBp || {});
  return [preferredBreakpoint, ...DEFAULT_BREAKPOINT_ORDER, ...extra].filter(
    (bp, index, arr) => bp && arr.indexOf(bp) === index,
  );
}

export function pickSourceBreakpoint(layoutsByBp) {
  const candidates = ['xl', 'lg', 'md', 'sm', 'xxs'];
  for (const bp of candidates) {
    const items = layoutsByBp?.[bp];
    if (Array.isArray(items) && items.length > 0) return bp;
  }
  return 'lg';
}

export function normalizeLayoutHeights(layoutsByBp, { preferredBreakpoint } = {}) {
  const source = layoutsByBp && typeof layoutsByBp === 'object' ? layoutsByBp : {};
  const order = uniqueBreakpointOrder(preferredBreakpoint, source);
  const heightById = new Map();

  order.forEach((bp) => {
    const items = Array.isArray(source[bp]) ? source[bp] : [];
    items.forEach((item) => {
      if (!item?.i || heightById.has(item.i) || !Number.isFinite(item.h)) return;
      heightById.set(item.i, item.h);
    });
  });

  const next = {};
  Object.entries(source).forEach(([bp, items]) => {
    next[bp] = (Array.isArray(items) ? items : []).map((item) => {
      const canonicalHeight = heightById.get(item?.i);
      if (!Number.isFinite(canonicalHeight)) return item;
      const minH = Number.isFinite(item?.minH) ? item.minH : 1;
      return {
        ...item,
        h: Math.max(minH, canonicalHeight),
      };
    });
  });

  return next;
}