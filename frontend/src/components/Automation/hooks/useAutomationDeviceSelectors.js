import { useMemo } from 'react';

import { normalizeDeviceLabel } from '../automationUtils';

export default function useAutomationDeviceSelectors({ devices, groups, selectedNode }) {
  const deviceOptions = useMemo(() => {
    const items = Array.isArray(devices) ? devices : [];
    return items
      .map(d => {
        const id = d?.device_id || d?.external_id || d?.id;
        if (!id) return null;
        return { id: String(id), label: normalizeDeviceLabel(d), raw: d };
      })
      .filter(Boolean)
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [devices]);

  const groupOptions = useMemo(() => {
    const items = Array.isArray(groups) ? groups : [];
    return items
      .map((group) => {
        const id = group?.id;
        const slug = typeof group?.slug === 'string' ? group.slug.trim() : '';
        if (!id || !slug) return null;
        const count = Array.isArray(group?.devices) ? group.devices.length : 0;
        const name = typeof group?.name === 'string' ? group.name.trim() : '';
        return {
          id: String(id),
          slug,
          selector: `group:${slug}`,
          label: count > 0 ? `${name || slug} (${count})` : (name || slug),
          raw: group,
        };
      })
      .filter(Boolean)
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [groups]);

  const deviceNameById = useMemo(() => {
    const items = Array.isArray(devices) ? devices : [];
    const m = new Map();
    items.forEach((d) => {
      const id = d?.device_id || d?.external_id || d?.id;
      if (!id) return;
      const name = typeof d?.name === 'string' ? d.name.trim() : '';
      m.set(String(id), name || String(id));
    });
    return m;
  }, [devices]);

  const deviceById = useMemo(() => {
    const m = new Map();
    deviceOptions.forEach(d => m.set(String(d.id), d));
    return m;
  }, [deviceOptions]);

  const triggerKeyOptions = useMemo(() => {
    if (!selectedNode || String(selectedNode.kind || '') !== 'trigger.device_state') return [];
    const targetsType = String(selectedNode?.data?.targets?.type || 'device').toLowerCase();
    const deviceId = targetsType === 'device' ? String(selectedNode?.data?.targets?.ids?.[0] || '').trim() : '';
    if (!deviceId) return [];
    const dev = deviceById.get(deviceId)?.raw;
    const state = dev?.state;
    if (!state || typeof state !== 'object') return [];
    return Object.keys(state).sort();
  }, [deviceById, selectedNode]);

  return {
    deviceOptions,
    groupOptions,
    deviceNameById,
    deviceById,
    triggerKeyOptions,
  };
}
