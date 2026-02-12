import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Responsive, WidthProvider } from 'react-grid-layout';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faPen, faPlus, faCheck, faTrash } from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../context/AuthContext';
import useDashboard from '../../../hooks/useDashboard';
import UnauthorizedView from '../../common/UnauthorizedView/UnauthorizedView';
import LoadingView from '../../common/LoadingView/LoadingView';
import WidgetRenderer from './WidgetRenderer';
import { getWidgetDefaultHeight } from './widgetRegistry';
import AddWidgetModal from './AddWidgetModal';
import WidgetSettingsModal from './WidgetSettingsModal';
import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';
import './Dashboard.css';

const ResponsiveGridLayout = WidthProvider(Responsive);

// Dashboard breakpoint configuration
// NOTE: WidthProvider measures the actual container width (sidebar-aware),
// so these breakpoints track available dashboard space rather than viewport width.
// We intentionally keep a small set of column modes.
//
// The 2-col cutoff is intentionally low because the Home view has extra padding,
// and the permanent sidebar reduces the measured container width.
//
const BREAKPOINTS = { xl: 1500, lg: 1200, md: 1080, sm: 720, xxs: 0 };
const COLS = { xl: 4, lg: 3, md: 3, sm: 2, xxs: 1 };
const ROW_HEIGHT = 56; // Fixed row height for consistent snapping
const MARGIN = [16, 16];

function parseAspectRatio(raw) {
  if (!raw || typeof raw !== 'string') return null;
  const trimmed = raw.trim();
  if (!trimmed || trimmed === 'auto') return null;
  if (trimmed.includes('/')) {
    const [aRaw, bRaw] = trimmed.split('/');
    const a = Number.parseFloat(String(aRaw).trim());
    const b = Number.parseFloat(String(bRaw).trim());
    if (Number.isFinite(a) && Number.isFinite(b) && a > 0 && b > 0) return a / b;
    return null;
  }
  const n = Number.parseFloat(trimmed);
  if (Number.isFinite(n) && n > 0) return n;
  return null;
}

function pxToRows(px) {
  // Convert pixel height to grid rows, rounding up.
  // Formula: totalPx = rows * ROW_HEIGHT + (rows - 1) * margin
  // Solving for rows: rows = (totalPx + margin) / (ROW_HEIGHT + margin)
  const marginY = MARGIN[1];
  return Math.max(1, Math.ceil((px + marginY) / (ROW_HEIGHT + marginY)));
}

function pickSourceBreakpoint(layoutsByBp) {
  const candidates = ['lg', 'md', 'sm', 'xxs'];
  for (const bp of candidates) {
    const items = layoutsByBp?.[bp];
    if (Array.isArray(items) && items.length > 0) return bp;
  }
  return 'lg';
}

function reflowToCols({ sourceLayout, instanceIds, cols }) {
  const clampedCols = Math.max(1, Math.floor(cols || 1));
  const byId = new Map((sourceLayout || []).map((it) => [it.i, it]));

  const indexById = new Map();
  (instanceIds || []).forEach((id, idx) => indexById.set(id, idx));

  const sortedIds = (instanceIds || []).slice().sort((a, b) => {
    const la = byId.get(a);
    const lb = byId.get(b);
    const hasA = !!la;
    const hasB = !!lb;
    if (hasA && hasB) {
      const dy = (la.y || 0) - (lb.y || 0);
      if (dy !== 0) return dy;
      const dx = (la.x || 0) - (lb.x || 0);
      if (dx !== 0) return dx;
    } else if (hasA && !hasB) {
      return -1;
    } else if (!hasA && hasB) {
      return 1;
    }
    return (indexById.get(a) ?? 0) - (indexById.get(b) ?? 0);
  });

  const colHeights = new Array(clampedCols).fill(0);
  const out = [];

  for (const id of sortedIds) {
    const base = byId.get(id) || {};
    const minH = Number.isFinite(base.minH) ? base.minH : 2;
    const h = Math.max(minH, Number.isFinite(base.h) ? base.h : 4);

    const rawW = Number.isFinite(base.w) ? base.w : 1;
    const w = Math.max(1, Math.min(clampedCols, rawW));

    let bestX = 0;
    let bestY = Infinity;

    for (let x = 0; x <= clampedCols - w; x += 1) {
      let y = 0;
      for (let k = 0; k < w; k += 1) {
        y = Math.max(y, colHeights[x + k]);
      }
      if (y < bestY) {
        bestY = y;
        bestX = x;
      }
    }

    const nextY = Number.isFinite(bestY) ? bestY : 0;
    for (let k = 0; k < w; k += 1) {
      colHeights[bestX + k] = nextY + h;
    }

    out.push({
      i: id,
      x: bestX,
      y: nextY,
      w,
      h,
      minW: 1,
      minH,
    });
  }

  return out;
}

