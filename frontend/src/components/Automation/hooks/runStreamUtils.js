export function isTriggerRunEvent(event) {
  return String(event?.node_kind || '').trim().toLowerCase().startsWith('trigger.');
}