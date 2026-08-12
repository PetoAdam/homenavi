import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getDashboard, updateDashboard, getWidgetCatalog } from '../services/dashboardService';
import { getWidgetDefaultHeight, listLocalWidgetCatalog } from '../components/Home/Dashboard/widgetRegistry';
import { clearStaleResourceCache, readStaleResourceCache, writeStaleResourceCache } from '../utils/staleResourceCache';
import { queryKeys } from '../state/queryKeys';

/**
 * useDashboard - Custom hook for dashboard state management
 * 
 * Handles:
 * - Loading dashboard from backend
 * - Saving dashboard with debounce
 * - Optimistic updates
 * - Version conflict resolution
 */

const SAVE_DEBOUNCE_MS = 800;
const DASHBOARD_CACHE_TTL_MS = 30 * 1000;

function asDashboardData(response, fallbackMessage) {
  if (response?.success) return response.data;
  throw new Error(response?.error || fallbackMessage);
}

export function dashboardScopeFromAccessToken(accessToken) {
  return accessToken ? accessToken.slice(-16) : 'anonymous';
}

export function parseDashboardDoc(dashboard) {
  if (!dashboard || !dashboard.doc) return { layouts: {}, items: [] };

  let doc = dashboard.doc;
  if (typeof doc === 'string') {
    try {
      doc = JSON.parse(doc);
    } catch {
      return { layouts: {}, items: [] };
    }
  }

  return {
    layouts: doc.layouts || {},
    items: Array.isArray(doc.items) ? doc.items : [],
  };
}

export function mergeWidgetCatalogData(remoteData) {
  const base = Array.isArray(remoteData) ? remoteData : [];
  const local = listLocalWidgetCatalog();
  const knownIds = new Set(base.map((item) => item?.id).filter(Boolean));
  return [...base, ...local.filter((item) => item?.id && !knownIds.has(item.id))];
}

export function applyPendingDashboardDoc(dashboard, pendingDoc) {
  if (!dashboard || !pendingDoc) return dashboard;
  return {
    ...dashboard,
    doc: pendingDoc,
  };
}

export function shouldPersistDashboardCache({ queryEnabled, dashboard, pendingDoc }) {
  return Boolean(queryEnabled && dashboard && !pendingDoc);
}

export async function fetchWidgetCatalogData(accessToken) {
  const response = await getWidgetCatalog(accessToken);
  if (!response?.success) {
    return listLocalWidgetCatalog();
  }
  return mergeWidgetCatalogData(response.data);
}

export function dashboardQueryOptions(accessToken, { enabled = true, initialData } = {}) {
  const scope = dashboardScopeFromAccessToken(accessToken);
  return {
    queryKey: queryKeys.dashboard.me(scope),
    queryFn: async () => asDashboardData(
      await getDashboard(accessToken),
      'Failed to load dashboard'
    ),
    enabled,
    staleTime: DASHBOARD_CACHE_TTL_MS,
    initialData,
    initialDataUpdatedAt: typeof initialData !== 'undefined' ? 0 : undefined,
  };
}

export function widgetCatalogQueryOptions(accessToken, { enabled = true, initialData } = {}) {
  const scope = dashboardScopeFromAccessToken(accessToken);
  return {
    queryKey: queryKeys.dashboard.catalog(scope),
    queryFn: async () => fetchWidgetCatalogData(accessToken),
    enabled,
    staleTime: DASHBOARD_CACHE_TTL_MS,
    initialData,
    initialDataUpdatedAt: typeof initialData !== 'undefined' ? 0 : undefined,
  };
}

