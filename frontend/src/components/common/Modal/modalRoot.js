export function getModalRoot() {
  if (typeof document === 'undefined') return null;
  return document.getElementById('modal-root') || document.body;
}