function getMaxRight(layout) {
  if (!Array.isArray(layout) || layout.length === 0) return 0;
  let maxRight = 0;
  for (const it of layout) {
    const x = Number.isFinite(it?.x) ? it.x : 0;
    const w = Number.isFinite(it?.w) ? it.w : 1;
    maxRight = Math.max(maxRight, x + w);
  }
  return maxRight;
}

export default function Dashboard() {
  const { user, accessToken, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const {
    dashboard,
    doc,
    catalog,
    loading,
    saving,
    error,
    updateLayouts,
    addWidget,
    removeWidget,
    updateWidgetSettings,
    getWidget,
    flushSave,
  } = useDashboard({ enabled: isResidentOrAdmin, accessToken });

  const [editMode, setEditMode] = useState(false);
  const [addModalOpen, setAddModalOpen] = useState(false);
  const [settingsModalOpen, setSettingsModalOpen] = useState(false);
  const [selectedWidgetId, setSelectedWidgetId] = useState(null);
  const [dragOverTrash, setDragOverTrash] = useState(false);
  const draggingWidgetRef = useRef(null);
  const trashZoneRef = useRef(null);

  // Current breakpoint state
  const [currentBreakpoint, setCurrentBreakpoint] = useState('lg');

  // Grid container ref for layout
  const gridContainerRef = useRef(null);

  // Client-side desired heights (e.g. map widget auto-sizing).
  const [desiredRowsByInstanceId, setDesiredRowsByInstanceId] = useState({});

  const widgetTypeByInstanceId = useMemo(() => {
    const map = new Map();
    (doc.items || []).forEach((item) => {
      if (item?.instance_id) {
        map.set(item.instance_id, item.widget_type);
      }
    });
    return map;
  }, [doc.items]);

  // Listen for widget auto-height requests
  useEffect(() => {
    const handler = (ev) => {
      const detail = ev?.detail;
      const instanceId = detail?.instanceId;
      const heightPx = Number(detail?.heightPx);
      if (!instanceId || !Number.isFinite(heightPx) || heightPx <= 0) return;

      const rows = pxToRows(heightPx);
      setDesiredRowsByInstanceId((prev) => {
        const current = prev[instanceId];
        if (current === rows) return prev;
        return { ...prev, [instanceId]: rows };
      });
    };

    window.addEventListener('homenavi:widgetDesiredHeight', handler);
    return () => window.removeEventListener('homenavi:widgetDesiredHeight', handler);
  }, []);

  // Handle entering edit mode
  const enterEditMode = useCallback(() => {
    setEditMode(true);
  }, []);

  // Handle exiting edit mode
  const exitEditMode = useCallback(() => {
    flushSave();
    setEditMode(false);
    setDragOverTrash(false);
  }, [flushSave]);

  // Handle layout changes
  const handleLayoutChange = useCallback((currentLayout, allLayouts) => {
    if (!editMode) return;
    updateLayouts(allLayouts);
  }, [editMode, updateLayouts]);

  // Handle breakpoint change
  const handleBreakpointChange = useCallback((newBreakpoint) => {
    setCurrentBreakpoint(newBreakpoint);
  }, []);

  // Handle widget settings
  const handleWidgetSettings = useCallback((instanceId) => {
    setSelectedWidgetId(instanceId);
    setSettingsModalOpen(true);
  }, []);

  // Handle widget remove from button
  const handleWidgetRemove = useCallback((instanceId) => {
    removeWidget(instanceId);
  }, [removeWidget]);

  // Handle add widget from catalog
  const handleAddWidget = useCallback((widgetType, initialSettings = {}) => {
    addWidget(widgetType, initialSettings);
    setAddModalOpen(false);
  }, [addWidget]);

  // Handle drag start for trash zone detection
  const handleDragStart = useCallback((...args) => {
    const oldItem = args[1];
    draggingWidgetRef.current = oldItem?.i || null;
  }, []);

  // Handle drag stop - check if dropped on trash
  const handleDragStop = useCallback(() => {
    if (dragOverTrash && draggingWidgetRef.current) {
      removeWidget(draggingWidgetRef.current);
    }
    draggingWidgetRef.current = null;
    setDragOverTrash(false);
  }, [dragOverTrash, removeWidget]);

  // Handle drag movement to detect trash zone
  const handleDrag = useCallback((...args) => {
    const e = args[4];
    if (!trashZoneRef.current || !editMode) return;
    
    const trashRect = trashZoneRef.current.getBoundingClientRect();
    const mouseY = e.clientY;
    const mouseX = e.clientX;
    
    const isOverTrash = (
      mouseX >= trashRect.left &&
      mouseX <= trashRect.right &&
      mouseY >= trashRect.top &&
      mouseY <= trashRect.bottom
    );
    
    setDragOverTrash(isOverTrash);
  }, [editMode]);

  // Convert dashboard layouts to react-grid-layout format
  const layouts = useMemo(() => {
    if (!doc.layouts || Object.keys(doc.layouts).length === 0) {
      return { lg: [], md: [], sm: [], xxs: [] };
    }

    const result = {};
    Object.entries(doc.layouts).forEach(([bp, items]) => {
      result[bp] = (items || []).map((item) => ({
        i: item.i,
        x: item.x || 0,
        y: item.y || 0,
        w: item.w || 1,
        h: (() => {
          const raw = item.h || 4;
          const widgetType = widgetTypeByInstanceId.get(item.i);
          const defaultH = getWidgetDefaultHeight(widgetType, catalog);

          if (widgetType === 'homenavi.map' || widgetType === 'homenavi.device') {
            // Auto-height only while the widget is still at its default height.
            // If the user resizes vertically, raw changes and we preserve it.
            const desired = desiredRowsByInstanceId[item.i];
            if (raw === defaultH && Number.isFinite(desired) && desired > 0) {
              return Math.max(2, desired);
            }
          }
          return raw;
        })(),
        minW: 1,
        minH: 2,
      }));
    });

    // In view mode, enforce deterministic 2-col/1-col ordering based on the widest layout.
    // This keeps the top-row widgets first when collapsing the dashboard.
    if (!editMode) {
      const instanceIds = (doc.items || []).map((it) => it?.instance_id).filter(Boolean);
      const sourceBp = pickSourceBreakpoint(result);
      const source = Array.isArray(result[sourceBp]) ? result[sourceBp] : [];

      // Always normalize the collapsed layouts.
      // Additionally, if a saved layout for a wider breakpoint only uses a single column
      // (all items at x=0, w=1), reflow it as well so the dashboard fills available width.
      ['lg', 'md', 'sm', 'xxs'].forEach((bp) => {
        const cols = COLS[bp];
        if (!Number.isFinite(cols) || cols <= 0) return;

        const existing = result[bp];
        const maxRight = getMaxRight(existing);
        const isCollapsedBp = bp === 'sm' || bp === 'xxs';
        const isSingleColumn = maxRight <= 1;

        if (isCollapsedBp || (cols > 1 && isSingleColumn && instanceIds.length > 1)) {
          result[bp] = reflowToCols({ sourceLayout: source, instanceIds, cols });
        }
      });
    }

    return result;
  }, [catalog, desiredRowsByInstanceId, doc.items, doc.layouts, editMode, widgetTypeByInstanceId]);

  // Get widget items
  const widgetItems = useMemo(() => doc.items || [], [doc.items]);

  const handleResizeStop = useCallback((layout, oldItem, newItem, placeholder, e, element) => {
    if (!editMode) return;
    const instanceId = newItem?.i;
    if (!instanceId) return;

    const widgetType = widgetTypeByInstanceId.get(instanceId);
    if (widgetType !== 'homenavi.map') return;
    // If the user is adjusting height manually, don't force recalculation.
    // We only auto-adjust map height when width changes while height is still at default.
    const defaultH = getWidgetDefaultHeight(widgetType, catalog);
    const didWidthChange = oldItem?.w !== newItem?.w;
    const isStillDefaultHeight = (newItem?.h || 0) === defaultH;
    if (!didWidthChange || !isStillDefaultHeight) return;
    if (!element || typeof element.querySelector !== 'function') return;

    const previewEl = element.querySelector('.map-widget__preview');
    if (!previewEl) return;

    const mapContentEl = element.querySelector('.map-widget__content');
    if (!mapContentEl) return;

    const previewRect = previewEl.getBoundingClientRect();
    // Use natural content height rather than stretched grid item height.
    const overheadPx = Math.max(0, mapContentEl.scrollHeight - previewRect.height);

    const computed = window.getComputedStyle(previewEl);
    const ratio = parseAspectRatio(computed.aspectRatio) || parseAspectRatio(previewEl.style.aspectRatio) || (16 / 10);
    const previewWidth = previewRect.width;
    if (!Number.isFinite(previewWidth) || previewWidth <= 0) return;
    if (!Number.isFinite(ratio) || ratio <= 0) return;

    const desiredPreviewHeight = previewWidth / ratio;
    const desiredTotalHeight = overheadPx + desiredPreviewHeight;
    const desiredRows = Math.max(newItem?.minH || 2, pxToRows(desiredTotalHeight));

    const bp = currentBreakpoint;
    const existing = Array.isArray(doc.layouts?.[bp]) ? doc.layouts[bp] : [];
    const nextBp = existing.map((it) => (it.i === instanceId ? { ...it, h: desiredRows } : it));
    const nextLayouts = { ...doc.layouts, [bp]: nextBp };
    updateLayouts(nextLayouts);
  }, [catalog, currentBreakpoint, doc.layouts, editMode, updateLayouts, widgetTypeByInstanceId]);

  // Permission check
  if (bootstrapping) {
    return <LoadingView message="Loading..." />;
  }

  if (!isResidentOrAdmin) {
    return (
      <UnauthorizedView
        title=""
        message="Sign in with a resident or admin account to view your dashboard."
        hideHeader
      />
    );
  }

  if (loading && !dashboard) {
    return <LoadingView message="Loading dashboard..." />;
  }

  if (error && !dashboard) {
    return (
      <div className="dashboard-error">
        <div className="dashboard-error__message">{error}</div>
      </div>
    );
  }

  return (
    <div className={`dashboard ${editMode ? 'dashboard--edit' : ''}`.trim()}>
      {/* Main grid layout */}
      <div className="dashboard__grid-container" ref={gridContainerRef}>
        <ResponsiveGridLayout
          className="dashboard__grid"
          layouts={layouts}
          breakpoints={BREAKPOINTS}
          cols={COLS}
          rowHeight={ROW_HEIGHT}
          margin={MARGIN}
          containerPadding={MARGIN}
          isDraggable={editMode}
          isResizable={editMode}
          onLayoutChange={handleLayoutChange}
          onBreakpointChange={handleBreakpointChange}
          onDragStart={handleDragStart}
          onDrag={handleDrag}
          onDragStop={handleDragStop}
          onResizeStop={handleResizeStop}
          draggableHandle=".widget-shell__drag-handle"
          // Allow vertical resizing; snapping is row-based.
          resizeHandles={['se', 'sw', 'ne', 'nw', 'e', 'w', 's', 'n']}
          compactType="vertical"
          preventCollision={false}
          useCSSTransforms
        >
          {widgetItems.map((item) => (
            <div key={item.instance_id} className="dashboard__widget-wrapper">
              <WidgetRenderer
                instanceId={item.instance_id}
                widgetType={item.widget_type}
                settings={item.settings || {}}
                catalog={catalog}
                enabled={item.enabled !== false}
                editMode={editMode}
                onSettings={() => handleWidgetSettings(item.instance_id)}
                onSaveSettings={(instanceId, newSettings) => {
                  updateWidgetSettings(instanceId, newSettings);
                }}
                onRemove={() => handleWidgetRemove(item.instance_id)}
              />
            </div>
          ))}
        </ResponsiveGridLayout>
      </div>

      {/* Trash drop zone (visible in edit mode) */}
      {editMode && (
        <div
          ref={trashZoneRef}
          className={`dashboard__trash-zone ${dragOverTrash ? 'dashboard__trash-zone--active' : ''}`}
        >
          <FontAwesomeIcon icon={faTrash} />
          <span>Drop to remove</span>
        </div>
      )}

      {/* Floating action buttons */}
      <div className="dashboard__fab-container">
        {!editMode ? (
          <button
            className="dashboard__fab dashboard__fab--edit"
            onClick={enterEditMode}
            title="Edit dashboard"
          >
            <FontAwesomeIcon icon={faPen} />
          </button>
        ) : (
          <>
            <button
              className="dashboard__fab dashboard__fab--add"
              onClick={() => setAddModalOpen(true)}
              title="Add widget"
            >
              <FontAwesomeIcon icon={faPlus} />
            </button>
            <button
              className="dashboard__fab dashboard__fab--done"
              onClick={exitEditMode}
              title="Done editing"
            >
              <FontAwesomeIcon icon={faCheck} />
            </button>
          </>
        )}
      </div>

      {/* Saving indicator */}
      {saving && (
        <div className="dashboard__saving-indicator">
          Saving...
        </div>
      )}

      {/* Add widget modal */}
      <AddWidgetModal
        open={addModalOpen}
        onClose={() => setAddModalOpen(false)}
        catalog={catalog}
        onAdd={handleAddWidget}
      />

      {/* Widget settings modal */}
      <WidgetSettingsModal
        open={settingsModalOpen}
        onClose={() => {
          setSettingsModalOpen(false);
          setSelectedWidgetId(null);
        }}
        widgetItem={selectedWidgetId ? getWidget(selectedWidgetId) : null}
        onSave={(instanceId, newSettings) => {
          updateWidgetSettings(instanceId, newSettings);
          setSettingsModalOpen(false);
          setSelectedWidgetId(null);
        }}
        onRemove={(instanceId) => {
          removeWidget(instanceId);
          setSettingsModalOpen(false);
          setSelectedWidgetId(null);
        }}
      />
    </div>
  );
}
