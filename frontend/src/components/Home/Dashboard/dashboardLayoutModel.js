export const DASHBOARD_COLUMN_MODES = ['4', '3', '2', '1'];

export const DASHBOARD_VIEWPORT_THRESHOLDS = {
  '4': 1600,
  '3': 1200,
  '2': 760,
  '1': 0,
};

const LEGACY_BREAKPOINTS = ['xl', 'lg', 'md', 'sm', 'xs', 'xxs'];

function safeLayoutArray(value) {
  return Array.isArray(value) ? value : [];
}

export function getColumnCount(mode) {
  const n = Number.parseInt(String(mode), 10);
  return Number.isFinite(n) && n > 0 ? n : 1;
}

export function getDashboardColumnMode(viewportWidth) {
  const width = Number(viewportWidth);
  if (!Number.isFinite(width)) return '3';
  for (const mode of DASHBOARD_COLUMN_MODES) {
    if (width >= DASHBOARD_VIEWPORT_THRESHOLDS[mode]) return mode;
  }
  return '1';
}

export function createEmptyLayoutsByCols() {
  return {
    '4': [],
    '3': [],
    '2': [],
    '1': [],
  };
}

export function getLayoutMaxRight(layout) {
  if (!Array.isArray(layout) || layout.length === 0) return 0;
  let maxRight = 0;
  for (const item of layout) {
    const x = Number.isFinite(item?.x) ? item.x : 0;
    const w = Number.isFinite(item?.w) ? item.w : 1;
    maxRight = Math.max(maxRight, x + w);
  }
  return maxRight;
}

function clampItemWidth(width, cols) {
  const n = Number(width);
  if (!Number.isFinite(n)) return 1;
  return Math.max(1, Math.min(cols, Math.round(n)));
}

function normalizeLayoutItem(item, cols) {
  return {
    ...item,
    i: item?.i,
    x: Number.isFinite(item?.x) ? item.x : 0,
    y: Number.isFinite(item?.y) ? item.y : 0,
    w: clampItemWidth(item?.w, cols),
    h: Number.isFinite(item?.h) ? item.h : 4,
    minW: Number.isFinite(item?.minW) ? item.minW : 1,
    minH: Number.isFinite(item?.minH) ? item.minH : 2,
  };
}

export function normalizeLayoutsByCols(rawLayouts) {
  const source = rawLayouts && typeof rawLayouts === 'object' ? rawLayouts : {};
  const next = createEmptyLayoutsByCols();

  DASHBOARD_COLUMN_MODES.forEach((mode) => {
    const cols = getColumnCount(mode);
    next[mode] = safeLayoutArray(source[mode]).map((item) => normalizeLayoutItem(item, cols));
  });

  return next;
}

export function normalizeLegacyLayoutsToCols(rawLayouts) {
  const source = rawLayouts && typeof rawLayouts === 'object' ? rawLayouts : {};
  const next = createEmptyLayoutsByCols();

  const xl = safeLayoutArray(source.xl).map((item) => normalizeLayoutItem(item, 4));
  const lg = safeLayoutArray(source.lg).map((item) => normalizeLayoutItem(item, 4));
  const md = safeLayoutArray(source.md).map((item) => normalizeLayoutItem(item, 4));
  const sm = safeLayoutArray(source.sm).map((item) => normalizeLayoutItem(item, 2));
  const xs = safeLayoutArray(source.xs).map((item) => normalizeLayoutItem(item, 1));
  const xxs = safeLayoutArray(source.xxs).map((item) => normalizeLayoutItem(item, 1));

  if (xl.length > 0) {
    next['4'] = xl;
  } else if (lg.length > 0 && getLayoutMaxRight(lg) >= 4) {
    next['4'] = lg;
  } else if (md.length > 0 && getLayoutMaxRight(md) >= 4) {
    next['4'] = md;
  } else if (lg.length > 0) {
    next['4'] = lg;
  } else if (md.length > 0) {
    next['4'] = md;
  }

  if (md.length > 0 && getLayoutMaxRight(md) <= 3) {
    next['3'] = md.map((item) => normalizeLayoutItem(item, 3));
  } else if (lg.length > 0 && getLayoutMaxRight(lg) <= 3) {
    next['3'] = lg.map((item) => normalizeLayoutItem(item, 3));
  }

  next['2'] = sm.length > 0 ? sm : [];
  next['1'] = xxs.length > 0 ? xxs : xs;

  return next;
}

