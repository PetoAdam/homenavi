import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  installIntegration,
  reloadIntegrations,
  restartAllIntegrations,
  setIntegrationAutoUpdate,
  uninstallIntegration,
  updateIntegration,
} from '../../../services/integrationService';
import { queryKeys } from '../../../state/queryKeys';

export function unwrapIntegrationResponse(response, fallbackMessage) {
  if (response?.success) {
    return response.data;
  }
  const detail = response?.data?.detail ? ` (${response.data.detail})` : '';
  const error = new Error(`${response?.error || fallbackMessage}${detail}`);
  error.responseData = response?.data;
  throw error;
}

export async function syncIntegrationCaches(queryClient, registryParams = {}) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: queryKeys.integrations.root }),
    queryClient.invalidateQueries({ queryKey: queryKeys.integrations.registry(registryParams) }),
    queryClient.invalidateQueries({ queryKey: queryKeys.integrations.marketplace() }),
  ]);
}

export function useIntegrationMutations(registryParams = {}) {
  const queryClient = useQueryClient();

  const reloadMutation = useMutation({
    mutationFn: async () => unwrapIntegrationResponse(
      await reloadIntegrations(),
      'Failed to refresh integrations'
    ),
    onSuccess: async () => {
      await syncIntegrationCaches(queryClient, registryParams);
    },
  });

  const restartAllMutation = useMutation({
    mutationFn: async () => unwrapIntegrationResponse(
      await restartAllIntegrations(),
      'Failed to restart integrations'
    ),
  });

  const installMutation = useMutation({
    mutationFn: async ({ id, upstream, composePayload }) => unwrapIntegrationResponse(
      await installIntegration(id, upstream, composePayload),
      'Failed to install integration'
    ),
    onSuccess: async () => {
      await syncIntegrationCaches(queryClient, registryParams);
    },
  });

  const uninstallMutation = useMutation({
    mutationFn: async ({ id }) => unwrapIntegrationResponse(
      await uninstallIntegration(id),
      'Failed to uninstall integration'
    ),
    onSuccess: async () => {
      await syncIntegrationCaches(queryClient, registryParams);
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({ id }) => unwrapIntegrationResponse(
      await updateIntegration(id),
      'Failed to update integration'
    ),
    onSuccess: async () => {
      await syncIntegrationCaches(queryClient, registryParams);
    },
  });

  const setAutoUpdateMutation = useMutation({
    mutationFn: async ({ id, enabled }) => unwrapIntegrationResponse(
      await setIntegrationAutoUpdate(id, enabled),
      'Failed to update auto-update policy'
    ),
    onSuccess: async () => {
      await syncIntegrationCaches(queryClient, registryParams);
    },
  });

  return {
    reloadMutation,
    restartAllMutation,
    installMutation,
    uninstallMutation,
    updateMutation,
    setAutoUpdateMutation,
  };
}
