import { useMemo } from 'react';

import { safeString } from '../../mapErsMeta';

export default function useDevicePalette({ devices }) {
  const devicesForPalette = useMemo(() => {
    const list = Array.isArray(devices) ? devices : [];
    return list
      .slice()
      .sort((a, b) => safeString(a?.displayName || a?.name).localeCompare(safeString(b?.displayName || b?.name)));
  }, [devices]);

  const deviceByKey = useMemo(() => {
    const m = new globalThis.Map();
    devicesForPalette.forEach(d => {
      const ersId = safeString(d?.ersId);
      const id = safeString(d?.id || d?.hdpId);
      if (ersId && !m.has(ersId)) m.set(ersId, d);
      if (id && !m.has(id)) m.set(id, d);
    });
    return m;
  }, [devicesForPalette]);

  return { devicesForPalette, deviceByKey };
}
