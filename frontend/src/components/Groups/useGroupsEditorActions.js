import { useCallback, useReducer } from 'react';
import { createErsGroup, deleteErsGroup, patchErsGroup, setErsGroupMembers } from '../../services/entityRegistryService';
import { groupsEditorInitialState, groupsEditorReducer } from './groupsEditorReducer';

export function useGroupsEditorActions({ accessToken, refresh, navigate, decodedGroupParam }) {
  const [state, dispatch] = useReducer(groupsEditorReducer, undefined, groupsEditorInitialState);

  const openCreate = useCallback(() => {
    dispatch({ type: 'open-create' });
  }, []);

  const openEdit = useCallback((group) => {
    dispatch({ type: 'open-edit', group });
  }, []);

  const closeEditor = useCallback(() => {
    dispatch({ type: 'close-editor' });
  }, []);

  const openDelete = useCallback((group) => {
    dispatch({ type: 'open-delete', group });
  }, []);

  const closeDelete = useCallback(() => {
    dispatch({ type: 'close-delete' });
  }, []);

  const handleSubmit = useCallback(async ({ id, name, description, deviceIds }) => {
    if (!accessToken) return;
    dispatch({ type: 'set-save-pending', value: true });
    dispatch({ type: 'set-editor-error', value: '' });
    try {
      if (id) {
        const patchResult = await patchErsGroup(id, { name, description }, accessToken);
        if (!patchResult.success) throw new Error(patchResult.error || 'Failed to update group');
        const membersResult = await setErsGroupMembers(id, deviceIds, accessToken);
        if (!membersResult.success) throw new Error(membersResult.error || 'Failed to update group members');
      } else {
        const createResult = await createErsGroup({ name, description, device_ids: deviceIds }, accessToken);
        if (!createResult.success) throw new Error(createResult.error || 'Failed to create group');
      }
      dispatch({ type: 'close-editor' });
      await refresh();
    } catch (err) {
      dispatch({ type: 'set-editor-error', value: err?.message || 'Unable to save group' });
    } finally {
      dispatch({ type: 'set-save-pending', value: false });
    }
  }, [accessToken, refresh]);

  const confirmDelete = useCallback(async () => {
    if (!accessToken || !state.deleteTarget?.id) return;
    dispatch({ type: 'set-delete-pending', value: true });
    dispatch({ type: 'set-delete-error', value: '' });
    try {
      const result = await deleteErsGroup(state.deleteTarget.id, accessToken);
      if (!result.success) throw new Error(result.error || 'Failed to delete group');
      const deletedId = state.deleteTarget.id;
      const deletedSlug = state.deleteTarget.slug;
      dispatch({ type: 'close-delete' });
      if (decodedGroupParam && (decodedGroupParam === deletedId || decodedGroupParam === deletedSlug)) {
        navigate('/groups');
      }
      await refresh();
    } catch (err) {
      dispatch({ type: 'set-delete-error', value: err?.message || 'Unable to delete group' });
    } finally {
      dispatch({ type: 'set-delete-pending', value: false });
    }
  }, [accessToken, decodedGroupParam, navigate, refresh, state.deleteTarget]);

  return {
    ...state,
    openCreate,
    openEdit,
    closeEditor,
    openDelete,
    closeDelete,
    handleSubmit,
    confirmDelete,
    dispatch,
  };
}
