import { useCallback, useEffect, useRef, useState } from 'react';
import { getDashboard, updateDashboard, getWidgetCatalog } from '../services/dashboardService';
import { getWidgetDefaultHeight, listLocalWidgetCatalog } from '../components/Home/Dashboard/widgetRegistry';

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

export default function useDashboard({ enabled, accessToken }) {
  const [dashboard, setDashboard] = useState(null);
  const [catalog, setCatalog] = useState([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const isMountedRef = useRef(false);
  const saveTimeoutRef = useRef(null);
  const pendingDocRef = useRef(null);
  
  // Parse doc from dashboard
  const parseDoc = useCallback((d) => {
    if (!d || !d.doc) return { layouts: {}, items: [] };
    
    let doc = d.doc;
    // Handle string JSON
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
  }, []);
  
  // Get current doc
  const doc = parseDoc(dashboard);

  const load = useCallback(async () => {
    if (!enabled || !accessToken) {
      if (isMountedRef.current) {
        setLoading(false);
      }
      return;
    }

    setLoading(true);
    setError('');

    try {
      const [dashRes, catRes] = await Promise.all([
        getDashboard(accessToken),
        getWidgetCatalog(accessToken),
      ]);

      if (!isMountedRef.current) return;

      if (!dashRes.success) {
        setError(dashRes.error || 'Failed to load dashboard');
        setDashboard(null);
      } else {
        setDashboard(dashRes.data);
      }

      if (catRes.success) {
        setCatalog(Array.isArray(catRes.data) ? catRes.data : []);
      } else {
        // Temporary fallback until backend catalog is always available.
        setCatalog(listLocalWidgetCatalog());
      }
    } catch (err) {
      if (!isMountedRef.current) return;
      setError(err?.message || 'Failed to load dashboard');
      setCatalog(listLocalWidgetCatalog());
    } finally {
      if (isMountedRef.current) {
        setLoading(false);
      }
    }
  }, [enabled, accessToken]);

  useEffect(() => {
    isMountedRef.current = true;
    load();
    return () => {
      isMountedRef.current = false;
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current);
        saveTimeoutRef.current = null;
      }
    };
  }, [load]);
  
  // Internal save function
  const doSave = useCallback(async (newDoc, currentVersion) => {
    if (!accessToken || !dashboard) return;
    
    setSaving(true);
    
    try {
      const res = await updateDashboard(currentVersion, newDoc, accessToken);
      
      if (!isMountedRef.current) return;
      
      if (res.success) {
        setDashboard(res.data);
        pendingDocRef.current = null;
      } else if (res.status === 409) {
        // Version conflict - reload
        console.warn('Dashboard version conflict, reloading...');
        await load();
      } else {
        setError(res.error || 'Failed to save dashboard');
      }
    } catch (err) {
      if (!isMountedRef.current) return;
      setError(err.message || 'Failed to save dashboard');
    } finally {
      if (isMountedRef.current) {
        setSaving(false);
      }
    }
  }, [accessToken, dashboard, load]);
  
  // Public save function with debounce
  const saveDoc = useCallback((newDoc, options = {}) => {
    const { immediate = false } = options;
    
    if (!dashboard) return;
    
    pendingDocRef.current = newDoc;
    const currentVersion = dashboard.layout_version;
    
    // Optimistic update
    setDashboard((prev) => ({
      ...prev,
      doc: newDoc,
    }));
    
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }
    
    if (immediate) {
      doSave(newDoc, currentVersion);
    } else {
      saveTimeoutRef.current = setTimeout(() => {
        if (pendingDocRef.current) {
          doSave(pendingDocRef.current, currentVersion);
        }
      }, SAVE_DEBOUNCE_MS);
    }
  }, [dashboard, doSave]);
  
  // Flush any pending saves (call when leaving edit mode)
  const flushSave = useCallback(() => {
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
      saveTimeoutRef.current = null;
    }
    
    if (pendingDocRef.current && dashboard) {
      doSave(pendingDocRef.current, dashboard.layout_version);
    }
  }, [dashboard, doSave]);
  
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
    loading,
    saving,
    error,
    reload: load,
    updateLayouts,
    addWidget,
    removeWidget,
    updateWidgetSettings,
    getWidget,
    flushSave,
  };
}
