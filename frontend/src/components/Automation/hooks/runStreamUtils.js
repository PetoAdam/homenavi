export function isTriggerRunEvent(event) {
  return String(event?.node_kind || '').trim().toLowerCase().startsWith('trigger.');
}

function normalizeStepStatus(status) {
  return String(status || '').trim().toLowerCase();
}

export function deriveLiveRunNodeStatesFromRunPayload(payload) {
  const steps = Array.isArray(payload?.steps)
    ? payload.steps
    : Array.isArray(payload?.run?.step_results)
      ? payload.run.step_results
      : Array.isArray(payload?.run?.steps)
        ? payload.run.steps
        : [];

  return steps.reduce((acc, step) => {
    const nodeId = String(step?.node_id || step?.nodeId || '').trim();
    if (!nodeId) return acc;

    const status = normalizeStepStatus(step?.status);
    if (['success', 'succeeded', 'completed', 'complete', 'done', 'finished', 'skipped'].includes(status)) {
      acc[nodeId] = 'done';
      return acc;
    }
    if (['failed', 'error', 'errored', 'cancelled', 'canceled', 'aborted', 'stopped'].includes(status)) {
      acc[nodeId] = 'failed';
      return acc;
    }
    if (['running', 'active', 'started', 'queued', 'pending', 'waiting', 'in_progress', 'in-progress'].includes(status)) {
      acc[nodeId] = 'active';
    }
    return acc;
  }, {});
}