export default function useDashboard({ enabled, accessToken }) {
  const queryClient = useQueryClient();
  const saveTimeoutRef = useRef(null);
  const pendingDocRef = useRef(null);
  const scope = useMemo(() => dashboardScopeFromAccessToken(accessToken), [accessToken]);
  const cacheKey = accessToken ? `homenavi:dashboard:${scope}` : '';
  const queryEnabled = Boolean(enabled && accessToken);
  const cached = useMemo(
    () => (queryEnabled ? readStaleResourceCache(cacheKey, DASHBOARD_CACHE_TTL_MS) : null),
    [cacheKey, queryEnabled],
  );

  const dashboardQuery = useQuery(dashboardQueryOptions(accessToken, {
    enabled: queryEnabled,
    initialData: cached?.dashboard,
  }));

  const catalogQuery = useQuery(widgetCatalogQueryOptions(accessToken, {
    enabled: queryEnabled,
    initialData: Array.isArray(cached?.catalog) ? cached.catalog : undefined,
  }));

  const dashboard = queryEnabled ? (dashboardQuery.data ?? null) : null;
  const catalog = useMemo(() => {
    if (!queryEnabled) return [];
    return Array.isArray(catalogQuery.data) ? catalogQuery.data : listLocalWidgetCatalog();
  }, [catalogQuery.data, queryEnabled]);
  const doc = parseDashboardDoc(dashboard);

  const reload = useCallback(async () => {
    if (!queryEnabled) return;
    await Promise.all([dashboardQuery.refetch(), catalogQuery.refetch()]);
  }, [catalogQuery, dashboardQuery, queryEnabled]);

  const saveMutation = useMutation({
    mutationFn: async ({ currentVersion, newDoc }) => {
      const response = await updateDashboard(currentVersion, newDoc, accessToken);
      if (response?.success) {
        return { conflict: false, dashboard: response.data };
      }
      if (response?.status === 409) {
        return { conflict: true, dashboard: null };
      }
      throw new Error(response?.error || 'Failed to save dashboard');
    },
    onSuccess: async (result) => {
      if (result.conflict) {
        const pendingDoc = pendingDocRef.current;
        const [{ data: reloadedDashboard }] = await Promise.all([
          dashboardQuery.refetch(),
          catalogQuery.refetch(),
        ]);
        if (pendingDoc && reloadedDashboard) {
          queryClient.setQueryData(
            queryKeys.dashboard.me(scope),
            applyPendingDashboardDoc(reloadedDashboard, pendingDoc),
          );
        }
        return;
      }
      queryClient.setQueryData(queryKeys.dashboard.me(scope), result.dashboard);
      writeStaleResourceCache(cacheKey, {
        dashboard: result.dashboard,
        catalog,
      });
      pendingDocRef.current = null;
    },
  });

  useEffect(() => {
    if (!queryEnabled) {
      clearStaleResourceCache(cacheKey);
    }
  }, [cacheKey, queryEnabled]);

  useEffect(() => {
    if (!shouldPersistDashboardCache({ queryEnabled, dashboard, pendingDoc: pendingDocRef.current })) return;
    writeStaleResourceCache(cacheKey, {
      dashboard,
      catalog,
    });
  }, [cacheKey, catalog, dashboard, queryEnabled]);

  useEffect(() => {
    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current);
        saveTimeoutRef.current = null;
      }
    };
  }, []);
  
  // Public save function with debounce
  const saveDoc = useCallback((newDoc, options = {}) => {
    const { immediate = false } = options;
    
    if (!dashboard) return;
    
    pendingDocRef.current = newDoc;
    const currentVersion = dashboard.layout_version;
    
    // Optimistic update
    queryClient.setQueryData(queryKeys.dashboard.me(scope), (prev) => (prev ? {
      ...prev,
      doc: newDoc,
    } : prev));
    
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }
    
    if (immediate) {
      void saveMutation.mutateAsync({ currentVersion, newDoc });
    } else {
      saveTimeoutRef.current = setTimeout(() => {
        if (pendingDocRef.current) {
          void saveMutation.mutateAsync({ currentVersion, newDoc: pendingDocRef.current });
        }
      }, SAVE_DEBOUNCE_MS);
    }
  }, [dashboard, queryClient, saveMutation, scope]);
  
  // Flush any pending saves (call when leaving edit mode)
  const flushSave = useCallback(() => {
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
      saveTimeoutRef.current = null;
    }
    
    if (pendingDocRef.current && dashboard) {
      void saveMutation.mutateAsync({
        currentVersion: dashboard.layout_version,
        newDoc: pendingDocRef.current,
      });
    }
  }, [dashboard, saveMutation]);
  
  // Update layout (from grid changes)
  const updateLayouts = useCallback((newLayouts) => {
    const newDoc = {
      ...doc,
      layouts: newLayouts,
    };
    saveDoc(newDoc);
  }, [doc, saveDoc]);
  
  // Add a widget
  const addWidget = useCallback((widgetType, initialSettings = {}) => {
    const instanceId = crypto.randomUUID();
    
    const newItem = {
      instance_id: instanceId,
      widget_type: widgetType,
      enabled: true,
      settings: initialSettings,
    };
    
    // Add to all layouts at position 0,0 (will be compacted)
    const newLayouts = { ...doc.layouts };

    const breakpoints = Object.keys(newLayouts || {});
    const targetBps = breakpoints.length > 0 ? breakpoints : ['lg', 'md', 'sm', 'xxs'];
    const defaultH = getWidgetDefaultHeight(widgetType, catalog);

    targetBps.forEach((bp) => {
      const existing = Array.isArray(newLayouts[bp]) ? newLayouts[bp] : [];
      newLayouts[bp] = [{ i: instanceId, x: 0, y: 0, w: 1, h: defaultH }, ...existing];
    });
    
    const newDoc = {
      layouts: newLayouts,
      items: [...doc.items, newItem],
    };
    
    saveDoc(newDoc);
    return instanceId;
  }, [catalog, doc, saveDoc]);
  
  // Remove a widget
  const removeWidget = useCallback((instanceId) => {
    const newLayouts = {};
    
    Object.entries(doc.layouts).forEach(([bp, items]) => {
      newLayouts[bp] = items.filter((item) => item.i !== instanceId);
    });
    
    const newItems = doc.items.filter((item) => item.instance_id !== instanceId);
    
    const newDoc = {
      layouts: newLayouts,
      items: newItems,
    };
    
    saveDoc(newDoc);
  }, [doc, saveDoc]);
  
  // Update widget settings
  const updateWidgetSettings = useCallback((instanceId, newSettings) => {
    const newItems = doc.items.map((item) => {
      if (item.instance_id !== instanceId) return item;
      return {
        ...item,
        settings: { ...(item.settings || {}), ...newSettings },
      };
    });
    
    const newDoc = {
      ...doc,
      items: newItems,
    };
    
    saveDoc(newDoc);
  }, [doc, saveDoc]);
  
  // Get widget by instance ID
  const getWidget = useCallback((instanceId) => {
    return doc.items.find((item) => item.instance_id === instanceId) || null;
  }, [doc]);
  
  return {
    dashboard,
    doc,
    catalog,
    loading: queryEnabled ? (dashboardQuery.isLoading || catalogQuery.isLoading) : false,
    saving: saveMutation.isPending,
    error: queryEnabled ? (saveMutation.error?.message || dashboardQuery.error?.message || '') : '',
    reload,
    updateLayouts,
    addWidget,
    removeWidget,
    updateWidgetSettings,
    getWidget,
    flushSave,
  };
}
