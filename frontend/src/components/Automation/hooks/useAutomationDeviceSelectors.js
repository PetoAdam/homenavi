import { useMemo } from 'react';

import { normalizeDeviceLabel } from '../automationUtils';
import { capabilitiesForDevice, compatibleCapabilities } from '../capabilityModel';

export default function useAutomationDeviceSelectors({ devices, groups, tags, selectedNode }) {
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

  const tagOptions = useMemo(() => {
    const items = Array.isArray(tags) ? tags : [];
    const allDevices = Array.isArray(devices) ? devices : [];
    return items.map((tag) => {
      const id = String(tag?.id || '').trim();
      const slug = String(tag?.slug || '').trim();
      if (!id || !slug) return null;
      const members = allDevices.filter(device => (Array.isArray(device?.tags) ? device.tags : []).some(deviceTag => String(deviceTag?.id || '').trim() === id));
      return { id, slug, selector: `tag:${slug}`, label: `${String(tag?.name || slug).trim()} (${members.length})`, raw: tag, members };
    }).filter(Boolean).sort((a, b) => a.label.localeCompare(b.label));
  }, [devices, tags]);

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

  const selectedCapabilities = useMemo(() => {
    if (!selectedNode) return [];
    const targetsType = String(selectedNode?.data?.targets?.type || 'device').toLowerCase();
    const deviceId = targetsType === 'device' ? String(selectedNode?.data?.targets?.ids?.[0] || '').trim() : '';
    if (deviceId) {
      const device = deviceById.get(deviceId)?.raw;
      return capabilitiesForDevice(device).filter(capability => capability.readable);
    }
    if (targetsType !== 'selector') return [];
    const selector = String(selectedNode?.data?.targets?.selector || '').trim().toLowerCase();
    const group = groupOptions.find(item => item.selector.toLowerCase() === selector)?.raw;
    const tag = tagOptions.find(item => item.selector.toLowerCase() === selector);
    const members = group ? (Array.isArray(group.devices) ? group.devices : []) : (tag?.members || []);
    return compatibleCapabilities(members).filter(capability => capability.readable);
  }, [deviceById, groupOptions, selectedNode, tagOptions]);

  const triggerKeyOptions = useMemo(() => selectedCapabilities.map(capability => capability.property), [selectedCapabilities]);

  return {
    deviceOptions,
    groupOptions,
    tagOptions,
    deviceNameById,
    deviceById,
    selectedCapabilities,
    compatibleCapabilities,
    triggerKeyOptions,
  };
}
