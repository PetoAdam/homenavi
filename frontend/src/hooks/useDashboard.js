import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getDashboard, updateDashboard, getWidgetCatalog } from '../services/dashboardService';
import {
  buildDenseLayout,
  DASHBOARD_COLUMN_MODES,
  normalizeLayoutHeightsByMode,
  parseDashboardDocShape,
  serializeDashboardDoc,
} from '../components/Home/Dashboard/dashboardLayoutModel';
import {
  getWidgetDefaultHeight,
  getWidgetPreferredWidth,
  listLocalWidgetCatalog,
} from '../components/Home/Dashboard/widgetRegistry';
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
  if (!dashboard || !dashboard.doc) return parseDashboardDocShape({});

  let doc = dashboard.doc;
  if (typeof doc === 'string') {
    try {
      doc = JSON.parse(doc);
    } catch {
      return parseDashboardDocShape({});
    }
  }

  return parseDashboardDocShape(doc);
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

export function getDashboardVersionForSave(cachedDashboard, fallbackDashboard) {
  const version = cachedDashboard?.layout_version ?? fallbackDashboard?.layout_version;
  return Number.isFinite(version) ? version : null;
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
  const inFlightDocRef = useRef(null);
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

  const buildLayoutsForItems = useCallback((items, sourceLayoutsByCols) => {
    const safeItems = Array.isArray(items) ? items : [];
    const instanceIds = safeItems.map((item) => item?.instance_id).filter(Boolean);
    const widgetTypeByInstanceId = new Map();
    const defaultHeightByInstanceId = new Map();

    safeItems.forEach((item) => {
      if (!item?.instance_id) return;
      widgetTypeByInstanceId.set(item.instance_id, item.widget_type);
      defaultHeightByInstanceId.set(
        item.instance_id,
        getWidgetDefaultHeight(item.widget_type, catalog),
      );
    });

    const nextLayouts = {};
    DASHBOARD_COLUMN_MODES.forEach((columnMode) => {
      nextLayouts[columnMode] = buildDenseLayout({
        sourceLayout: sourceLayoutsByCols?.[columnMode] || [],
        instanceIds,
        columnMode,
        widgetTypeByInstanceId,
        preferredWidthForType: (widgetType, cols) => getWidgetPreferredWidth(widgetType, catalog, String(cols)),
        defaultHeightByInstanceId,
      });
    });

    return nextLayouts;
  }, [catalog]);

  const reload = useCallback(async () => {
    if (!queryEnabled) return;
    await Promise.all([dashboardQuery.refetch(), catalogQuery.refetch()]);
  }, [catalogQuery, dashboardQuery, queryEnabled]);

  const flushPendingSaveRef = useRef(() => {});

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
    onSuccess: async (result, variables) => {
      if (result.conflict) {
        const pendingDoc = pendingDocRef.current;
        inFlightDocRef.current = null;
        const [{ data: reloadedDashboard }] = await Promise.all([
          dashboardQuery.refetch(),
          catalogQuery.refetch(),
        ]);
        if (pendingDoc && reloadedDashboard) {
          queryClient.setQueryData(
            queryKeys.dashboard.me(scope),
            applyPendingDashboardDoc(reloadedDashboard, pendingDoc),
          );
          flushPendingSaveRef.current();
        }
        return;
      }

      inFlightDocRef.current = null;
      if (pendingDocRef.current === variables.newDoc) {
        queryClient.setQueryData(queryKeys.dashboard.me(scope), result.dashboard);
        writeStaleResourceCache(cacheKey, {
          dashboard: result.dashboard,
          catalog,
        });
        pendingDocRef.current = null;
        return;
      }

      queryClient.setQueryData(
        queryKeys.dashboard.me(scope),
        applyPendingDashboardDoc(result.dashboard, pendingDocRef.current),
      );
      flushPendingSaveRef.current();
    },
  });

  const flushPendingSave = useCallback(() => {
    if (!pendingDocRef.current || inFlightDocRef.current) return;

    const currentDashboard = queryClient.getQueryData(queryKeys.dashboard.me(scope));
    const currentVersion = getDashboardVersionForSave(currentDashboard, dashboard);
    if (!Number.isFinite(currentVersion)) return;

    const docToSave = pendingDocRef.current;
    inFlightDocRef.current = docToSave;
    void saveMutation.mutateAsync({ currentVersion, newDoc: docToSave });
  }, [dashboard, queryClient, saveMutation, scope]);

  flushPendingSaveRef.current = flushPendingSave;

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

    const serializedDoc = serializeDashboardDoc(newDoc);
    
    pendingDocRef.current = serializedDoc;
    const currentVersion = dashboard.layout_version;
    
    // Optimistic update
    queryClient.setQueryData(queryKeys.dashboard.me(scope), (prev) => (prev ? {
      ...prev,
      doc: serializedDoc,
    } : prev));
    
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }
    
    if (immediate) {
      flushPendingSave();
    } else {
      saveTimeoutRef.current = setTimeout(() => {
        flushPendingSave();
      }, SAVE_DEBOUNCE_MS);
    }
  }, [dashboard, flushPendingSave, queryClient, scope]);
  
  // Flush any pending saves (call when leaving edit mode)
  const flushSave = useCallback(() => {
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
      saveTimeoutRef.current = null;
    }
    
    if (pendingDocRef.current && dashboard) {
      flushPendingSave();
    }
  }, [dashboard, flushPendingSave]);
  
  // Update layout (from grid changes)
  const updateLayouts = useCallback((newLayouts) => {
    const newDoc = {
      ...doc,
      layoutsByCols: newLayouts,
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
    
    const nextItems = [...doc.items, newItem];
    const defaultH = getWidgetDefaultHeight(widgetType, catalog);
    const nextLayouts = buildLayoutsForItems(nextItems, {
      ...doc.layoutsByCols,
      '4': [{ i: instanceId, x: 0, y: 0, w: Math.min(2, 4), h: defaultH }, ...(doc.layoutsByCols?.['4'] || [])],
      '3': [{ i: instanceId, x: 0, y: 0, w: Math.min(getWidgetPreferredWidth(widgetType, catalog, '3'), 3), h: defaultH }, ...(doc.layoutsByCols?.['3'] || [])],
      '2': [{ i: instanceId, x: 0, y: 0, w: Math.min(getWidgetPreferredWidth(widgetType, catalog, '2'), 2), h: defaultH }, ...(doc.layoutsByCols?.['2'] || [])],
      '1': [{ i: instanceId, x: 0, y: 0, w: 1, h: defaultH }, ...(doc.layoutsByCols?.['1'] || [])],
    });
    
    const newDoc = {
      layoutsByCols: nextLayouts,
      items: nextItems,
    };
    
    saveDoc(newDoc);
    return instanceId;
  }, [catalog, doc, saveDoc]);
  
  // Remove a widget
  const removeWidget = useCallback((instanceId) => {
    const newLayouts = {};
    
    Object.entries(doc.layoutsByCols).forEach(([columnMode, items]) => {
      newLayouts[columnMode] = items.filter((item) => item.i !== instanceId);
    });
    
    const newItems = doc.items.filter((item) => item.instance_id !== instanceId);
    
    const newDoc = {
      layoutsByCols: newLayouts,
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
    saveParsedDoc: saveDoc,
    updateLayouts,
    addWidget,
    removeWidget,
    updateWidgetSettings,
    getWidget,
    flushSave,
  };
}
