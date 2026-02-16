import React, { useEffect, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faArrowLeft, faPen, faPlay, faPlus } from '@fortawesome/free-solid-svg-icons';
import { useParams } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import PageHeader from '../common/PageHeader/PageHeader';
import Snackbar from '../common/Snackbar/Snackbar';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import LoadingView from '../common/LoadingView/LoadingView';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassPill from '../common/GlassPill/GlassPill';
import Button from '../common/Button/Button';
import SearchBar from '../common/SearchBar/SearchBar';
import useEditorHistory from './hooks/useEditorHistory';
import useRunStream from './hooks/useRunStream';
import useAutomationLists from './hooks/useAutomationLists';
import useAutomationDeviceSelectors from './hooks/useAutomationDeviceSelectors';
import useAutomationCanvas from './hooks/useAutomationCanvas';
import useAutomationConnectMode from './hooks/useAutomationConnectMode';
import useAutomationEditorLoader from './hooks/useAutomationEditorLoader';
import useAutomationHotkeys from './hooks/useAutomationHotkeys';
import useAutomationWorkflowActions from './hooks/useAutomationWorkflowActions';
import useErsInventory from '../../hooks/useErsInventory';
import useDeviceHubDevices from '../../hooks/useDeviceHubDevices';
import {
  createWorkflow,
  deleteWorkflow,
  disableWorkflow,
  enableWorkflow,
  getRun,
  runWorkflow,
  updateWorkflow,
} from '../../services/automationService';
import { listUsers } from '../../services/authService';
import {
  buildDefinitionFromEditor,
  defaultNodeData,
  defaultWorkflowName,
  editorSnapshotForSave,
  groupPaletteItems,
  isTriggerNode,
  nodeBodyText,
  nodeSubtitle,
  nodeTitle,
  parseWorkflowIntoEditor,
} from './definition';
import { iconForNodeKind } from './automationIcons';
import { zoomAroundPoint } from './automationUtils';
import { computeEdgesToRender, computeSvgWorldSize } from './automationCanvasSelectors';
import AutomationTopbar from './components/AutomationTopbar';
import AutomationLeftPanel from './components/AutomationLeftPanel';
import AutomationCanvas from './components/AutomationCanvas';
import AutomationPropertiesPanel from './components/AutomationPropertiesPanel';
import AutomationRuns from './components/AutomationRuns';
import './Automation.css';

const NODE_WIDTH = 260;
const NODE_HEADER_HEIGHT = 40;
const GRID_SIZE = 28;

function defaultEditorState() {
  return {
    workflowName: defaultWorkflowName(),
    nodes: [],
    edges: [],
  };
}

