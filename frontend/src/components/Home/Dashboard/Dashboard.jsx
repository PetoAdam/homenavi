import React, { useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react';
import GridLayout, { WidthProvider } from 'react-grid-layout';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowRight,
  faCheck,
  faCircleInfo,
  faPen,
  faPlus,
  faRotateLeft,
  faRotateRight,
  faSliders,
  faTrash,
  faWandMagicSparkles,
} from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../context/AuthContext';
import useDashboard from '../../../hooks/useDashboard';
import useEditorHistory from '../../../hooks/useEditorHistory';
import UnauthorizedView from '../../common/UnauthorizedView/UnauthorizedView';
import LoadingView from '../../common/LoadingView/LoadingView';
import BaseModal from '../../common/BaseModal/BaseModal';
import WidgetRenderer from './WidgetRenderer';
import {
  appendLayoutItem,
  buildDenseLayout,
  createDashboardDocSnapshot,
  ensureLayoutsByCols,
  DASHBOARD_COLUMN_MODES,
  getColumnCount,
  getDashboardColumnMode,
  resolveNearestSourceMode,
} from './dashboardLayoutModel';
import { getWidgetDefaultHeight, getWidgetPreferredWidth } from './widgetRegistry';
import AddWidgetModal from './AddWidgetModal';
import DashboardLayoutToolsModal from './DashboardLayoutToolsModal';
import WidgetSettingsModal from './WidgetSettingsModal';
import { dashboardUiInitialState, dashboardUiReducer } from './dashboardUiReducer';
import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';
import './Dashboard.css';

const DashboardGridLayout = WidthProvider(GridLayout);
const ROW_HEIGHT = 56; // Fixed row height for consistent snapping
const MARGIN = [16, 16];
const DASHBOARD_TUTORIAL_DISMISSED_KEY = 'homenavi:dashboard:tutorial-dismissed:v1';

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