export function sortLayoutItemsByPriority(layout, instanceIds) {
  const byId = new Map(safeLayoutArray(layout).map((item) => [item.i, item]));
  const order = new Map();
  safeLayoutArray(instanceIds).forEach((id, index) => {
    order.set(id, index);
  });

  return safeLayoutArray(instanceIds).slice().sort((a, b) => {
    const left = byId.get(a);
    const right = byId.get(b);
    const hasLeft = !!left;
    const hasRight = !!right;
    if (hasLeft && hasRight) {
      const dy = (left.y || 0) - (right.y || 0);
      if (dy !== 0) return dy;
      const dx = (left.x || 0) - (right.x || 0);
      if (dx !== 0) return dx;
    } else if (hasLeft && !hasRight) {
      return -1;
    } else if (!hasLeft && hasRight) {
      return 1;
    }
    return (order.get(a) ?? 0) - (order.get(b) ?? 0);
  });
}

export function buildDenseLayout({
  sourceLayout,
  instanceIds,
  columnMode,
  widgetTypeByInstanceId,
  preferredWidthForType,
  defaultHeightByInstanceId,
  resizeWidths = true,
  resizeHeights = false,
  growWidths = false,
  shrinkWidths = false,
  growHeights = false,
  shrinkHeights = false,
  reorganize = true,
}) {
  const cols = getColumnCount(columnMode);
  const byId = new Map(safeLayoutArray(sourceLayout).map((item) => [item.i, normalizeLayoutItem(item, cols)]));
  const sortedIds = sortLayoutItemsByPriority(sourceLayout, instanceIds);
  const plannedItems = sortedIds.map((instanceId) => {
    const base = byId.get(instanceId) || {};
    const widgetType = widgetTypeByInstanceId.get(instanceId) || '';
    const preferredWidth = typeof preferredWidthForType === 'function'
      ? preferredWidthForType(widgetType, cols)
      : undefined;
    const rawWidth = Number.isFinite(base.w) ? base.w : 1;
    const minH = Number.isFinite(base.minH) ? base.minH : 2;
    const fallbackHeight = defaultHeightByInstanceId.get(instanceId) ?? 4;

    return {
      ...base,
      i: instanceId,
      w: resolvePlannedLayoutWidth({
        rawWidth,
        preferredWidth,
        cols,
        resizeWidths,
        shrinkWidths,
      }),
      h: resolvePlannedLayoutHeight({
        rawHeight: Number.isFinite(base.h) ? base.h : fallbackHeight,
        fallbackHeight,
        minH,
        resizeHeights,
        shrinkHeights,
      }),
      minW: 1,
      minH,
    };
  });

  if (!reorganize) {
    const adjusted = adjustLayoutItemsInPlace(plannedItems, { byId, cols });
    if (!growWidths && !growHeights) {
      return adjusted;
    }
    return expandLayoutItemsToFill(adjusted, {
      cols,
      fillWidths: growWidths,
      fillHeights: growHeights,
    });
  }

  const columnHeights = new Array(cols).fill(0);
  const out = [];

  for (const planned of plannedItems) {
    const nextWidth = planned.w;
    const height = planned.h;
    let bestX = 0;
    let bestY = Number.POSITIVE_INFINITY;

    for (let x = 0; x <= cols - nextWidth; x += 1) {
      let y = 0;
      for (let index = 0; index < nextWidth; index += 1) {
        y = Math.max(y, columnHeights[x + index]);
      }
      if (y < bestY) {
        bestY = y;
        bestX = x;
      }
    }

    const y = Number.isFinite(bestY) ? bestY : 0;
    for (let index = 0; index < nextWidth; index += 1) {
      columnHeights[bestX + index] = y + height;
    }

    out.push({
      ...planned,
      x: bestX,
      y,
    });
  }

  if (!growWidths && !growHeights) {
    return out;
  }

  return expandLayoutItemsToFill(out, {
    cols,
    fillWidths: growWidths,
    fillHeights: growHeights,
  });
}

function resolvePlannedLayoutWidth({ rawWidth, preferredWidth, cols, resizeWidths, shrinkWidths }) {
  const current = clampItemWidth(rawWidth, cols);
  if (!Number.isFinite(preferredWidth)) {
    return current;
  }
  const preferred = clampItemWidth(preferredWidth, cols);
  if (resizeWidths) {
    return preferred;
  }
  if (shrinkWidths) {
    return Math.min(current, preferred);
  }
  return current;
}

function resolvePlannedLayoutHeight({ rawHeight, fallbackHeight, minH, resizeHeights, shrinkHeights }) {
  const current = Math.max(minH, Number.isFinite(rawHeight) ? rawHeight : fallbackHeight);
  const preferred = Math.max(minH, fallbackHeight);
  if (resizeHeights) {
    return preferred;
  }
  if (shrinkHeights) {
    return Math.min(current, preferred);
  }
  return current;
}

