export function newNodeId(prefix = 'node') {
  return `${prefix}_${Date.now()}_${Math.floor(Math.random() * 1e6)}`;
}

export function normalizeDeviceLabel(device) {
  const name = typeof device?.name === 'string' ? device.name.trim() : '';
  const id = device?.device_id || device?.external_id || device?.id || '';
  if (name && id) return `${name} (${id})`;
  if (name) return name;
  return String(id || 'Unknown device');
}

export function clamp(n, min, max) {
  return Math.max(min, Math.min(max, n));
}

export function getDropPosition(e, canvasEl) {
  const rect = canvasEl.getBoundingClientRect();
  const x = e.clientX - rect.left;
  const y = e.clientY - rect.top;
  return { x, y };
}

export function screenToWorld({ x, y }, viewport) {
  return {
    x: (x - viewport.x) / viewport.scale,
    y: (y - viewport.y) / viewport.scale,
  };
}

export function zoomAroundPoint(viewport, pointScreen, nextScale) {
  const scale = clamp(nextScale, 0.4, 1.8);
  const world = screenToWorld(pointScreen, viewport);
  return {
    scale,
    x: pointScreen.x - world.x * scale,
    y: pointScreen.y - world.y * scale,
  };
}
