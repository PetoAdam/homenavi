import { useEffect, useRef } from 'react';

export function findLatestRunningRun(runs) {
  const items = Array.isArray(runs) ? runs : [];
  return items.find((run) => String(run?.status || '').trim().toLowerCase() === 'running' && String(run?.id || '').trim()) || null;
}

export default function useAutoAttachRunningRun({
  accessToken,
  selectedWorkflow,
  runs,
  fetchRuns,
  clearLiveRunHighlights,
  closeRunWs,
  startRunStream,
  getRun,
}) {
  const lastAttachedKeyRef = useRef('');

  useEffect(() => {
    const workflowId = String(selectedWorkflow?.id || '').trim();
    if (!workflowId || !accessToken) {
      lastAttachedKeyRef.current = '';
      return;
    }

    const runningRun = findLatestRunningRun(runs);
    if (!runningRun) {
      lastAttachedKeyRef.current = '';
      return;
    }

    const runId = String(runningRun.id || '').trim();
    const key = `${workflowId}:${runId}`;
    if (!runId || lastAttachedKeyRef.current === key) {
      return;
    }

    lastAttachedKeyRef.current = key;
    clearLiveRunHighlights();
    closeRunWs();
    startRunStream({
      runId,
      accessToken,
      workflowId,
      refreshRuns: (id) => fetchRuns?.(id, undefined, { silent: true }),
      getRun,
    });
  }, [accessToken, clearLiveRunHighlights, closeRunWs, fetchRuns, getRun, runs, selectedWorkflow, startRunStream]);
}