import {
  applyOrthoSnap,
  closestPointOnLineThroughPoint,
  closestPointOnSegment,
  dist,
  unitVector,
} from './mapGeometry';

export function collectSnapTargets(rooms, draft) {
  const vertices = [];
  const segments = [];

  (Array.isArray(rooms) ? rooms : []).forEach((r) => {
    const pts = Array.isArray(r?.points) ? r.points : [];
    pts.forEach((p) => vertices.push(p));
    for (let i = 0; i < pts.length; i += 1) {
      const a = pts[i];
      const b = pts[(i + 1) % pts.length];
      if (a && b) segments.push({ a, b });
    }
  });

  if (draft?.points?.length) {
    draft.points.forEach((p) => vertices.push(p));
    for (let i = 0; i < draft.points.length - 1; i += 1) {
      segments.push({ a: draft.points[i], b: draft.points[i + 1] });
    }
  }

  return { vertices, segments };
}

export function snapAxisToVertices(rawPoint, vertices, snapWorld, excludePoint) {
  if (!rawPoint) return rawPoint;

  let best = null; // { kind, d, x, y }
  (Array.isArray(vertices) ? vertices : []).forEach((v) => {
    if (!v) return;
    if (excludePoint && Math.abs(v.x - excludePoint.x) + Math.abs(v.y - excludePoint.y) < 1e-9) return;

    const dx = Math.abs(rawPoint.x - v.x);
    const dy = Math.abs(rawPoint.y - v.y);

    if (dx <= snapWorld) {
      const cand = { kind: 'axis-x', d: dx, x: v.x, y: rawPoint.y };
      if (!best || cand.d < best.d) best = cand;
    }

    if (dy <= snapWorld) {
      const cand = { kind: 'axis-y', d: dy, x: rawPoint.x, y: v.y };
      if (!best || cand.d < best.d) best = cand;
    }
  });

  if (!best) return rawPoint;
  return { x: best.x, y: best.y, snapped: true, snapKind: best.kind };
}

export function snapPoint(p, vertices, segments, snapWorld, opts = {}) {
  if (!p) return null;

  const enableVertex = opts.vertex !== false;
  const enableEdge = opts.edge !== false;

  let best = { point: p, d: Infinity, kind: '' };

  if (enableVertex) {
    (Array.isArray(vertices) ? vertices : []).forEach((v) => {
      const d = dist(p, v);
      if (d < best.d) {
        best = { point: { ...v }, d, kind: 'vertex' };
      }
    });
  }

  if (enableEdge) {
    (Array.isArray(segments) ? segments : []).forEach((s) => {
      const { point } = closestPointOnSegment(p, s.a, s.b);
      const d = dist(p, point);
      if (d < best.d) {
        best = { point, d, kind: 'segment' };
      }
    });
  }

  if (best.d <= snapWorld) {
    return { ...best.point, snapped: true };
  }
  return { ...p, snapped: false };
}

export function snapToGrid(p, gridSize, snapWorld) {
  if (!p) return null;
  const gx = Math.round(p.x / gridSize) * gridSize;
  const gy = Math.round(p.y / gridSize) * gridSize;
  const candidate = { x: gx, y: gy };
  if (dist(p, candidate) <= snapWorld) return { ...candidate, snapped: true, snapKind: 'grid' };
  return { ...p, snapped: false };
}

export function guideLineFromPointDir(origin, dir) {
  if (!origin || !dir) return null;
  const L = 100000;
  return {
    x1: origin.x - dir.x * L,
    y1: origin.y - dir.y * L,
    x2: origin.x + dir.x * L,
    y2: origin.y + dir.y * L,
  };
}

