import { useCallback, useEffect, useRef, useState } from 'react';
import { getSharedWebSocket, wsUrlForPath } from '../../../services/realtime/sharedWebSocket';
import { deriveLiveRunNodeStatesFromRunPayload, isTriggerRunEvent } from './runStreamUtils';

const RUN_HIGHLIGHT_CLEAR_GRACE_MS = 5000;
const TRIGGER_ACTIVE_MIN_MS = 700;

function mergeNodeStates(prev, updates) {
  const current = prev || {};
  const nextEntries = Object.entries(updates || {});
  if (nextEntries.length === 0) return current;

  let changed = false;
  const next = { ...current };
  nextEntries.forEach(([key, value]) => {
    if (next[key] === value) return;
    next[key] = value;
    changed = true;
  });
  return changed ? next : current;
}

/**
 * WebSocket-driven live run highlighting.
 *
 * Keeps node highlight states in sync with streamed automation run events.
 * Includes a polling safety net for cases where the WS is blocked or drops.
 */
export default function useRunStream({ onToast, onError } = {}) {
  const [liveRunNodeStates, setLiveRunNodeStates] = useState({}); // nodeId -> 'active'|'done'|'failed'

  const liveRunTimersRef = useRef(new Map());
  const runWsCleanupRef = useRef(null);
  const liveRunRef = useRef({ runId: null, finished: false });
  const pollTokenRef = useRef(0);
  const clearAllHighlightsTimerRef = useRef(null);
  const nodeActivatedAtRef = useRef(new Map());

  const onToastRef = useRef(onToast);
  const onErrorRef = useRef(onError);
  useEffect(() => {
    onToastRef.current = onToast;
  }, [onToast]);
  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  const clearLiveRunHighlights = useCallback(() => {
    if (clearAllHighlightsTimerRef.current) {
      window.clearTimeout(clearAllHighlightsTimerRef.current);
      clearAllHighlightsTimerRef.current = null;
    }
    for (const t of liveRunTimersRef.current.values()) {
      window.clearTimeout(t);
    }
    liveRunTimersRef.current.clear();
    nodeActivatedAtRef.current.clear();
    setLiveRunNodeStates({});
    liveRunRef.current = { runId: null, finished: false };
  }, []);

  const closeRunWs = useCallback(() => {
    try {
      runWsCleanupRef.current?.();
    } catch {
      // ignore
    }
    runWsCleanupRef.current = null;
  }, []);

  const scheduleClearAllHighlights = useCallback((delayMs = RUN_HIGHLIGHT_CLEAR_GRACE_MS) => {
    if (clearAllHighlightsTimerRef.current) {
      window.clearTimeout(clearAllHighlightsTimerRef.current);
      clearAllHighlightsTimerRef.current = null;
    }
    clearAllHighlightsTimerRef.current = window.setTimeout(() => {
      clearAllHighlightsTimerRef.current = null;
      closeRunWs();
      clearLiveRunHighlights();
    }, delayMs);
  }, [clearLiveRunHighlights, closeRunWs]);

  const setLiveNodeState = (nodeId, state, { clearAfterMs } = {}) => {
    const id = String(nodeId || '').trim();
    if (!id) return;

    const existing = liveRunTimersRef.current.get(id);
    if (existing) {
      window.clearTimeout(existing);
      liveRunTimersRef.current.delete(id);
    }

    if (state === 'active') {
      nodeActivatedAtRef.current.set(id, Date.now());
    }

    setLiveRunNodeStates((prev) => mergeNodeStates(prev, { [id]: state }));

    if (Number.isFinite(clearAfterMs) && clearAfterMs > 0) {
      const t = window.setTimeout(() => {
        liveRunTimersRef.current.delete(id);
        setLiveRunNodeStates((prev) => {
          if (!prev || !(id in prev)) return prev;
          const next = { ...prev };
          delete next[id];
          return next;
        });
      }, clearAfterMs);
      liveRunTimersRef.current.set(id, t);
    }
  };

  const startRunStream = async ({
    runId,
    accessToken,
    workflowId,
    refreshRuns,
    getRun,
  }) => {
    const id = String(runId || '').trim();
    if (!id) return;

    if (clearAllHighlightsTimerRef.current) {
      window.clearTimeout(clearAllHighlightsTimerRef.current);
      clearAllHighlightsTimerRef.current = null;
    }

    if (liveRunRef.current.runId === id && !liveRunRef.current.finished) {
      return;
    }

    pollTokenRef.current += 1;
    const token = pollTokenRef.current;

    liveRunRef.current = { runId: id, finished: false };

    const safeRefreshRuns = () => {
      try {
        refreshRuns?.(workflowId);
      } catch {
        // ignore
      }
    };

    const syncLiveNodeStatesFromRunPayload = (payload) => {
      const nextStates = deriveLiveRunNodeStatesFromRunPayload(payload);
      if (Object.keys(nextStates).length === 0) return;
      setLiveRunNodeStates((prev) => mergeNodeStates(prev, nextStates));
    };

    const finalizeRun = (status, errorMessage = '') => {
      liveRunRef.current = { ...liveRunRef.current, finished: true };
      if (status === 'success') {
        onToastRef.current?.('Run complete');
      } else {
        if (errorMessage) {
          onErrorRef.current?.(errorMessage);
        }
        onToastRef.current?.('Run failed');
      }
      safeRefreshRuns();
      scheduleClearAllHighlights();
    };

    // Connect WebSocket for live step events.
    try {
      const wsUrl = wsUrlForPath(`/ws/automation/runs/${encodeURIComponent(id)}`);
      const channel = getSharedWebSocket(wsUrl);

      const unsubMessage = channel.subscribe((ev) => {
        let msg;
        try {
          msg = JSON.parse(String(ev.data || ''));
        } catch {
          return;
        }
        if (!msg || typeof msg !== 'object') return;

        const msgRunId = String(msg.run_id || '').trim();
        if (msgRunId && liveRunRef.current.runId && msgRunId !== liveRunRef.current.runId) return;

        const type = String(msg.type || '').trim();
        const nodeId = String(msg.node_id || '').trim();
        const status = String(msg.status || '').trim();

        if (type === 'run_started') {
          onToastRef.current?.('Running…');
          return;
        }

        if (type === 'run_waiting') {
          onToastRef.current?.('Waiting for result…');
          return;
        }

        if (type === 'node_started') {
          // Keep active while node runs (sleep stays active until node_finished).
          setLiveNodeState(nodeId, 'active');
          return;
        }

        if (type === 'node_finished') {
          const terminalState = status === 'success' ? 'done' : 'failed';
          // Trigger nodes complete synchronously. Defer their terminal state two
          // frames so the preceding node_started highlight is painted first.
          if (isTriggerRunEvent(msg)) {
            const activeSince = nodeActivatedAtRef.current.get(nodeId) ?? Date.now();
            const remainingActiveMs = Math.max(0, TRIGGER_ACTIVE_MIN_MS - (Date.now() - activeSince));
            window.requestAnimationFrame(() => {
              window.requestAnimationFrame(() => {
                if (remainingActiveMs > 0) {
                  const t = window.setTimeout(() => {
                    liveRunTimersRef.current.delete(nodeId);
                    setLiveNodeState(nodeId, terminalState);
                  }, remainingActiveMs);
                  liveRunTimersRef.current.set(nodeId, t);
                  return;
                }
                setLiveNodeState(nodeId, terminalState);
              });
            });
          } else {
            setLiveNodeState(nodeId, terminalState);
          }
          return;
        }

        if (type === 'run_finished') {
          finalizeRun(status, String(msg.error || 'Run failed'));
        }
      });

      const unsubStatus = channel.onStatus(async ({ status: wsStatus }) => {
        // If the WS drops early, fall back to checking run status.
        if (wsStatus !== 'closed' && wsStatus !== 'error') return;
        if (token !== pollTokenRef.current) return;
        const current = liveRunRef.current;
        if (!current?.runId) return;
        if (current.finished) return;
        if (!getRun || !accessToken) return;

        const rr = await getRun(current.runId, accessToken);
        if (rr?.success) {
          syncLiveNodeStatesFromRunPayload(rr.data);
          const status = String(rr.data?.run?.status || '').toLowerCase();
          if (status && status !== 'running') {
            finalizeRun(status, String(rr.data?.run?.error || 'Run failed'));
          }
        }
      });

      runWsCleanupRef.current = () => {
        unsubMessage();
        unsubStatus();
      };
    } catch {
      // ignore; we'll just fall back to polling.
    }

    // Poll as a safety net (covers cases where WS is blocked).
    (async () => {
      if (!getRun || !accessToken) return;

      const startedAt = Date.now();
      while (Date.now() - startedAt < 60_000) {
        if (token !== pollTokenRef.current) return;

        const current = liveRunRef.current;
        if (!current?.runId || current.finished) return;

        const rr = await getRun(current.runId, accessToken);
        if (rr?.success) {
          syncLiveNodeStatesFromRunPayload(rr.data);
          const status = String(rr.data?.run?.status || '').toLowerCase();
          if (status && status !== 'running') {
            finalizeRun(status, String(rr.data?.run?.error || 'Run failed'));
            return;
          }
        }
        await new Promise(r => window.setTimeout(r, 850));
      }
    })();
  };

  useEffect(() => {
    return () => {
      pollTokenRef.current += 1;
      closeRunWs();
      clearLiveRunHighlights();
    };
  }, [clearLiveRunHighlights, closeRunWs]);

  return {
    liveRunNodeStates,
    clearLiveRunHighlights,
    closeRunWs,
    startRunStream,
  };
}
