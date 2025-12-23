export function computeEdgesToRender({ nodes, edges, nodeWidth, nodeHeaderHeight }) {
  const safeNodes = Array.isArray(nodes) ? nodes : [];
  const nodeMap = new Map(safeNodes.map(n => [n?.id, n]));
  const safeEdges = Array.isArray(edges) ? edges : [];

  return safeEdges
    .map((e, idx) => {
      if (!e || !e.from || !e.to) return null;
      const fromId = String(e.from);
      const toId = String(e.to);
      const from = nodeMap.get(fromId);
      const to = nodeMap.get(toId);
      if (!from || !to) return null;

      const fromX = Number(from.x);
      const fromY = Number(from.y);
      const toX = Number(to.x);
      const toY = Number(to.y);
      if (!Number.isFinite(fromX) || !Number.isFinite(fromY) || !Number.isFinite(toX) || !Number.isFinite(toY)) return null;

      const x1 = fromX + nodeWidth;
      const y1 = fromY + nodeHeaderHeight / 2;
      const x2 = toX;
      const y2 = toY + nodeHeaderHeight / 2;
      const dx = x2 - x1;
      const dir = dx >= 0 ? 1 : -1;
      const handle = Math.min(120, Math.abs(dx) / 2);
      const c1x = x1 + dir * handle;
      const c2x = x2 - dir * handle;
      return { key: `${fromId}->${toId}#${idx}`, from: fromId, to: toId, x1, y1, x2, y2, c1x, c2x };
    })
    .filter(Boolean);
}

export function computeSvgWorldSize({ nodes, canvasSize, viewportScale, nodeWidth, nodeHeaderHeight }) {
  const safeNodes = Array.isArray(nodes) ? nodes : [];
  const baseW = (canvasSize?.width || 1) / (viewportScale || 1);
  const baseH = (canvasSize?.height || 1) / (viewportScale || 1);

  // Expand bounds in all directions so paths don't clip when nodes are near the visible edges.
  let minX = 0;
  let minY = 0;
  let maxX = baseW;
  let maxY = baseH;

  for (const n of safeNodes) {
    if (!n) continue;

    const x1 = Number(n.x || 0);
    const y1 = Number(n.y || 0);
    const x2 = x1 + nodeWidth;
    const y2 = y1 + nodeHeaderHeight;

    minX = Math.min(minX, x1 - 360);
    minY = Math.min(minY, y1 - 360);
    maxX = Math.max(maxX, x2 + 360);
    maxY = Math.max(maxY, y2 + 360);
  }

  // Keep a sensible upper bound to avoid extreme sizes from accidental drags.
  minX = Math.max(minX, -12000);
  minY = Math.max(minY, -12000);
  maxX = Math.min(maxX, 12000);
  maxY = Math.min(maxY, 12000);

  const w = Math.max(1200, maxX - minX);
  const h = Math.max(800, maxY - minY);
  return { x: minX, y: minY, w, h };
}
