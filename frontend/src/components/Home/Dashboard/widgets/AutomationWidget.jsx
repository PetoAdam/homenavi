import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faPlay,
  faCheck,
  faXmark,
  faSpinner,
  faBolt,
  faQuestionCircle,
} from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../../context/AuthContext';
import { listWorkflows, runWorkflow, getRun } from '../../../../services/automationService';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import GlassPill from '../../../common/GlassPill/GlassPill';
import ProgressBar from '../../../common/ProgressBar/ProgressBar';
import Button from '../../../common/Button/Button';
import './AutomationWidget.css';

const POLL_INTERVAL = 1000;
const MAX_POLLS = 60;

export default function AutomationWidget({
  // instanceId is unused (widget is identified by settings)
  settings = {},
  // enabled is unused (widget controls its own rendering)
  editMode,
  onSettings,
  onRemove,
}) {
  const navigate = useNavigate();
  const { accessToken, user } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const [workflows, setWorkflows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [status, setStatus] = useState(null);
  const [running, setRunning] = useState(false);
  const [runResult, setRunResult] = useState(null);
  const [runProgress, setRunProgress] = useState(0);

  const pollTimeoutRef = useRef(null);
  const runTokenRef = useRef(0);
  const wsRef = useRef(null);
  
  // Track if user is dragging to prevent navigation (must be with other hooks)
  const [isDragging, setIsDragging] = useState(false);
  const dragStartPos = useRef({ x: 0, y: 0 });

  const workflowId = settings.workflow_id || '';

  useEffect(() => {
    return () => {
      if (pollTimeoutRef.current) {
        clearTimeout(pollTimeoutRef.current);
        pollTimeoutRef.current = null;
      }
      try {
        wsRef.current?.close();
      } catch {
        // ignore
      }
      wsRef.current = null;
    };
  }, []);

  // Fetch workflows
  useEffect(() => {
    let cancelled = false;

    const fetchWorkflows = async () => {
      if (!accessToken || !isResidentOrAdmin) {
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);
      setStatus(null);

      try {
        const res = await listWorkflows(accessToken);

        if (cancelled) return;

        if (!res.success) {
          if (res.status === 401) {
            setStatus(401);
          } else if (res.status === 403) {
            setStatus(403);
          } else {
            setError(res.error || 'Failed to load workflows');
          }
        } else {
          const wfs = Array.isArray(res.data) ? res.data : (res.data?.workflows || []);
          // Filter to workflows with manual trigger
          const manualTriggerWorkflows = wfs.filter((wf) => {
            const def = wf.definition || {};
            const nodes = def.nodes || [];
            return nodes.some((n) => n.kind === 'trigger.manual' || n.type === 'trigger.manual');
          });
          setWorkflows(manualTriggerWorkflows);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err.message || 'Failed to load workflows');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchWorkflows();

    return () => {
      cancelled = true;
    };
  }, [accessToken, isResidentOrAdmin]);

  // Find selected workflow
  const selectedWorkflow = useMemo(() => {
    if (!workflowId) return null;
    return workflows.find((wf) => wf.id === workflowId) || null;
  }, [workflowId, workflows]);

  // Run workflow
  const handleRun = useCallback(async () => {
    if (editMode || !selectedWorkflow || running) return;

    // Cancel any previous poll loop
    runTokenRef.current += 1;
    const token = runTokenRef.current;
    if (pollTimeoutRef.current) {
      clearTimeout(pollTimeoutRef.current);
      pollTimeoutRef.current = null;
    }
    try {
      wsRef.current?.close();
    } catch {
      // ignore
    }
    wsRef.current = null;

    setRunning(true);
    setRunResult(null);
    setRunProgress(0);

    try {
      const res = await runWorkflow(selectedWorkflow.id, accessToken);

      if (!res.success) {
        setRunResult({ success: false, error: res.error || 'Failed to run workflow' });
        setRunning(false);
        return;
      }

      const runId = res.data?.run_id || res.data?.id;
      if (!runId) {
        setRunProgress(100);
        setRunResult({ success: true });
        setRunning(false);
        return;
      }

      // Live stage updates via WS; poll remains as fallback.
      try {
        const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
        const wsUrl = `${proto}://${window.location.host}/api/automation/runs/${encodeURIComponent(runId)}/ws`;
        const ws = new WebSocket(wsUrl);
        wsRef.current = ws;

        ws.onmessage = (ev) => {
          if (token !== runTokenRef.current) return;
          let msg;
          try {
            msg = JSON.parse(String(ev.data || ''));
          } catch {
            return;
          }
          if (!msg || typeof msg !== 'object') return;
          const type = String(msg.type || '');
          const status = String(msg.status || '').toLowerCase();
          const nodeKind = String(msg.node_kind || '').toLowerCase();

          if (type === 'node_finished' && !nodeKind.startsWith('trigger.')) {
            // Nudge progress forward even if REST run schema doesn't expose steps.
            setRunProgress((prev) => (prev >= 99 ? prev : Math.min(99, prev + 10)));
          }

          if (type === 'run_finished') {
            // Stop any pending poll loop.
            runTokenRef.current += 1;
            if (pollTimeoutRef.current) {
              clearTimeout(pollTimeoutRef.current);
              pollTimeoutRef.current = null;
            }
            setRunProgress(100);
            const ok = status === 'success' || status === 'completed' || status === 'succeeded';
            setRunResult(ok ? { success: true } : { success: false, error: 'Workflow failed' });
            setRunning(false);
            try {
              wsRef.current?.close();
            } catch {
              // ignore
            }
            wsRef.current = null;
          }
        };

        ws.onclose = () => {
          // leave fallback poll running
        };
      } catch {
        // ignore
      }

      // Poll for run status
      let polls = 0;
      let pollErrors = 0;
      const MAX_POLL_ERRORS = 5;

      const isTerminalSuccess = (s) => {
        const v = (s || '').toString().toLowerCase();
        return ['completed', 'complete', 'success', 'succeeded', 'done', 'finished'].includes(v);
      };

      const isTerminalFailure = (s) => {
        const v = (s || '').toString().toLowerCase();
        return ['failed', 'error', 'errored', 'cancelled', 'canceled', 'aborted', 'stopped'].includes(v);
      };

      const computeProgress = (run) => {
        const p = Number(run?.progress);
        if (Number.isFinite(p) && p >= 0 && p <= 100) return Math.round(p);

        const steps = Array.isArray(run?.step_results)
          ? run.step_results
          : (Array.isArray(run?.steps) ? run.steps : []);
        if (!Array.isArray(steps) || steps.length === 0) return 0;

        const done = steps.filter((s) => {
          const st = (s?.status || '').toString().toLowerCase();
          return ['completed', 'complete', 'succeeded', 'success', 'failed', 'error', 'skipped'].includes(st);
        }).length;
        return Math.round((done / steps.length) * 100);
      };
      
      const pollStatus = async () => {
        if (token !== runTokenRef.current) return;

        if (polls >= MAX_POLLS) {
          setRunResult({ success: false, error: 'Timeout waiting for run to complete' });
          setRunning(false);
          return;
        }

        polls++;

        try {
          const statusRes = await getRun(runId, accessToken);

          if (token !== runTokenRef.current) return;

          if (statusRes.success) {
            pollErrors = 0; // Reset error count on success
            const run = statusRes.data;
            const statusText = run?.status || run?.state || run?.result || '';
            const progress = computeProgress(run);
            setRunProgress(progress);

            const hasTerminalTimestamp = Boolean(
              run?.completed_at
              || run?.completedAt
              || run?.finished_at
              || run?.finishedAt
              || run?.ended_at
              || run?.endedAt
              || run?.end_time
              || run?.endTime
            );

            if (isTerminalSuccess(statusText) || hasTerminalTimestamp) {
              setRunProgress(100);
              setRunResult({ success: true });
              setRunning(false);
              return;
            }
            if (isTerminalFailure(statusText)) {
              setRunResult({ success: false, error: run?.error || run?.message || 'Workflow failed' });
              setRunning(false);
              return;
            }

            // Some backends only expose step_results/progress and never flip status.
            if (progress >= 100 && polls >= 2) {
              setRunProgress(100);
              const ok = !run?.error && !run?.failed && !run?.failure;
              setRunResult(ok ? { success: true } : { success: false, error: run?.error || 'Workflow failed' });
              setRunning(false);
              return;
            }
          } else {
            // Handle failed API response
            pollErrors++;
            if (pollErrors >= MAX_POLL_ERRORS) {
              setRunResult({ success: false, error: statusRes.error || 'Failed to get run status' });
              setRunning(false);
              return;
            }
          }

          pollTimeoutRef.current = setTimeout(pollStatus, POLL_INTERVAL);
        } catch {
          pollErrors++;
          if (pollErrors >= MAX_POLL_ERRORS) {
            setRunResult({ success: false, error: 'Connection lost while checking status' });
            setRunning(false);
            return;
          }
          pollTimeoutRef.current = setTimeout(pollStatus, POLL_INTERVAL);
        }
      };

      pollTimeoutRef.current = setTimeout(pollStatus, POLL_INTERVAL);
    } catch (err) {
      setRunResult({ success: false, error: err.message || 'Failed to run workflow' });
      setRunning(false);
    }
  }, [editMode, selectedWorkflow, running, accessToken]);

  // Navigate to automation editor
  const handleOpen = useCallback(() => {
    if (editMode) return;
    const targetId = selectedWorkflow?.id || workflowId;
    if (targetId) {
      navigate(`/automation/${encodeURIComponent(String(targetId))}`);
      return;
    }
    navigate('/automation');
  }, [editMode, selectedWorkflow, workflowId, navigate]);

  const handlePointerDown = useCallback((e) => {
    dragStartPos.current = { x: e.clientX, y: e.clientY };
    setIsDragging(false);
  }, []);

  const handlePointerMove = useCallback((e) => {
    const dx = Math.abs(e.clientX - dragStartPos.current.x);
    const dy = Math.abs(e.clientY - dragStartPos.current.y);
    if (dx > 5 || dy > 5) {
      setIsDragging(true);
    }
  }, []);

  const handleWidgetClick = useCallback((e) => {
    // Don't navigate if we were dragging or in edit mode
    if (isDragging || editMode) return;
    // Don't navigate if clicking on interactive elements
    if (e.target.closest('button, .glass-pill')) return;
    handleOpen();
  }, [isDragging, editMode, handleOpen]);

  // No workflow selected
  if (!workflowId) {
    return (
      <WidgetShell
        title={settings.title || 'Automation'}
        subtitle="No workflow selected"
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="automation-widget automation-widget--empty"
      >
        <div className="automation-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="automation-widget__empty-icon" />
          <span>Configure this widget to select a workflow</span>
        </div>
      </WidgetShell>
    );
  }

  return (
    <WidgetShell
      className="automation-widget"
      loading={loading}
      error={error}
      status={status}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      interactive={!editMode}
      onClick={handleWidgetClick}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      role={!editMode ? 'button' : undefined}
      tabIndex={!editMode ? 0 : undefined}
      onKeyDown={(e) => {
        if (editMode) return;
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleOpen();
        }
      }}
    >
      <div className="automation-widget__content">
        {/* Header with icon and name */}
        {selectedWorkflow && (
          <div className="automation-widget__header">
            <div className="automation-widget__icon-wrapper">
              <FontAwesomeIcon icon={faBolt} className="automation-widget__icon" />
            </div>
            <span className="automation-widget__workflow-name">
              {selectedWorkflow.name || 'Unnamed workflow'}
            </span>
            <span className="automation-widget__subtitle">Manual Trigger</span>
          </div>
        )}

        {/* Workflow not found */}
        {!loading && !selectedWorkflow && workflowId && (
          <div className="automation-widget__not-found">
            <span>Workflow not found</span>
          </div>
        )}

        {/* Status section: progress or result */}
        {(running || runResult) && (
          <div className="automation-widget__status">
            {running && (
              <>
                <ProgressBar progress={runProgress} className="automation-widget__progress" />
                <span className="automation-widget__progress-text">Running... {runProgress}%</span>
              </>
            )}
            {runResult && !running && (
              <div className={`automation-widget__result ${runResult.success ? 'success' : 'error'}`}>
                <FontAwesomeIcon icon={runResult.success ? faCheck : faXmark} />
                <span>{runResult.success ? 'Completed' : runResult.error}</span>
              </div>
            )}
          </div>
        )}

        {/* Run button */}
        <Button
          className={`automation-widget__run-btn ${running ? 'automation-widget__run-btn--running' : ''}`}
          onClick={(e) => {
            e.stopPropagation();
            handleRun();
          }}
          disabled={editMode || running || !selectedWorkflow}
        >
          <span className="automation-widget__run-btn-icon" aria-hidden>
            <FontAwesomeIcon icon={running ? faSpinner : faPlay} />
          </span>
          <span>{running ? 'Running...' : 'Run'}</span>
        </Button>
      </div>
    </WidgetShell>
  );
}

AutomationWidget.defaultHeight = 3;
