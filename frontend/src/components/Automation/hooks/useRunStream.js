import { useEffect, useRef, useState } from 'react';

/**
 * WebSocket-driven live run highlighting.
 *
 * Keeps node highlight states in sync with streamed automation run events.
 * Includes a polling safety net for cases where the WS is blocked or drops.
 */
export default function useRunStream({ onToast, onError } = {}) {
  const [liveRunNodeStates, setLiveRunNodeStates] = useState({}); // nodeId -> 'active'|'done'|'failed'

  const liveRunTimersRef = useRef(new Map());
  const runWsRef = useRef(null);
  const liveRunRef = useRef({ runId: null, finished: false });
  const pollTokenRef = useRef(0);

  const onToastRef = useRef(onToast);
  const onErrorRef = useRef(onError);
  useEffect(() => {
    onToastRef.current = onToast;
  }, [onToast]);
  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  const clearLiveRunHighlights = () => {
    for (const t of liveRunTimersRef.current.values()) {
      window.clearTimeout(t);
    }
    liveRunTimersRef.current.clear();
    setLiveRunNodeStates({});
    liveRunRef.current = { runId: null, finished: false };
  };

  const closeRunWs = () => {
    const ws = runWsRef.current;
    runWsRef.current = null;
    if (!ws) return;
    try {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onerror = null;
      ws.onclose = null;
      ws.close();
    } catch {
      // ignore
    }
  };

  const setLiveNodeState = (nodeId, state, { clearAfterMs } = {}) => {
    const id = String(nodeId || '').trim();
    if (!id) return;

    const existing = liveRunTimersRef.current.get(id);
    if (existing) {
      window.clearTimeout(existing);
      liveRunTimersRef.current.delete(id);
    }

    setLiveRunNodeStates((prev) => ({ ...prev, [id]: state }));

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

    // Connect WebSocket for live step events.
    try {
      const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
      const wsUrl = `${proto}://${window.location.host}/ws/automation/runs/${encodeURIComponent(id)}`;
      const ws = new WebSocket(wsUrl);
      runWsRef.current = ws;

      ws.onmessage = (ev) => {
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
          if (status === 'success') {
            setLiveNodeState(nodeId, 'done', { clearAfterMs: 1100 });
          } else {
            setLiveNodeState(nodeId, 'failed', { clearAfterMs: 2200 });
          }
          return;
        }

        if (type === 'run_finished') {
          liveRunRef.current = { ...liveRunRef.current, finished: true };
          if (status === 'success') {
            onToastRef.current?.('Run complete');
          } else {
            onErrorRef.current?.(String(msg.error || 'Run failed'));
            onToastRef.current?.('Run failed');
          }
          safeRefreshRuns();
        }
      };

      ws.onclose = async () => {
        // If the WS drops early, fall back to checking run status.
        if (token !== pollTokenRef.current) return;
        const current = liveRunRef.current;
        if (!current?.runId) return;
        if (current.finished) return;
        if (!getRun || !accessToken) return;

        const rr = await getRun(current.runId, accessToken);
        if (rr?.success) {
          const status = String(rr.data?.run?.status || '').toLowerCase();
          if (status && status !== 'running') {
            liveRunRef.current = { ...current, finished: true };
            onToastRef.current?.(status === 'success' ? 'Run complete' : 'Run finished');
          }
          safeRefreshRuns();
        }
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
          const status = String(rr.data?.run?.status || '').toLowerCase();
          if (status && status !== 'running') {
            liveRunRef.current = { ...current, finished: true };
            onToastRef.current?.(status === 'success' ? 'Run complete' : 'Run finished');
            safeRefreshRuns();
            return;
          }
        }
        // eslint-disable-next-line no-await-in-loop
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
    liveRunNodeStates,
    clearLiveRunHighlights,
    closeRunWs,
    startRunStream,
  };
}
