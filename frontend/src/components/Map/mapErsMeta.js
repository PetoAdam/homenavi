function normalizeNumber(value) {
  const n = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(n)) return null;
  return n;
}

function normalizePointArray(value) {
  const pts = Array.isArray(value) ? value : [];
  const out = [];
  for (const p of pts) {
    const x = normalizeNumber(p?.x);
    const y = normalizeNumber(p?.y);
    if (x === null || y === null) continue;
    out.push({ x, y });
  }
  return out;
}

export function safeString(value) {
  return typeof value === 'string' ? value : '';
}

export function normalizeNumberOrNull(value) {
  return normalizeNumber(value);
}

export function normalizePointArrayFromMeta(value) {
  return normalizePointArray(value);
}

export function readRoomGeometryFromErsMeta(room) {
  const meta = room?.meta && typeof room.meta === 'object' ? room.meta : null;
  const mapMeta = meta?.map && typeof meta.map === 'object' ? meta.map : null;
  if (!mapMeta) return null;

  const points = normalizePointArray(mapMeta.points);
  const wallLengths = Array.isArray(mapMeta.wall_lengths)
    ? mapMeta.wall_lengths.map(v => (normalizeNumber(v) ?? null))
    : (Array.isArray(mapMeta.wallLengths) ? mapMeta.wallLengths.map(v => (normalizeNumber(v) ?? null)) : null);

  if (points.length < 3) return null;
  return {
    points,
    wallLengths: Array.isArray(wallLengths) ? wallLengths : [],
  };
}

export function readDevicePlacementFromErsMeta(device) {
  const meta = device?.meta && typeof device.meta === 'object' ? device.meta : null;
  const mapMeta = meta?.map && typeof meta.map === 'object' ? meta.map : null;
  if (!mapMeta) return null;
  const x = normalizeNumber(mapMeta.x);
  const y = normalizeNumber(mapMeta.y);
  if (x === null || y === null) return null;
  return { x, y };
}

export function readFavoriteFieldsFromErsMeta(device) {
  const meta = device?.meta && typeof device.meta === 'object' ? device.meta : null;
  const mapMeta = meta?.map && typeof meta.map === 'object' ? meta.map : null;
  if (!mapMeta) return [];

  const rawArray = mapMeta.favorite_fields ?? mapMeta.favoriteFields ?? mapMeta.favorite_keys ?? mapMeta.favoriteKeys;
  const rawSingle = mapMeta.favorite_field ?? mapMeta.favoriteField ?? mapMeta.favorite_key ?? mapMeta.favoriteKey;

  const normalize = (value) => (typeof value === 'string' ? value.trim() : '');

  const out = [];
  if (Array.isArray(rawArray)) {
    rawArray.forEach(v => {
      const s = normalize(v);
      if (s) out.push(s);
    });
  }
  if (out.length === 0) {
    const s = normalize(rawSingle);
    if (s) out.push(s);
  }

  const seen = new Set();
  return out.filter(v => {
    if (seen.has(v)) return false;
    seen.add(v);
    return true;
  });
}
