import { useEffect } from 'react';

import useEditorHistory from '../../../../hooks/useEditorHistory';

export default function usePersistedLayout({ storageKey, snapshotForSave }) {
  const history = useEditorHistory({
    initialEditor: () => {
      try {
        const raw = window.localStorage.getItem(storageKey);
        if (!raw) return { rooms: [], devicePlacements: {} };
        const parsed = JSON.parse(raw);
        const rooms = Array.isArray(parsed?.rooms) ? parsed.rooms : [];
        const devicePlacements = parsed?.devicePlacements && typeof parsed.devicePlacements === 'object'
          ? parsed.devicePlacements
          : {};
        return { rooms, devicePlacements };
      } catch {
        return { rooms: [], devicePlacements: {} };
      }
    },
    snapshotForSave,
  });

  useEffect(() => {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(history.editor));
    } catch {
      // ignore
    }
  }, [history.editor, storageKey]);

  return history;
}
