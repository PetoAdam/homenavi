import { readDevicePlacementFromErsMeta, readRoomGeometryFromErsMeta, safeString } from './mapErsMeta';

export function mergeRoomsFromErs(prevLayout, ersRooms) {
  const prev = prevLayout && typeof prevLayout === 'object' ? prevLayout : {};
  if (!Array.isArray(ersRooms) || ersRooms.length === 0) return prev;

  const prevRooms = Array.isArray(prev.rooms) ? prev.rooms : [];
  const prevById = new globalThis.Map(prevRooms.map((r) => [safeString(r?.id), r]));

  const mergedRooms = [];
  ersRooms.forEach((room) => {
    const id = safeString(room?.id);
    if (!id) return;

    const local = prevById.get(id);
    const geom = readRoomGeometryFromErsMeta(room);

    const localPoints = Array.isArray(local?.points) ? local.points : [];
    const localWalls = Array.isArray(local?.wallLengths) ? local.wallLengths : [];

    const points = geom?.points || localPoints;
    const wallLengths = geom?.wallLengths || localWalls;

    mergedRooms.push({
      id,
      name: safeString(room?.name) || safeString(local?.name) || 'Room',
      points: Array.isArray(points) ? points : [],
      wallLengths: Array.isArray(wallLengths) ? wallLengths : [],
    });

    prevById.delete(id);
  });

  // ERS is the source of truth for persisted rooms.
  // Do not keep orphaned local rooms here, otherwise stale geometry from an older
  // layout snapshot can be rendered on top of the current map after refreshes.

  return { ...prev, rooms: mergedRooms };
}

export function mergeDevicePlacementsFromErs(prevLayout, devicesForPalette, draggingDeviceKey) {
  const prev = prevLayout && typeof prevLayout === 'object' ? prevLayout : {};
  if (!Array.isArray(devicesForPalette) || devicesForPalette.length === 0) return prev;

  const prevPlacements = prev.devicePlacements && typeof prev.devicePlacements === 'object' ? prev.devicePlacements : {};
  let changed = false;
  const nextPlacements = {};

  const readExistingPlacement = (aliases) => {
    for (const alias of aliases) {
      const key = safeString(alias);
      if (!key) continue;
      const placement = prevPlacements[key];
      if (placement && typeof placement === 'object') {
        return { key, placement };
      }
    }
    return null;
  };

  devicesForPalette.forEach((d) => {
    const key = safeString(d?.ersId || d?.id || d?.hdpId);
    if (!key) return;

    // Avoid fighting the pointer while dragging a device.
    const aliases = [key, d?.id, d?.hdpId, ...(Array.isArray(d?.hdpIds) ? d.hdpIds : [])];
    const existingEntry = readExistingPlacement(aliases);
    if (draggingDeviceKey && aliases.some((alias) => safeString(alias) === draggingDeviceKey)) {
      if (existingEntry) {
        nextPlacements[key] = existingEntry.placement;
        if (existingEntry.key !== key) changed = true;
      }
      return;
    }

    const existing = existingEntry?.placement;
    const placement = readDevicePlacementFromErsMeta(d);

    const hasXY = existing && Number.isFinite(Number(existing.x)) && Number.isFinite(Number(existing.y));
    if (!placement && hasXY) {
      nextPlacements[key] = existing;
      if (existingEntry?.key !== key) changed = true;
      return;
    }
    if (!placement) {
      if (existingEntry) changed = true;
      return;
    }

    const roomId = safeString(d?.room_id) || safeString(existing?.roomId);
    const nextPlacement = { roomId, x: placement.x, y: placement.y };
    if (!existing || Number(existing.x) !== placement.x || Number(existing.y) !== placement.y || safeString(existing.roomId) !== roomId || existingEntry?.key !== key) {
      nextPlacements[key] = nextPlacement;
      changed = true;
      return;
    }

    nextPlacements[key] = existing;
  });

  const prevKeys = Object.keys(prevPlacements);
  const nextKeys = Object.keys(nextPlacements);
  if (!changed && (prevKeys.length !== nextKeys.length || prevKeys.some((key) => !(key in nextPlacements)))) {
    changed = true;
  }

  return changed ? { ...prev, devicePlacements: nextPlacements } : prev;
}