function Automation() {
  const { workflowId: workflowIdParam } = useParams();
  const { accessToken, user, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const isAdmin = user?.role === 'admin';
  const currentUserId = user?.id || user?.user_id || user?.sub || '';

  const [viewMode, setViewMode] = useState(() => (workflowIdParam ? 'edit' : 'overview'));
  const isEditMode = viewMode === 'edit';
  const [isNarrow, setIsNarrow] = useState(false);
  const [hasOverviewSelection, setHasOverviewSelection] = useState(false);
  const [workflowSearch, setWorkflowSearch] = useState('');

  const [err, setErr] = useState('');
  const [toast, setToast] = useState('');

  const [userOptions, setUserOptions] = useState([]);
  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      if (!accessToken) return;
      if (!isResidentOrAdmin) return;

      // Residents can only notify themselves.
      if (!isAdmin) {
        const label = user?.email
          ? `${user.email}${user.user_name ? ` (${user.user_name})` : ''}`
          : (user?.user_name || 'Me');
        if (!cancelled && currentUserId) {
          setUserOptions([{ id: String(currentUserId), label }]);
        }
        return;
      }

      const res = await listUsers({ page: 1, pageSize: 100 }, accessToken);
      if (cancelled) return;
      if (res?.success) {
        const users = res.data?.users || [];
        const opts = (Array.isArray(users) ? users : [])
          .map((u) => {
            const id = String(u?.id || '').trim();
            if (!id) return null;
            const name = [u?.first_name, u?.last_name].map((x) => String(x || '').trim()).filter(Boolean).join(' ');
            const base = name || String(u?.user_name || '').trim() || String(u?.email || '').trim() || id;
            const suffix = u?.email ? ` · ${u.email}` : '';
            return { id, label: `${base}${suffix}` };
          })
          .filter(Boolean);
        setUserOptions(opts);
      } else {
        setUserOptions([]);
      }
    };

    load();
    return () => {
      cancelled = true;
    };
  }, [accessToken, isAdmin, isResidentOrAdmin, currentUserId, user?.email, user?.user_name, user?.first_name, user?.last_name]);

  const {
    loading,
    workflows,
    selectedId,
    setSelectedId,
    selectedWorkflow,
    fetchWorkflows,
    upsertWorkflowInList,
    removeWorkflowFromList,
    runs,
    runsLoading,
    runsLimit,
    setRunsLimit,
    runsHasMore,
    fetchRuns,
    refreshAllData,
  } = useAutomationLists({ accessToken, onError: setErr });

  const filteredWorkflows = useMemo(() => {
    const list = Array.isArray(workflows) ? workflows : [];
    const query = String(workflowSearch || '').trim().toLowerCase();
    if (!query) return list;
    return list.filter((wf) => {
      const name = String(wf?.name || '').toLowerCase();
      const id = String(wf?.id || '').toLowerCase();
      return name.includes(query) || id.includes(query);
    });
  }, [workflows, workflowSearch]);

  // Allow deep-linking to a specific workflow.
  useEffect(() => {
    const id = String(workflowIdParam || '').trim();
    if (!id) return;
    if (selectedId === id) return;
    setSelectedId(id);
  }, [workflowIdParam, selectedId, setSelectedId]);

  useEffect(() => {
    if (workflowIdParam) setViewMode('edit');
  }, [workflowIdParam]);

  useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    const media = window.matchMedia('(max-width: 720px)');
    const handleChange = () => setIsNarrow(media.matches);
    handleChange();
    if (media.addEventListener) {
      media.addEventListener('change', handleChange);
      return () => media.removeEventListener('change', handleChange);
    }
    media.addListener(handleChange);
    return () => media.removeListener(handleChange);
  }, []);

  const {
    devices: realtimeDevices,
    loading: realtimeLoading,
    error: realtimeError,
  } = useDeviceHubDevices({ enabled: Boolean(isResidentOrAdmin), metadataMode: 'rest' });

  const {
    devices: ersMergedDevices,
    loading: ersInventoryLoading,
    error: ersInventoryError,
    refresh: refreshErsInventory,
  } = useErsInventory({ enabled: Boolean(isResidentOrAdmin && accessToken), accessToken, realtimeDevices });

  const mergedDevicesLoading = Boolean(realtimeLoading || ersInventoryLoading);

  useEffect(() => {
    if (!realtimeError) return;
    setErr(realtimeError);
  }, [realtimeError]);

  useEffect(() => {
    if (!ersInventoryError) return;
    setErr(ersInventoryError);
  }, [ersInventoryError]);

  const refreshAllDataWithInventory = async () => {
    await refreshAllData();
    await refreshErsInventory?.();
  };

  const [selectedNodeId, setSelectedNodeId] = useState('workflow'); // workflow|nodeId
  const {
    editor,
    setEditor,
    editorRef,
    resetHistory,
    applyEditorUpdate,
    applyEditorUpdateBatched,
    canUndo,
    canRedo,
    undo,
    redo,
    commitExternalSnapshot,
  } = useEditorHistory({
    initialEditor: defaultEditorState,
    snapshotForSave: editorSnapshotForSave,
  });

  const { liveRunNodeStates, clearLiveRunHighlights, closeRunWs, startRunStream } = useRunStream({
    onToast: setToast,
    onError: setErr,
  });

  const [lastSavedSnapshot, setLastSavedSnapshot] = useState('');
  const isDirty = useMemo(() => {
    try {
      if (!selectedWorkflow) {
        // New/unsaved workflow: treat as dirty if it has any meaningful content.
        const name = String(editor.workflowName || '').trim();
        const nodes = Array.isArray(editor.nodes) ? editor.nodes : [];
        const edges = Array.isArray(editor.edges) ? editor.edges : [];
        const hasAny = !!name || nodes.length > 0 || edges.length > 0;
        return hasAny;
      }
      return editorSnapshotForSave(editor) !== lastSavedSnapshot;
    } catch {
      return true;
    }
  }, [editor, selectedWorkflow, lastSavedSnapshot]);

  const canvasRef = useRef(null);

  const {
    viewport,
    setViewport,
    canvasSize,
    onPaletteDragStart,
    onCanvasDragOver,
    onCanvasDrop,
    onNodePointerDown,
    beginPan,
    addNodeAtCenter,
    onCanvasPointerDown: handleCanvasPointerDown,
    onCanvasPointerMove: handleCanvasPointerMove,
    onCanvasPointerUp: handleCanvasPointerUp,
  } = useAutomationCanvas({
    canvasRef,
    editor,
    editorRef,
    setEditor,
    commitExternalSnapshot,
    applyEditorUpdate,
    setSelectedNodeId,
    defaultNodeData,
    nodeWidth: NODE_WIDTH,
    nodeHeaderHeight: NODE_HEADER_HEIGHT,
  });

  const {
    connectModeRef,
    connectMode,
    setConnectMode,
    connectHoverId,
    setConnectHoverId,
    cancelConnect,
    commitConnection,
    startConnectFromNode,
  } = useAutomationConnectMode({
    editorNodes: editor.nodes,
    viewport,
    canvasRef,
    applyEditorUpdate,
    isTriggerNode,
    nodeWidth: NODE_WIDTH,
    nodeHeaderHeight: NODE_HEADER_HEIGHT,
    setSelectedNodeId,
  });

  useEffect(() => {
    if (!isEditMode && connectMode) cancelConnect();
  }, [cancelConnect, connectMode, isEditMode]);

  const selectedNode = useMemo(() => {
    if (!selectedNodeId || selectedNodeId === 'workflow') return null;
    const nodes = Array.isArray(editor.nodes) ? editor.nodes : [];
    return nodes.find(n => n.id === selectedNodeId) || null;
  }, [editor.nodes, selectedNodeId]);

  const selectedConnections = useMemo(() => {
    const id = String(selectedNodeId || '');
    if (!id || id === 'workflow') return { incomingCount: 0, outgoingCount: 0 };
    const edges = Array.isArray(editor.edges) ? editor.edges : [];
    return {
      incomingCount: edges.filter(e => e && String(e.to) === id).length,
      outgoingCount: edges.filter(e => e && String(e.from) === id).length,
    };
  }, [editor.edges, selectedNodeId]);

  const disconnectIncoming = () => {
    const id = String(selectedNodeId || '');
    if (!id || id === 'workflow') return;
    applyEditorUpdate(prev => {
      const edges = Array.isArray(prev.edges) ? prev.edges : [];
      return { ...prev, edges: edges.filter(e => String(e?.to) !== id) };
    });
  };

  const disconnectOutgoing = () => {
    const id = String(selectedNodeId || '');
    if (!id || id === 'workflow') return;
    applyEditorUpdate(prev => {
      const edges = Array.isArray(prev.edges) ? prev.edges : [];
      return { ...prev, edges: edges.filter(e => String(e?.from) !== id) };
    });
  };

  const deleteEdge = (fromId, toId) => {
    const from = String(fromId || '').trim();
    const to = String(toId || '').trim();
    if (!from || !to) return;
    applyEditorUpdate(prev => {
      const edges = Array.isArray(prev.edges) ? prev.edges : [];
      return { ...prev, edges: edges.filter(e => !(String(e?.from) === from && String(e?.to) === to)) };
    });
  };

  const deleteSelectedNode = () => {
    if (!selectedNode || selectedNodeId === 'workflow') return;
    const id = String(selectedNode.id || '');
    if (!id) return;

    cancelConnect();
    applyEditorUpdate(prev => {
      const nodes = Array.isArray(prev.nodes) ? prev.nodes : [];
      const edges = Array.isArray(prev.edges) ? prev.edges : [];
      return {
        ...prev,
        nodes: nodes.filter(n => String(n?.id) !== id),
        edges: edges.filter(e => String(e?.from) !== id && String(e?.to) !== id),
      };
    });
    setSelectedNodeId('workflow');
  };

  useAutomationEditorLoader({
    selectedId,
    workflows,
    parseWorkflowIntoEditor,
    defaultEditorState,
    setEditor,
    setSelectedNodeId,
    resetHistory,
    setLastSavedSnapshot,
    editorSnapshotForSave,
  });

  useAutomationHotkeys({
    selectedNodeId,
    undo,
    redo,
    deleteSelectedNode,
    enabled: isEditMode,
  });


  const { deviceOptions, deviceNameById, triggerKeyOptions } = useAutomationDeviceSelectors({
    devices: ersMergedDevices,
    selectedNode,
  });

  const clearCanvas = () => {
    applyEditorUpdate(prev => ({
      ...prev,
      nodes: [],
      edges: [],
    }));
    setSelectedNodeId('workflow');
  };

  const {
    saving,
    lastSavedAt,
    startNewWorkflow,
    saveWorkflowInternal,
    toggleEnabled,
    removeWorkflow,
  } = useAutomationWorkflowActions({
    accessToken,
    selectedWorkflow,
    editor,
    isDirty,
    buildDefinitionFromEditor,
    editorSnapshotForSave,
    defaultEditorState,
    createWorkflow,
    updateWorkflow,
    enableWorkflow,
    disableWorkflow,
    deleteWorkflow,
    setSelectedId,
    upsertWorkflowInList,
    removeWorkflowFromList,
    fetchWorkflows,
    setEditor,
    setSelectedNodeId,
    resetHistory,
    clearLiveRunHighlights,
    closeRunWs,
    setLastSavedSnapshot,
    setToast,
    setErr,
  });

  const runNow = async () => {
    if (!accessToken || !selectedWorkflow) return;
    if (saving) return;
    if (isDirty) {
      const saved = await saveWorkflowInternal({ silent: true });
      if (!saved) {
        setErr('Unable to save changes');
        return;
      }
    }
    setErr('');
    clearLiveRunHighlights();
    closeRunWs();
    setToast('Running…');
    const res = await runWorkflow(selectedWorkflow.id, accessToken);
    if (res.success) {
      const runId = String(res.data?.run_id || '').trim();
      if (!runId) {
        setToast('Run started');
        // No run_id means we can't stream; fall back to runs list.
        fetchRuns(selectedWorkflow.id, 5);
        return;
      }

      startRunStream({
        runId,
        accessToken,
        workflowId: selectedWorkflow.id,
        refreshRuns: (workflowId) => fetchRuns(workflowId, 5),
        getRun,
      });
    } else {
      clearLiveRunHighlights();
      closeRunWs();
      setErr(res.error || 'Failed to run workflow');
    }
  };

  const canExecuteFromNode = !!selectedWorkflow && !saving && !loading;
  const executeFromNodeTitle = !selectedWorkflow ? 'Create the workflow first' : 'Execute (Run)';

  const edgesToRender = useMemo(() => {
    return computeEdgesToRender({
      nodes: editor.nodes,
      edges: editor.edges,
      nodeWidth: NODE_WIDTH,
      nodeHeaderHeight: NODE_HEADER_HEIGHT,
    });
  }, [editor.nodes, editor.edges]);

  const svgWorldSize = useMemo(() => {
    return computeSvgWorldSize({
      nodes: editor.nodes,
      canvasSize: { width: canvasSize.width, height: canvasSize.height },
      viewportScale: viewport.scale,
      nodeWidth: NODE_WIDTH,
      nodeHeaderHeight: NODE_HEADER_HEIGHT,
    });
  }, [editor.nodes, canvasSize.width, canvasSize.height, viewport.scale]);

  const onCanvasPointerDown = (e) => {
    handleCanvasPointerDown(e);
    const canvasEl = canvasRef.current;
    if (!canvasEl) return;
    if (e.button !== 0) return;
    if (isEditMode && e.target?.closest?.('.automation-node')) return;

    if (isEditMode && connectMode && connectMode.mode === 'click') {
      cancelConnect();
      return;
    }

    beginPan(e);
  };

  const paletteGroups = useMemo(() => groupPaletteItems(), []);
  const showOverviewDetail = !isNarrow || hasOverviewSelection;

  if (!isResidentOrAdmin) {
    if (bootstrapping) {
      return <LoadingView title="Automation" message="Loading automation…" />;
    }
    return (
      <UnauthorizedView
        title="Automation"
        message="You do not have permission to view this page."
      />
    );
  }

  return (
    <div className={`automation-page${!isEditMode && !isNarrow ? ' automation-page--overview-desktop' : ''}`}>
      <PageHeader
        title="Automation"
        subtitle={`Build workflows by dragging nodes onto a canvas · ${Array.isArray(workflows) ? workflows.length : 0} workflows`}
      >
        <div className="automation-header-actions" />
      </PageHeader>

      {err && <div className="alert error" role="alert">{err}</div>}

      {isEditMode ? (
        <div className="automation-layout">
          <div className="automation-editor-shell fade-in" key="automation-editor">
            {isNarrow && (
              <div className="automation-editor-back">
                <GlassPill
                  icon={faArrowLeft}
                  text="Back"
                  tone="default"
                  className="page-header-back-pill"
                  onClick={() => {
                    setViewMode('overview');
                    setHasOverviewSelection(Boolean(selectedWorkflow));
                  }}
                />
              </div>
            )}
            <AutomationTopbar
              workflows={workflows}
              selectedId={selectedId}
              onSelectId={setSelectedId}
              startNewWorkflow={startNewWorkflow}
              selectedWorkflow={selectedWorkflow}
              saving={saving}
              lastSavedAt={lastSavedAt}
              loading={loading}
              devicesLoading={mergedDevicesLoading}
              refreshAllData={refreshAllDataWithInventory}
              clearCanvas={clearCanvas}
              canUndo={canUndo}
              undo={undo}
              canRedo={canRedo}
              redo={redo}
              toggleEnabled={toggleEnabled}
              runNow={runNow}
              removeWorkflow={removeWorkflow}
              isAdmin={isAdmin}
            />

            <div className="automation-editor">
              <AutomationLeftPanel
                paletteGroups={paletteGroups}
                iconForNodeKind={iconForNodeKind}
                onPaletteDragStart={onPaletteDragStart}
                addNodeAtCenter={addNodeAtCenter}
              />

              <AutomationCanvas
                canvasRef={canvasRef}
                onCanvasDragOver={onCanvasDragOver}
                onCanvasDrop={onCanvasDrop}
                onCanvasPointerDown={onCanvasPointerDown}
                onCanvasPointerMove={handleCanvasPointerMove}
                onCanvasPointerUp={handleCanvasPointerUp}
                onCanvasPointerCancel={handleCanvasPointerUp}
                GRID_SIZE={GRID_SIZE}
                viewport={viewport}
                setViewport={setViewport}
                svgWorldSize={svgWorldSize}
                edgesToRender={edgesToRender}
                connectMode={connectMode}
                connectHoverId={connectHoverId}
                setConnectHoverId={setConnectHoverId}
                connectModeRef={connectModeRef}
                setConnectMode={setConnectMode}
                cancelConnect={cancelConnect}
                deleteEdge={deleteEdge}
                editorNodes={editor.nodes}
                selectedNodeId={selectedNodeId}
                setSelectedNodeId={setSelectedNodeId}
                NODE_WIDTH={NODE_WIDTH}
                NODE_HEADER_HEIGHT={NODE_HEADER_HEIGHT}
                isTriggerNode={isTriggerNode}
                nodeTitle={nodeTitle}
                nodeSubtitle={nodeSubtitle}
                nodeBodyText={nodeBodyText}
                iconForNodeKind={iconForNodeKind}
                deviceNameById={deviceNameById}
                liveRunNodeStates={liveRunNodeStates}
                commitConnection={commitConnection}
                startConnectFromNode={startConnectFromNode}
                onNodePointerDown={onNodePointerDown}
                executeFromNodeTitle={executeFromNodeTitle}
                canExecuteFromNode={canExecuteFromNode}
                runNow={runNow}
                canvasSize={canvasSize}
                zoomAroundPoint={zoomAroundPoint}
                workflowName={editor.workflowName}
                onWorkflowNameChange={(name) => applyEditorUpdateBatched('workflow-name', prev => ({ ...prev, workflowName: name }))}
              />

              <AutomationPropertiesPanel
                selectedNodeId={selectedNodeId}
                selectedNode={selectedNode}
                selectedConnections={selectedConnections}
                isTriggerNode={isTriggerNode}
                defaultNodeData={defaultNodeData}
                applyEditorUpdate={applyEditorUpdate}
                applyEditorUpdateBatched={applyEditorUpdateBatched}
                deviceOptions={deviceOptions}
                triggerKeyOptions={triggerKeyOptions}
                userOptions={userOptions}
                isAdmin={isAdmin}
                currentUserId={currentUserId}
                disconnectIncoming={disconnectIncoming}
                disconnectOutgoing={disconnectOutgoing}
                deleteSelectedNode={deleteSelectedNode}
              />
            </div>
          </div>

          <AutomationRuns
            selectedWorkflow={selectedWorkflow}
            runs={runs}
            runsLoading={runsLoading}
            runsLimit={runsLimit}
            setRunsLimit={setRunsLimit}
            runsHasMore={runsHasMore}
            isNarrow={isNarrow}
            fetchRuns={fetchRuns}
          />
        </div>
      ) : (
        <div className={`automation-overview-layout${showOverviewDetail ? '' : ' automation-overview-layout--list-only'}`}>
          {(!isNarrow || !hasOverviewSelection) && (
            <div className="automation-overview-list-column">
              <GlassCard interactive={false} className="automation-overview-list">
              <div className="automation-overview-list-header">
                <div className="automation-overview-list-meta">
                  <div className="automation-overview-title">Workflows</div>
                  <div className="automation-overview-count-pill">
                    {workflowSearch.trim()
                      ? `${filteredWorkflows.length} of ${Array.isArray(workflows) ? workflows.length : 0}`
                      : `${Array.isArray(workflows) ? workflows.length : 0} total`}
                  </div>
                </div>
                <SearchBar
                  value={workflowSearch}
                  onChange={setWorkflowSearch}
                  onClear={() => setWorkflowSearch('')}
                  placeholder="Search workflows…"
                  ariaLabel="Search workflows"
                  className="automation-overview-search"
                />
              </div>

              {Array.isArray(filteredWorkflows) && filteredWorkflows.length ? (
                <div className="automation-workflow-list">
                  {filteredWorkflows.map((wf) => (
                    <button
                      key={wf.id}
                      type="button"
                      className={`automation-workflow-item${!isNarrow && selectedId === wf.id ? ' selected' : ''}`}
                      onClick={() => {
                        setSelectedId(wf.id);
                        setViewMode('overview');
                        setHasOverviewSelection(true);
                      }}
                    >
                      <div className="name">{wf.name}</div>
                      <div className="meta">
                        <span className={`badge ${wf.enabled ? 'success' : 'muted'}`}>{wf.enabled ? 'Enabled' : 'Disabled'}</span>
                        <span className="muted">{wf.updated_at ? `Updated ${new Date(wf.updated_at).toLocaleDateString()}` : 'No updates yet'}</span>
                      </div>
                    </button>
                  ))}
                  <button
                    type="button"
                    className="automation-workflow-item automation-workflow-item--add"
                    onClick={() => {
                      startNewWorkflow();
                      setViewMode('edit');
                    }}
                  >
                    <span className="automation-workflow-add-icon">
                      <FontAwesomeIcon icon={faPlus} />
                    </span>
                    <span className="automation-workflow-add-title">Add new workflow</span>
                    <span className="automation-workflow-add-subtitle">Create and start editing</span>
                    <span className="automation-workflow-add-cta">Open editor</span>
                  </button>
              </div>
              ) : (
                <div className="automation-workflow-list">
                  <div className="muted">
                    {workflowSearch.trim()
                      ? 'No workflows match your search.'
                      : 'No workflows yet. Create one to get started.'}
                  </div>
                  <button
                    type="button"
                    className="automation-workflow-item automation-workflow-item--add"
                    onClick={() => {
                      startNewWorkflow();
                      setViewMode('edit');
                    }}
                  >
                    <span className="automation-workflow-add-icon">
                      <FontAwesomeIcon icon={faPlus} />
                    </span>
                    <span className="automation-workflow-add-title">Add new workflow</span>
                    <span className="automation-workflow-add-subtitle">Create and start editing</span>
                    <span className="automation-workflow-add-cta">Open editor</span>
                  </button>
                </div>
              )}
              </GlassCard>
            </div>
          )}

          {showOverviewDetail && (
            <div className="automation-overview-preview-column">
              {isNarrow && (
                <div className="automation-overview-back">
                  <GlassPill
                    icon={faArrowLeft}
                    text="Back"
                    tone="default"
                    className="page-header-back-pill"
                    onClick={() => setHasOverviewSelection(false)}
                  />
                </div>
              )}
              <GlassCard interactive={false} className="automation-overview-preview">
                <div className="automation-overview-preview-header">
                  <div className="automation-overview-preview-meta">
                    <div className="automation-overview-preview-title">{selectedWorkflow ? selectedWorkflow.name : 'Preview'}</div>
                    <div className="automation-overview-preview-status-row">
                      {selectedWorkflow ? (
                        <span className={`automation-overview-status-pill ${selectedWorkflow.enabled ? 'is-enabled' : 'is-disabled'}`}>
                          {selectedWorkflow.enabled ? 'Enabled' : 'Disabled'}
                        </span>
                      ) : (
                        <span className="muted">Select a workflow to preview</span>
                      )}
                    </div>
                  </div>
                  <div className="automation-overview-preview-actions">
                    <Button
                      onClick={() => setViewMode('edit')}
                      className="automation-overview-edit"
                    >
                      <span className="btn-icon"><FontAwesomeIcon icon={faPen} /></span>
                      <span className="btn-label">Edit</span>
                    </Button>
                    {selectedWorkflow && (
                      <Button
                        variant="secondary"
                        onClick={runNow}
                        disabled={saving}
                      >
                        <span className="btn-icon"><FontAwesomeIcon icon={faPlay} /></span>
                        <span className="btn-label">Run</span>
                      </Button>
                    )}
                  </div>
                </div>

                <div className="automation-overview-preview-body">
                  {selectedWorkflow ? (
                    <>
                      <AutomationCanvas
                        canvasRef={canvasRef}
                        onCanvasPointerDown={onCanvasPointerDown}
                        onCanvasPointerMove={handleCanvasPointerMove}
                        onCanvasPointerUp={handleCanvasPointerUp}
                        onCanvasPointerCancel={handleCanvasPointerUp}
                        GRID_SIZE={GRID_SIZE}
                        viewport={viewport}
                        setViewport={setViewport}
                        svgWorldSize={svgWorldSize}
                        edgesToRender={edgesToRender}
                        connectMode={null}
                        connectHoverId={null}
                        setConnectHoverId={() => {}}
                        connectModeRef={connectModeRef}
                        setConnectMode={() => {}}
                        cancelConnect={() => {}}
                        deleteEdge={() => {}}
                        editorNodes={editor.nodes}
                        selectedNodeId={selectedNodeId}
                        setSelectedNodeId={setSelectedNodeId}
                        NODE_WIDTH={NODE_WIDTH}
                        NODE_HEADER_HEIGHT={NODE_HEADER_HEIGHT}
                        isTriggerNode={isTriggerNode}
                        nodeTitle={nodeTitle}
                        nodeSubtitle={nodeSubtitle}
                        nodeBodyText={nodeBodyText}
                        iconForNodeKind={iconForNodeKind}
                        deviceNameById={deviceNameById}
                        liveRunNodeStates={liveRunNodeStates}
                        commitConnection={() => {}}
                        startConnectFromNode={() => {}}
                        onNodePointerDown={() => {}}
                        executeFromNodeTitle={executeFromNodeTitle}
                        canExecuteFromNode={canExecuteFromNode}
                        runNow={runNow}
                        canvasSize={canvasSize}
                        zoomAroundPoint={zoomAroundPoint}
                        workflowName={editor.workflowName}
                        onWorkflowNameChange={(name) => applyEditorUpdateBatched('workflow-name', prev => ({ ...prev, workflowName: name }))}
                        readOnly
                      />
                      {!isNarrow && (
                        <div className="automation-overview-next-section" aria-hidden="true">
                          <span className="automation-overview-next-title">Recent runs</span>
                          <span className="automation-overview-next-hint">Scroll down ↓</span>
                        </div>
                      )}
                    </>
                  ) : (
                    <div className="automation-overview-empty muted">Pick a workflow to see its canvas preview.</div>
                  )}
                </div>
              </GlassCard>

              <AutomationRuns
                selectedWorkflow={selectedWorkflow}
                runs={runs}
                runsLoading={runsLoading}
                runsLimit={runsLimit}
                setRunsLimit={setRunsLimit}
                runsHasMore={runsHasMore}
                isNarrow={isNarrow}
                fetchRuns={fetchRuns}
              />
            </div>
          )}
        </div>
      )}

      {!isEditMode && (
        <button
          type="button"
          className="devices-fab"
          onClick={() => {
            startNewWorkflow();
            setViewMode('edit');
          }}
          aria-label="Add workflow"
          title="Add workflow"
        >
          <span className="devices-fab-icon">
            <FontAwesomeIcon icon={faPlus} />
          </span>
          <span className="devices-fab-label">Add workflow</span>
        </button>
      )}

      <Snackbar message={toast} onClose={() => setToast('')} />
    </div>
  );
}

export default Automation;