export default function Dashboard() {
  const { user, accessToken, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const {
    dashboard,
    doc,
    catalog,
    loading,
    error,
    saveParsedDoc,
    flushSave,
  } = useDashboard({ enabled: isResidentOrAdmin, accessToken });

  const [uiState, dispatchUi] = useReducer(dashboardUiReducer, undefined, dashboardUiInitialState);
  const {
    editMode,
    addModalOpen,
    settingsModalOpen,
    selectedWidgetId,
    dragOverTrash,
    currentBreakpoint,
    desiredRowsByInstanceId,
  } = uiState;

  const draggingWidgetRef = useRef(null);
  const trashZoneRef = useRef(null);
  const [viewportWidth, setViewportWidth] = useState(() => (typeof window !== 'undefined' ? window.innerWidth : 1440));
  const [toolsOpen, setToolsOpen] = useState(false);
  const [tutorialOpen, setTutorialOpen] = useState(false);
  const [importSourceMode, setImportSourceMode] = useState('4');
  const [autoLayoutOptions, setAutoLayoutOptions] = useState({
    reorganize: true,
    growWidths: false,
    shrinkWidths: false,
    growHeights: false,
    shrinkHeights: false,
  });
  const skipNextEditorPersistRef = useRef(false);
  const lastSyncedServerDocSnapshotRef = useRef('');

  // Grid container ref for layout
  const gridContainerRef = useRef(null);

  useEffect(() => {
    const handleResize = () => setViewportWidth(window.innerWidth);
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const activeColumnMode = useMemo(() => getDashboardColumnMode(viewportWidth), [viewportWidth]);

  useEffect(() => {
    dispatchUi({ type: 'set-current-breakpoint', value: activeColumnMode });
  }, [activeColumnMode]);

  const {
    editor: editorDoc,
    setEditor,
    resetHistory,
    applyEditorUpdate,
    applyEditorUpdateBatched,
    canUndo,
    canRedo,
    undo,
    redo,
  } = useEditorHistory({
    initialEditor: doc,
    snapshotForSave: createDashboardDocSnapshot,
  });

  const workingDoc = editMode ? editorDoc : doc;
  const serverDocSnapshot = useMemo(() => createDashboardDocSnapshot(doc), [doc]);

  const widgetTypeByInstanceId = useMemo(() => {
    const map = new Map();
    (workingDoc.items || []).forEach((item) => {
      if (item?.instance_id) {
        map.set(item.instance_id, item.widget_type);
      }
    });
    return map;
  }, [workingDoc.items]);

  const defaultHeightByInstanceId = useMemo(() => {
    const map = new Map();
    (workingDoc.items || []).forEach((item) => {
      if (!item?.instance_id) return;
      map.set(item.instance_id, getWidgetDefaultHeight(item.widget_type, catalog));
    });
    return map;
  }, [catalog, workingDoc.items]);

  useEffect(() => {
    if (importSourceMode !== currentBreakpoint) return;
    const nextMode = DASHBOARD_COLUMN_MODES.find((mode) => mode !== currentBreakpoint) || '4';
    setImportSourceMode(nextMode);
  }, [currentBreakpoint, importSourceMode]);

  const syncEditorToServerDoc = useCallback((nextDoc) => {
    skipNextEditorPersistRef.current = true;
    setEditor(nextDoc);
    resetHistory();
  }, [resetHistory, setEditor]);

  useEffect(() => {
    if (editMode) return;
    if (lastSyncedServerDocSnapshotRef.current === serverDocSnapshot) return;
    lastSyncedServerDocSnapshotRef.current = serverDocSnapshot;
    syncEditorToServerDoc(doc);
  }, [doc, editMode, serverDocSnapshot, syncEditorToServerDoc]);

  useEffect(() => {
    if (!editMode) return;
    if (skipNextEditorPersistRef.current) {
      skipNextEditorPersistRef.current = false;
      return;
    }
    saveParsedDoc(editorDoc);
  }, [editMode, editorDoc, saveParsedDoc]);

  useEffect(() => {
    if (!editMode) return undefined;
    const handleKeyDown = (event) => {
      if (!(event.metaKey || event.ctrlKey)) return;
      if (event.key.toLowerCase() !== 'z') return;
      event.preventDefault();
      if (event.shiftKey) {
        redo();
        return;
      }
      undo();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [editMode, redo, undo]);

  const applyDashboardDocUpdate = useCallback((updater, { batchKey } = {}) => {
    if (editMode) {
      const applyChange = (prev) => {
        const base = prev && typeof prev === 'object' ? prev : doc;
        const next = updater(base);
        return next || base;
      };
      if (batchKey) {
        applyEditorUpdateBatched(batchKey, applyChange);
      } else {
        applyEditorUpdate(applyChange);
      }
      return;
    }
    const next = updater(doc);
    if (next) saveParsedDoc(next);
  }, [applyEditorUpdate, applyEditorUpdateBatched, doc, editMode, saveParsedDoc]);

  const buildWidgetMaps = useCallback((items) => {
    const widgetTypeMap = new Map();
    const defaultHeightMap = new Map();
    (items || []).forEach((item) => {
      if (!item?.instance_id) return;
      widgetTypeMap.set(item.instance_id, item.widget_type);
      defaultHeightMap.set(item.instance_id, getWidgetDefaultHeight(item.widget_type, catalog));
    });
    return { widgetTypeMap, defaultHeightMap };
  }, [catalog]);

  const dismissTutorial = useCallback((persistDismissal) => {
    if (persistDismissal) {
      window.localStorage.setItem(DASHBOARD_TUTORIAL_DISMISSED_KEY, '1');
    }
    setTutorialOpen(false);
  }, []);

  // Listen for widget auto-height requests
  useEffect(() => {
    const handler = (ev) => {
      const detail = ev?.detail;
      const instanceId = detail?.instanceId;
      const heightPx = Number(detail?.heightPx);
      if (!instanceId || !Number.isFinite(heightPx) || heightPx <= 0) return;

      const rows = pxToRows(heightPx);
      dispatchUi({ type: 'set-desired-row-height', instanceId, value: rows });
    };

    window.addEventListener('homenavi:widgetDesiredHeight', handler);
    return () => window.removeEventListener('homenavi:widgetDesiredHeight', handler);
  }, []);

  // Handle entering edit mode
  const enterEditMode = useCallback(() => {
    syncEditorToServerDoc(doc);
    dispatchUi({ type: 'set-edit-mode', value: true });
    const tutorialDismissed = typeof window !== 'undefined'
      && window.localStorage.getItem(DASHBOARD_TUTORIAL_DISMISSED_KEY) === '1';
    if (!tutorialDismissed) {
      setTutorialOpen(true);
    }
  }, [doc, syncEditorToServerDoc]);

  // Handle exiting edit mode
  const exitEditMode = useCallback(() => {
    flushSave();
    dispatchUi({ type: 'set-edit-mode', value: false });
    dispatchUi({ type: 'set-drag-over-trash', value: false });
    setToolsOpen(false);
  }, [flushSave]);

  // Handle layout changes
  const handleLayoutChange = useCallback((currentLayout) => {
    if (!editMode) return;
    applyDashboardDocUpdate((prev) => ({
      ...prev,
      layoutsByCols: {
        ...prev.layoutsByCols,
        [currentBreakpoint]: currentLayout,
      },
    }), { batchKey: `layout:${currentBreakpoint}` });
  }, [applyDashboardDocUpdate, currentBreakpoint, editMode]);

  // Handle widget settings
  const handleWidgetSettings = useCallback((instanceId) => {
    dispatchUi({ type: 'set-selected-widget-id', value: instanceId });
    dispatchUi({ type: 'set-settings-modal-open', value: true });
  }, []);

  // Handle widget remove from button
  const handleWidgetRemove = useCallback((instanceId) => {
    applyDashboardDocUpdate((prev) => ({
      ...prev,
      items: prev.items.filter((item) => item.instance_id !== instanceId),
      layoutsByCols: Object.fromEntries(
        Object.entries(prev.layoutsByCols || {}).map(([mode, items]) => [mode, items.filter((item) => item.i !== instanceId)]),
      ),
    }));
  }, [applyDashboardDocUpdate]);

  // Handle add widget from catalog
  const handleAddWidget = useCallback((widgetType, initialSettings = {}) => {
    applyDashboardDocUpdate((prev) => {
      const instanceId = crypto.randomUUID();
      const newItem = {
        instance_id: instanceId,
        widget_type: widgetType,
        enabled: true,
        settings: initialSettings,
      };
      const defaultH = getWidgetDefaultHeight(widgetType, catalog);
      const maps = buildWidgetMaps(prev.items);
      const ensured = ensureLayoutsByCols({
        layoutsByCols: prev.layoutsByCols,
        items: prev.items,
        widgetTypeByInstanceId: maps.widgetTypeMap,
        preferredWidthForType: (type, cols) => getWidgetPreferredWidth(type, catalog, String(cols)),
        defaultHeightByInstanceId: maps.defaultHeightMap,
      });
      const layoutsByCols = {};
      DASHBOARD_COLUMN_MODES.forEach((columnMode) => {
        layoutsByCols[columnMode] = appendLayoutItem({
          layout: ensured[columnMode],
          columnMode,
          instanceId,
          width: getWidgetPreferredWidth(widgetType, catalog, columnMode),
          height: defaultH,
        });
      });
      return {
        ...prev,
        items: [...prev.items, newItem],
        layoutsByCols,
      };
    });
    dispatchUi({ type: 'set-add-modal-open', value: false });
  }, [applyDashboardDocUpdate, buildWidgetMaps, catalog, currentBreakpoint]);

  // Handle drag start for trash zone detection
  const handleDragStart = useCallback((...args) => {
    const oldItem = args[1];
    draggingWidgetRef.current = oldItem?.i || null;
  }, []);

  // Handle drag stop - check if dropped on trash
  const handleDragStop = useCallback(() => {
    if (dragOverTrash && draggingWidgetRef.current) {
      handleWidgetRemove(draggingWidgetRef.current);
    }
    draggingWidgetRef.current = null;
    dispatchUi({ type: 'set-drag-over-trash', value: false });
  }, [dragOverTrash, handleWidgetRemove]);

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
    
    dispatchUi({ type: 'set-drag-over-trash', value: isOverTrash });
  }, [editMode]);

  // Convert dashboard layouts to react-grid-layout format
  const layoutsByCols = useMemo(() => {
    const ensured = ensureLayoutsByCols({
      layoutsByCols: workingDoc.layoutsByCols,
      items: workingDoc.items || [],
      widgetTypeByInstanceId,
      preferredWidthForType: (widgetType, cols) => getWidgetPreferredWidth(widgetType, catalog, String(cols)),
      defaultHeightByInstanceId,
    });

    const result = {};
    Object.entries(ensured).forEach(([columnMode, items]) => {
      result[columnMode] = (items || []).map((item) => ({
        i: item.i,
        x: item.x || 0,
        y: item.y || 0,
        w: item.w || 1,
        h: (() => {
          const raw = item.h || 4;
          const widgetType = widgetTypeByInstanceId.get(item.i);
          const defaultH = getWidgetDefaultHeight(widgetType, catalog);

          const isIntegrationWidget = typeof widgetType === 'string' && widgetType.includes('.integration.');
          if (widgetType === 'homenavi.map' || widgetType === 'homenavi.device' || isIntegrationWidget) {
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

    return result;
  }, [activeColumnMode, catalog, defaultHeightByInstanceId, desiredRowsByInstanceId, widgetTypeByInstanceId, workingDoc.items, workingDoc.layoutsByCols]);

  const activeLayout = useMemo(() => layoutsByCols[currentBreakpoint] || [], [currentBreakpoint, layoutsByCols]);

  // Get widget items
  const widgetItems = useMemo(() => workingDoc.items || [], [workingDoc.items]);
  const selectedWidget = useMemo(
    () => (selectedWidgetId ? widgetItems.find((item) => item.instance_id === selectedWidgetId) || null : null),
    [selectedWidgetId, widgetItems],
  );

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
    const existing = Array.isArray(workingDoc.layoutsByCols?.[bp]) ? workingDoc.layoutsByCols[bp] : [];
    const nextBp = existing.map((it) => (it.i === instanceId ? { ...it, h: desiredRows } : it));
    applyDashboardDocUpdate((prev) => ({
      ...prev,
      layoutsByCols: { ...prev.layoutsByCols, [bp]: nextBp },
    }));
  }, [applyDashboardDocUpdate, catalog, currentBreakpoint, editMode, widgetTypeByInstanceId, workingDoc.layoutsByCols]);

  const importIntoCurrent = useCallback(() => {
    applyDashboardDocUpdate((prev) => {
      const maps = buildWidgetMaps(prev.items);
      return {
        ...prev,
        layoutsByCols: {
          ...prev.layoutsByCols,
          [currentBreakpoint]: buildDenseLayout({
            sourceLayout: prev.layoutsByCols?.[importSourceMode] || [],
            instanceIds: (prev.items || []).map((item) => item?.instance_id).filter(Boolean),
            columnMode: currentBreakpoint,
            widgetTypeByInstanceId: maps.widgetTypeMap,
            preferredWidthForType: (widgetType, cols) => getWidgetPreferredWidth(widgetType, catalog, String(cols)),
            defaultHeightByInstanceId: maps.defaultHeightMap,
            resizeWidths: false,
            resizeHeights: false,
          }),
        },
      };
    });
    setToolsOpen(false);
  }, [applyDashboardDocUpdate, buildWidgetMaps, catalog, currentBreakpoint, importSourceMode]);

  const autoLayoutCurrent = useCallback(() => {
    applyDashboardDocUpdate((prev) => {
      const maps = buildWidgetMaps(prev.items);
      return {
        ...prev,
        layoutsByCols: {
          ...prev.layoutsByCols,
          [currentBreakpoint]: buildDenseLayout({
            sourceLayout: prev.layoutsByCols?.[currentBreakpoint] || [],
            instanceIds: (prev.items || []).map((item) => item?.instance_id).filter(Boolean),
            columnMode: currentBreakpoint,
            widgetTypeByInstanceId: maps.widgetTypeMap,
            preferredWidthForType: (widgetType, cols) => getWidgetPreferredWidth(widgetType, catalog, String(cols)),
            defaultHeightByInstanceId: maps.defaultHeightMap,
            resizeWidths: false,
            resizeHeights: false,
            growWidths: autoLayoutOptions.growWidths,
            shrinkWidths: autoLayoutOptions.shrinkWidths,
            growHeights: autoLayoutOptions.growHeights,
            shrinkHeights: autoLayoutOptions.shrinkHeights,
            reorganize: autoLayoutOptions.reorganize,
          }),
        },
      };
    });
    setToolsOpen(false);
  }, [
    applyDashboardDocUpdate,
    autoLayoutOptions.growHeights,
    autoLayoutOptions.growWidths,
    autoLayoutOptions.reorganize,
    autoLayoutOptions.shrinkHeights,
    autoLayoutOptions.shrinkWidths,
    buildWidgetMaps,
    catalog,
    currentBreakpoint,
  ]);

  const openLayoutToolsFromTutorial = useCallback(() => {
    setTutorialOpen(false);
    setToolsOpen(true);
  }, []);

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
        <DashboardGridLayout
          className="dashboard__grid"
          layout={activeLayout}
          cols={getColumnCount(currentBreakpoint)}
          rowHeight={ROW_HEIGHT}
          margin={MARGIN}
          containerPadding={MARGIN}
          isDraggable={editMode}
          isResizable={editMode}
          onLayoutChange={handleLayoutChange}
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
                  applyDashboardDocUpdate((prev) => ({
                    ...prev,
                    items: prev.items.map((candidate) => {
                      if (candidate.instance_id !== instanceId) return candidate;
                      return {
                        ...candidate,
                        settings: { ...(candidate.settings || {}), ...newSettings },
                      };
                    }),
                  }));
                }}
                onRemove={() => handleWidgetRemove(item.instance_id)}
              />
            </div>
          ))}
        </DashboardGridLayout>
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
              className="dashboard__fab dashboard__fab--tools"
              onClick={() => setToolsOpen(true)}
              title="Layout tools"
            >
              <FontAwesomeIcon icon={faSliders} />
            </button>
            <button
              className="dashboard__fab dashboard__fab--secondary"
              onClick={undo}
              title="Undo"
              disabled={!canUndo}
            >
              <FontAwesomeIcon icon={faRotateLeft} />
            </button>
            <button
              className="dashboard__fab dashboard__fab--secondary"
              onClick={redo}
              title="Redo"
              disabled={!canRedo}
            >
              <FontAwesomeIcon icon={faRotateRight} />
            </button>
            <button
              className="dashboard__fab dashboard__fab--add"
              onClick={() => dispatchUi({ type: 'set-add-modal-open', value: true })}
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

      {/* Add widget modal */}
      <AddWidgetModal
        open={addModalOpen}
        onClose={() => dispatchUi({ type: 'set-add-modal-open', value: false })}
        catalog={catalog}
        onAdd={handleAddWidget}
      />

      {/* Widget settings modal */}
      <WidgetSettingsModal
        open={settingsModalOpen}
        onClose={() => {
          dispatchUi({ type: 'set-settings-modal-open', value: false });
          dispatchUi({ type: 'set-selected-widget-id', value: null });
        }}
        widgetItem={selectedWidget}
        onSave={(instanceId, newSettings) => {
          applyDashboardDocUpdate((prev) => ({
            ...prev,
            items: prev.items.map((candidate) => {
              if (candidate.instance_id !== instanceId) return candidate;
              return {
                ...candidate,
                settings: { ...(candidate.settings || {}), ...newSettings },
              };
            }),
          }));
          dispatchUi({ type: 'set-settings-modal-open', value: false });
          dispatchUi({ type: 'set-selected-widget-id', value: null });
        }}
        onRemove={(instanceId) => {
          handleWidgetRemove(instanceId);
          dispatchUi({ type: 'set-settings-modal-open', value: false });
          dispatchUi({ type: 'set-selected-widget-id', value: null });
        }}
      />

      <DashboardLayoutToolsModal
        open={toolsOpen}
        onClose={() => setToolsOpen(false)}
        currentMode={currentBreakpoint}
        importSourceMode={importSourceMode}
        onImportSourceModeChange={setImportSourceMode}
        onImportIntoCurrent={importIntoCurrent}
        autoLayoutReorganize={autoLayoutOptions.reorganize}
        autoLayoutGrowWidths={autoLayoutOptions.growWidths}
        autoLayoutShrinkWidths={autoLayoutOptions.shrinkWidths}
        autoLayoutGrowHeights={autoLayoutOptions.growHeights}
        autoLayoutShrinkHeights={autoLayoutOptions.shrinkHeights}
        onAutoLayoutOptionChange={(key, value) => setAutoLayoutOptions((prev) => ({ ...prev, [key]: value }))}
        onAutoLayoutCurrent={autoLayoutCurrent}
      />

      <BaseModal
        open={tutorialOpen}
        onClose={() => dismissTutorial(false)}
        backdropClassName="widget-settings__backdrop"
        dialogClassName="widget-settings-modal dashboard-tutorial-modal"
        closeAriaLabel="Close dashboard tutorial"
      >
        <div className="widget-settings__header dashboard-tutorial-modal__header">
          <div className="widget-settings__icon dashboard-tutorial-modal__badge">
            <FontAwesomeIcon icon={faWandMagicSparkles} />
          </div>
          <div className="widget-settings__header-text">
            <h2 className="widget-settings__title">Welcome to dashboard editing</h2>
            <span className="widget-settings__type">A quick interactive tour before you start rearranging widgets.</span>
          </div>
        </div>
        <div className="widget-settings__content dashboard-tutorial-modal__content">
          <div className="dashboard-tutorial-modal__steps">
            <div className="dashboard-tutorial-modal__step">
              <div className="dashboard-tutorial-modal__step-icon dashboard-tutorial-modal__step-icon--accent">
                <FontAwesomeIcon icon={faPen} />
              </div>
              <div>
                <h3>Move with confidence</h3>
                <p>Drag widgets to reorder them and resize them from any edge without leaving edit mode.</p>
              </div>
            </div>
            <div className="dashboard-tutorial-modal__step">
              <div className="dashboard-tutorial-modal__step-icon dashboard-tutorial-modal__step-icon--info">
                <FontAwesomeIcon icon={faSliders} />
              </div>
              <div>
                <h3>Use layout tools</h3>
                <p>Import another saved column mode or auto-layout the current one with repacking plus optional grow and shrink adjustments for widths and heights.</p>
              </div>
            </div>
            <div className="dashboard-tutorial-modal__step">
              <div className="dashboard-tutorial-modal__step-icon dashboard-tutorial-modal__step-icon--success">
                <FontAwesomeIcon icon={faRotateLeft} />
              </div>
              <div>
                <h3>Undo is always nearby</h3>
                <p>Undo and redo stay available from the floating controls and from Ctrl/Cmd+Z while editing.</p>
              </div>
            </div>
          </div>
          <div className="dashboard-tutorial-modal__hint">
            <FontAwesomeIcon icon={faCircleInfo} />
            <span>Need to remove something fast? Drop it on the trash bar at the bottom.</span>
          </div>
          <div className="dashboard-tutorial-modal__actions">
            <button className="widget-settings__btn widget-settings__btn--neutral dashboard-tools-modal__button dashboard-tools-modal__button--secondary" onClick={() => dismissTutorial(true)}>
              Do not show again
            </button>
            <button className="widget-settings__btn widget-settings__btn--neutral dashboard-tools-modal__button dashboard-tools-modal__button--secondary" onClick={() => dismissTutorial(false)}>
              Skip
            </button>
            <button className="widget-settings__btn widget-settings__btn--save dashboard-tools-modal__button" onClick={openLayoutToolsFromTutorial}>
              <FontAwesomeIcon icon={faArrowRight} />
              <span>Open layout tools</span>
            </button>
          </div>
        </div>
      </BaseModal>
    </div>
  );
}
