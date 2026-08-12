import { useCallback, useMemo } from 'react';
import { createErsTag, deleteErsTag, patchErsDevice, setErsDeviceTags } from '../../../services/entityRegistryService';

export function readFavoriteFieldsFromErsMeta(ersDevice) {
  const meta = ersDevice?.meta && typeof ersDevice.meta === 'object' ? ersDevice.meta : null;
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
  return Array.from(new Set(out));
}

export function collectFavoriteFieldOptionsFromDevice(device) {
  const state = device?.state && typeof device.state === 'object' && !Array.isArray(device.state) ? device.state : null;
  if (!state) return [];
  const reserved = new Set([
    'schema', 'device_id', 'deviceid', 'external_id', 'externalid', 'protocol', 'topic', 'retained',
    'ts', 'timestamp', 'time', 'received_at', 'receivedat',
    'capabilities',
  ]);
  return Object.keys(state)
    .filter(k => k && !reserved.has(k.toLowerCase()))
    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
}

export function useDeviceDetailMetadataActions({
  accessToken,
  device,
  ersDevice,
  ersRooms,
  ersTags,
  currentRoomId,
  currentTagIds,
  editRoomId,
  editTagIds,
  newTagName,
  favoriteSaving,
  setFavoriteSaving,
  setFavoriteError,
  setGroupingEditing,
  setNewTagName,
  setEditTagIds,
  refreshErs,
}) {
  const favoriteFieldOptions = useMemo(() => collectFavoriteFieldOptionsFromDevice(device), [device]);
  const favoriteFields = useMemo(() => readFavoriteFieldsFromErsMeta(ersDevice), [ersDevice]);

  const currentRoomName = useMemo(() => {
    const rid = currentRoomId;
    if (!rid) return 'None';
    const room = (Array.isArray(ersRooms) ? ersRooms : []).find(r => (r?.id || '').toString() === rid);
    return room?.name || ersDevice?.room?.name || 'None';
  }, [currentRoomId, ersRooms, ersDevice?.room?.name]);

  const currentTags = useMemo(() => {
    const ids = new Set(currentTagIds);
    return (Array.isArray(ersTags) ? ersTags : [])
      .filter(t => ids.has((t?.id || '').toString()))
      .map(t => ({ id: (t?.id || '').toString(), name: t?.name || '' }))
      .filter(t => t.id && t.name);
  }, [currentTagIds, ersTags]);

  const tagOptions = useMemo(() => (
    (Array.isArray(ersTags) ? ersTags : [])
      .slice()
      .sort((a, b) => (a?.name || '').localeCompare(b?.name || '', undefined, { sensitivity: 'base' }))
      .map(t => ({ value: (t?.id || '').toString(), label: t?.name || '' }))
      .filter(t => t.value && t.label)
  ), [ersTags]);

  const saveFavoriteFields = useCallback(async (nextValues) => {
    if (!accessToken) {
      setFavoriteError('Authentication required');
      return;
    }
    if (!ersDevice?.ersId) {
      setFavoriteError('This device is not registered in ERS yet.');
      return;
    }
    const list = Array.isArray(nextValues) ? nextValues : [];
    const normalized = list.map(v => (typeof v === 'string' ? v.trim() : '')).filter(Boolean);
    const deduped = Array.from(new Set(normalized));
    setFavoriteSaving(true);
    setFavoriteError('');
    try {
      const res = await patchErsDevice(ersDevice.ersId, {
        meta: { map: { favorite_fields: deduped.length ? deduped : null } },
      }, accessToken);
      if (!res.success) throw new Error(res.error || 'Unable to save favorite field');
      await refreshErs();
    } catch (err) {
      setFavoriteError(err?.message || 'Unable to save favorite field');
    } finally {
      setFavoriteSaving(false);
    }
  }, [accessToken, ersDevice?.ersId, refreshErs, setFavoriteError, setFavoriteSaving]);

  const saveGrouping = useCallback(async () => {
    if (!accessToken) {
      setFavoriteError('Authentication required');
      return;
    }
    if (!ersDevice?.ersId) return;
    setFavoriteSaving(true);
    setFavoriteError('');
    try {
      const nextRoom = editRoomId ? editRoomId : null;
      const roomChanged = (currentRoomId || '') !== (editRoomId || '');
      const nextTags = Array.isArray(editTagIds) ? editTagIds : [];
      const tagsChanged = JSON.stringify([...currentTagIds].sort()) !== JSON.stringify([...nextTags].map(String).sort());

      if (roomChanged) {
        const res = await patchErsDevice(ersDevice.ersId, { room_id: nextRoom }, accessToken);
        if (!res.success) throw new Error(res.error || 'Unable to update room');
      }

      if (tagsChanged) {
        const res = await setErsDeviceTags(ersDevice.ersId, nextTags, accessToken);
        if (!res.success) throw new Error(res.error || 'Unable to update tags');
      }

      await refreshErs();
      setGroupingEditing(false);
      setNewTagName('');
    } catch (err) {
      setFavoriteError(err?.message || 'Unable to update grouping');
    } finally {
      setFavoriteSaving(false);
    }
  }, [accessToken, currentRoomId, currentTagIds, editRoomId, editTagIds, ersDevice?.ersId, refreshErs, setFavoriteError, setFavoriteSaving, setGroupingEditing, setNewTagName]);

  const handleCreateTag = useCallback(async () => {
    const name = typeof newTagName === 'string' ? newTagName.trim() : '';
    if (!name) return;
    if (!accessToken) {
      setFavoriteError('Authentication required');
      return;
    }
    setFavoriteSaving(true);
    setFavoriteError('');
    try {
      const res = await createErsTag({ name }, accessToken);
      if (!res.success) throw new Error(res.error || 'Unable to create tag');
      const createdId = (res.data?.id || '').toString();
      await refreshErs();
      if (createdId) {
        setEditTagIds(prev => (prev.includes(createdId) ? prev : [...prev, createdId]));
      }
      setNewTagName('');
    } catch (err) {
      setFavoriteError(err?.message || 'Unable to create tag');
    } finally {
      setFavoriteSaving(false);
    }
  }, [accessToken, newTagName, refreshErs, setEditTagIds, setFavoriteError, setFavoriteSaving, setNewTagName]);

  const handleDeleteTag = useCallback(async (tagId, tagName) => {
    const id = typeof tagId === 'string' ? tagId.trim() : '';
    if (!id) return;
    if (!accessToken) {
      setFavoriteError('Authentication required');
      return;
    }
    const label = typeof tagName === 'string' && tagName.trim() ? tagName.trim() : 'this tag';
    const ok = window.confirm(`Delete tag "${label}"? This removes it from all devices.`);
    if (!ok) return;

    setFavoriteSaving(true);
    setFavoriteError('');
    try {
      const res = await deleteErsTag(id, accessToken);
      if (!res.success) throw new Error(res.error || 'Unable to delete tag');
      setEditTagIds(prev => (Array.isArray(prev) ? prev.filter(x => x !== id) : []));
      await refreshErs();
    } catch (err) {
      setFavoriteError(err?.message || 'Unable to delete tag');
    } finally {
      setFavoriteSaving(false);
    }
  }, [accessToken, refreshErs, setEditTagIds, setFavoriteError, setFavoriteSaving]);

  const beginGroupingEdit = useCallback(() => {
    if (!ersDevice) return;
    setGroupingEditing(true);
    setFavoriteError('');
  }, [ersDevice, setFavoriteError, setGroupingEditing]);

  const cancelGroupingEdit = useCallback(() => {
    setGroupingEditing(false);
    setFavoriteError('');
    setNewTagName('');
  }, [setFavoriteError, setGroupingEditing, setNewTagName]);

  return {
    currentRoomName,
    currentTags,
    favoriteFieldOptions,
    favoriteFields,
    handleCreateTag,
    handleDeleteTag,
    beginGroupingEdit,
    cancelGroupingEdit,
    saveGrouping,
    saveFavoriteFields,
    tagOptions,
    favoriteSaving,
  };
}
