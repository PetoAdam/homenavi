export function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

export function dist(a, b) {
  const dx = a.x - b.x;
  const dy = a.y - b.y;
  return Math.hypot(dx, dy);
}

export function closestPointOnSegment(p, a, b) {
  const abx = b.x - a.x;
  const aby = b.y - a.y;
  const apx = p.x - a.x;
  const apy = p.y - a.y;
  const abLen2 = abx * abx + aby * aby;
  if (abLen2 === 0) return { point: { ...a }, t: 0 };
  const t = clamp((apx * abx + apy * aby) / abLen2, 0, 1);
  return { point: { x: a.x + abx * t, y: a.y + aby * t }, t };
}

export function pointInPolygon(point, polygon) {
  const pts = Array.isArray(polygon) ? polygon : [];
  if (pts.length < 3) return false;
  let inside = false;
  for (let i = 0, j = pts.length - 1; i < pts.length; j = i++) {
    const xi = pts[i].x;
    const yi = pts[i].y;
    const xj = pts[j].x;
    const yj = pts[j].y;
    const intersect = ((yi > point.y) !== (yj > point.y))
      && (point.x < (xj - xi) * (point.y - yi) / ((yj - yi) || 1e-9) + xi);
    if (intersect) inside = !inside;
  }
  return inside;
}

export function computeSegmentLengths(points) {
  const pts = Array.isArray(points) ? points : [];
  if (pts.length < 2) return [];
  const out = [];
  for (let i = 0; i < pts.length - 1; i += 1) {
    out.push(dist(pts[i], pts[i + 1]));
  }
  return out;
}

export function roomPolygonToPath(points) {
  const pts = Array.isArray(points) ? points : [];
  if (pts.length === 0) return '';
  return `M ${pts.map(p => `${p.x} ${p.y}`).join(' L ')} Z`;
}

export function applyOrthoSnap(prev, next, ratio) {
  if (!prev || !next) return next;
  const dx = next.x - prev.x;
  const dy = next.y - prev.y;
  const adx = Math.abs(dx);
  const ady = Math.abs(dy);
  if (adx === 0 && ady === 0) return next;
  if (adx * ratio > ady) {
    // mostly horizontal
    return { ...next, y: prev.y, snapped: true, snapKind: 'ortho' };
  }
  if (ady * ratio > adx) {
    // mostly vertical
    return { ...next, x: prev.x, snapped: true, snapKind: 'ortho' };
  }
  return next;
}

export function applyLengthToSegment(points, index, length) {
  const pts = Array.isArray(points) ? points.slice() : [];
  if (!Number.isFinite(length) || length === null) return pts;
  if (index < 0 || index >= pts.length - 1) return pts;
  const a = pts[index];
  const b = pts[index + 1];
  if (!a || !b) return pts;
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const d = Math.hypot(dx, dy);
  if (!Number.isFinite(d) || d <= 1e-6) return pts;
  const s = length / d;
  pts[index + 1] = { x: a.x + dx * s, y: a.y + dy * s };
  return pts;
}

export function unitVector(dx, dy) {
  const d = Math.hypot(dx, dy);
  if (!Number.isFinite(d) || d <= 1e-9) return null;
  return { x: dx / d, y: dy / d };
}

export function closestPointOnLineThroughPoint(p, origin, dir) {
  if (!p || !origin || !dir) return null;
  const opx = p.x - origin.x;
  const opy = p.y - origin.y;
  const t = opx * dir.x + opy * dir.y;
  return { x: origin.x + dir.x * t, y: origin.y + dir.y * t, t };
}