function adjustLayoutItemsInPlace(plannedItems, { byId, cols }) {
  const result = [];
  const originals = plannedItems.map((item) => byId.get(item.i) || normalizeLayoutItem(item, cols));

  plannedItems.forEach((planned, index) => {
    const candidate = {
      ...planned,
      x: Math.max(0, Math.min(Number.isFinite(planned.x) ? planned.x : 0, cols - planned.w)),
      y: Number.isFinite(planned.y) ? planned.y : 0,
    };
    const futureOriginals = originals.slice(index + 1);
    const others = [...result, ...futureOriginals];

    if (canPlaceLayoutRect(others, candidate)) {
      result.push(candidate);
      return;
    }

    const fallback = originals[index];
    result.push({
      ...fallback,
      x: Math.max(0, Math.min(Number.isFinite(fallback.x) ? fallback.x : 0, cols - fallback.w)),
      y: Number.isFinite(fallback.y) ? fallback.y : 0,
    });
  });

  return result;
}

function rectanglesOverlap(a, b) {
  return a.x < b.x + b.w
    && b.x < a.x + a.w
    && a.y < b.y + b.h
    && b.y < a.y + a.h;
}

function canPlaceLayoutRect(layout, candidate) {
  return safeLayoutArray(layout).every((item) => !rectanglesOverlap(candidate, item));
}

function rangesOverlap(startA, endA, startB, endB) {
  return startA < endB && startB < endA;
}

function expandLayoutItemsToFill(layout, { cols, fillWidths, fillHeights }) {
  const ordered = safeLayoutArray(layout)
    .map((item) => normalizeLayoutItem(item, cols))
    .sort((left, right) => {
      const dy = left.y - right.y;
      if (dy !== 0) return dy;
      return left.x - right.x;
    });

  return ordered.map((item, index, items) => {
    const others = items.filter((_, otherIndex) => otherIndex !== index);
    let next = { ...item };

    if (fillWidths) {
      let maxRight = cols;
      others.forEach((other) => {
        const sameVerticalBand = rangesOverlap(next.y, next.y + next.h, other.y, other.y + other.h);
        if (!sameVerticalBand || other.x <= next.x) return;
        maxRight = Math.min(maxRight, other.x);
      });
      next.w = clampItemWidth(maxRight - next.x, cols);
    }

    if (fillHeights) {
      let nextBottom = next.y + next.h;
      others.forEach((other) => {
        const sameHorizontalBand = rangesOverlap(next.x, next.x + next.w, other.x, other.x + other.w);
        if (!sameHorizontalBand || other.y < next.y + next.h) return;
        if (nextBottom === next.y + next.h) {
          nextBottom = other.y;
          return;
        }
        nextBottom = Math.min(nextBottom, other.y);
      });
      if (nextBottom > next.y + next.h) {
        next.h = Math.max(next.minH || 1, nextBottom - next.y);
      }
    }

    return next;
  });
}

export function appendLayoutItem({
  layout,
  columnMode,
  instanceId,
  width,
  height,
  minW = 1,
  minH = 2,
}) {
  const cols = getColumnCount(columnMode);
  const normalized = safeLayoutArray(layout).map((item) => normalizeLayoutItem(item, cols));
  const nextWidth = clampItemWidth(width, cols);
  const nextHeight = Math.max(minH, Number.isFinite(height) ? Math.round(height) : 4);
  const ordered = normalized.slice().sort((left, right) => {
    const dy = left.y - right.y;
    if (dy !== 0) return dy;
    return left.x - right.x;
  });
  const last = ordered.at(-1);
  let nextItem = {
    i: instanceId,
    x: 0,
    y: 0,
    w: nextWidth,
    h: nextHeight,
    minW,
    minH,
  };

  if (last) {
    const candidateRight = {
      ...nextItem,
      x: last.x + last.w,
      y: last.y,
    };
    if (candidateRight.x + candidateRight.w <= cols && canPlaceLayoutRect(normalized, candidateRight)) {
      nextItem = candidateRight;
    } else {
      let maxBottom = 0;
      normalized.forEach((item) => {
        maxBottom = Math.max(maxBottom, item.y + item.h);
      });
      nextItem = {
        ...nextItem,
        x: 0,
        y: maxBottom,
      };
    }
  }

  return [...normalized, nextItem];
}