export function snapForDraw(prevPoint, rawPoint, firstPoint, params) {
  const {
    vertices,
    segments,
    snapSettings,
    snapWorld,
    gridSize,
    orthoRatio,
  } = params || {};

  if (!rawPoint) return { point: null, guide: null };

  let p = { ...rawPoint, snapped: false, snapKind: '' };
  let guide = null;

  // Always allow snapping to the first point to close the polygon neatly.
  if (firstPoint && dist(p, firstPoint) <= snapWorld) {
    p = { x: firstPoint.x, y: firstPoint.y, snapped: true, snapKind: 'close' };
    guide = { kind: 'close', px: p.x, py: p.y };
    return { point: p, guide };
  }

  if (snapSettings?.grid) {
    const g = snapToGrid(p, gridSize, snapWorld);
    if (g?.snapped) {
      p = g;
      guide = { kind: 'grid', px: p.x, py: p.y };
    }
  }

  const geom = snapPoint(p, vertices, segments, snapWorld, { vertex: snapSettings?.vertex, edge: snapSettings?.edge });
  if (geom?.snapped) {
    p = { ...geom, snapKind: 'geom' };
    guide = { kind: 'geom', px: p.x, py: p.y };
  }

  // Align to existing corners by snapping to their X/Y axis.
  if (!p.snapped && snapSettings?.align) {
    const axis = snapAxisToVertices(p, vertices, snapWorld);
    if (axis?.snapped) {
      p = { ...axis, snapKind: axis.snapKind || 'axis' };
      const vertical = p.snapKind === 'axis-x';
      const dir = vertical ? { x: 0, y: 1 } : { x: 1, y: 0 };
      const gl = guideLineFromPointDir({ x: p.x, y: p.y }, dir);
      guide = gl ? { kind: p.snapKind, ...gl, px: p.x, py: p.y } : { kind: p.snapKind, px: p.x, py: p.y };
    }
  }

  if (!p.snapped && prevPoint && snapSettings?.align) {
    let bestSeg = null;
    let bestD = Infinity;
    (Array.isArray(segments) ? segments : []).forEach((s) => {
      const cp = closestPointOnSegment(p, s.a, s.b);
      const d = dist(p, cp.point);
      if (d < bestD) {
        bestD = d;
        bestSeg = s;
      }
    });

    if (bestSeg && bestD <= snapWorld * 6) {
      const dir = unitVector(bestSeg.b.x - bestSeg.a.x, bestSeg.b.y - bestSeg.a.y);
      if (dir) {
        const perp = { x: -dir.y, y: dir.x };
        const candParallel = closestPointOnLineThroughPoint(p, prevPoint, dir);
        const candPerp = closestPointOnLineThroughPoint(p, prevPoint, perp);
        const dPar = candParallel ? dist(p, candParallel) : Infinity;
        const dPerp = candPerp ? dist(p, candPerp) : Infinity;

        if (dPar <= snapWorld || dPerp <= snapWorld) {
          if (dPar <= dPerp) {
            p = { x: candParallel.x, y: candParallel.y, snapped: true, snapKind: 'align-parallel' };
            const gl = guideLineFromPointDir(prevPoint, dir);
            guide = gl ? { kind: 'align-parallel', ...gl, px: p.x, py: p.y } : { kind: 'align-parallel', px: p.x, py: p.y };
          } else {
            p = { x: candPerp.x, y: candPerp.y, snapped: true, snapKind: 'align-perp' };
            const gl = guideLineFromPointDir(prevPoint, perp);
            guide = gl ? { kind: 'align-perp', ...gl, px: p.x, py: p.y } : { kind: 'align-perp', px: p.x, py: p.y };
          }
        }
      }
    }
  }

  if (!p.snapped && prevPoint && snapSettings?.ortho) {
    const o = applyOrthoSnap(prevPoint, p, orthoRatio);
    if (o?.snapped) {
      p = o;
      const isHorizontal = Number.isFinite(prevPoint.y) && Number.isFinite(p.y) && Math.abs(p.y - prevPoint.y) <= 1e-9;
      const dir = isHorizontal ? { x: 1, y: 0 } : { x: 0, y: 1 };
      const gl = guideLineFromPointDir(prevPoint, dir);
      guide = gl ? { kind: 'ortho', ...gl, px: p.x, py: p.y } : { kind: 'ortho', px: p.x, py: p.y };
    }
  }

  return { point: p, guide };
}
