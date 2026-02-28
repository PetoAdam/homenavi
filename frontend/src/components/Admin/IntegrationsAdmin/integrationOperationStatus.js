export const TERMINAL_OPERATION_STAGES = new Set(['ready', 'error', 'installed', 'failed', 'restarted']);

export function normalizeOperationStage(stage) {
  return String(stage || '').trim().toLowerCase();
}

export function isTerminalOperationStatus(status) {
  if (!status || typeof status !== 'object') return false;
  const stage = normalizeOperationStage(status.stage);
  if (!stage) return false;
  return TERMINAL_OPERATION_STAGES.has(stage);
}

export function isSuccessfulOperationStatus(status) {
  if (!status || typeof status !== 'object') return false;
  const stage = normalizeOperationStage(status.stage);
  return stage === 'ready' || stage === 'installed' || stage === 'restarted';
}