export function normalizeLayoutHeightsByMode(layoutsByCols, { preferredMode } = {}) {
  const source = normalizeLayoutsByCols(layoutsByCols);
  const order = [preferredMode, ...DASHBOARD_COLUMN_MODES].filter(
    (mode, index, arr) => mode && arr.indexOf(mode) === index,
  );
  const heightById = new Map();

  order.forEach((mode) => {
    safeLayoutArray(source[mode]).forEach((item) => {
      if (!item?.i || heightById.has(item.i) || !Number.isFinite(item.h)) return;
      heightById.set(item.i, item.h);
    });
  });

  const next = createEmptyLayoutsByCols();
  DASHBOARD_COLUMN_MODES.forEach((mode) => {
    next[mode] = safeLayoutArray(source[mode]).map((item) => {
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

export function resolveNearestSourceMode(layoutsByCols, targetMode) {
  const normalized = normalizeLayoutsByCols(layoutsByCols);
  const priority = {
    '4': ['4', '3', '2', '1'],
    '3': ['3', '4', '2', '1'],
    '2': ['2', '3', '1', '4'],
    '1': ['1', '2', '3', '4'],
  };

  const candidates = priority[targetMode] || DASHBOARD_COLUMN_MODES;
  for (const mode of candidates) {
    if (safeLayoutArray(normalized[mode]).length > 0) return mode;
  }
  return targetMode;
}

export function generateLayoutForMode({
  layoutsByCols,
  items,
  targetMode,
  widgetTypeByInstanceId,
  preferredWidthForType,
  defaultHeightByInstanceId,
}) {
  const normalized = normalizeLayoutsByCols(layoutsByCols);
  if (safeLayoutArray(normalized[targetMode]).length > 0) return normalized[targetMode];

  const sourceMode = resolveNearestSourceMode(normalized, targetMode);
  const sourceLayout = safeLayoutArray(normalized[sourceMode]);
  const instanceIds = safeLayoutArray(items).map((item) => item?.instance_id).filter(Boolean);
  return buildDenseLayout({
    sourceLayout,
    instanceIds,
    columnMode: targetMode,
    widgetTypeByInstanceId,
    preferredWidthForType,
    defaultHeightByInstanceId,
  });
}

export function ensureLayoutsByCols({
  layoutsByCols,
  items,
  widgetTypeByInstanceId,
  preferredWidthForType,
  defaultHeightByInstanceId,
  targetModes = DASHBOARD_COLUMN_MODES,
}) {
  const normalized = normalizeLayoutsByCols(layoutsByCols);
  const next = { ...normalized };

  targetModes.forEach((mode) => {
    next[mode] = generateLayoutForMode({
      layoutsByCols: next,
      items,
      targetMode: mode,
      widgetTypeByInstanceId,
      preferredWidthForType,
      defaultHeightByInstanceId,
    });
  });

  return next;
}

export function createDashboardDocSnapshot(doc) {
  const source = doc && typeof doc === 'object' ? doc : {};
  const items = safeLayoutArray(source.items).map((item) => ({
    instance_id: item?.instance_id,
    widget_type: item?.widget_type,
    enabled: item?.enabled !== false,
    settings: item?.settings || {},
  }));
  const layoutsByCols = normalizeLayoutsByCols(source.layoutsByCols || source.layouts_by_cols || {});
  return JSON.stringify({ items, layouts_by_cols: layoutsByCols });
}

export function parseDashboardDocShape(rawDoc) {
  const doc = rawDoc && typeof rawDoc === 'object' ? rawDoc : {};
  const items = safeLayoutArray(doc.items);
  const hasLayoutsByCols = doc.layouts_by_cols && typeof doc.layouts_by_cols === 'object';
  const hasLegacyLayouts = doc.layouts && typeof doc.layouts === 'object';
  const layoutsByCols = hasLayoutsByCols
    ? normalizeLayoutsByCols(doc.layouts_by_cols)
    : hasLegacyLayouts
      ? normalizeLegacyLayoutsToCols(doc.layouts)
      : createEmptyLayoutsByCols();

  return {
    items,
    layoutsByCols,
  };
}

export function serializeDashboardDoc(doc) {
  const source = doc && typeof doc === 'object' ? doc : {};
  return {
    items: safeLayoutArray(source.items),
    layouts_by_cols: normalizeLayoutsByCols(source.layoutsByCols),
  };
}

export function mapColumnModeToLegacyBreakpoint(mode) {
  if (mode === '4') return 'lg';
  if (mode === '3') return 'md';
  if (mode === '2') return 'sm';
  return 'xxs';
}

export function listLegacyBreakpoints() {
  return LEGACY_BREAKPOINTS.slice();
}