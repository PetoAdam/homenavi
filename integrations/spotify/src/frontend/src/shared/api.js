function resolveIntegrationBasePath() {
  if (typeof window === 'undefined') return '';
  const path = window.location.pathname || '';
  const parts = path.split('/').filter(Boolean);
  const idx = parts.indexOf('integrations');
  if (idx >= 0 && parts[idx + 1]) {
    return `/${['integrations', parts[idx + 1]].join('/')}`;
  }
  return '';
}

function buildUrl(path) {
  const base = resolveIntegrationBasePath();
  if (!path.startsWith('/')) return `${base}/${path}`;
  return `${base}${path}`;
}

async function jsonRequest(path, options = {}) {
  const resp = await fetch(buildUrl(path), options);
  if (resp.status === 204) return null;
  const text = await resp.text();
  if (!resp.ok) {
    const message = text || 'Request failed';
    throw new Error(message);
  }
  if (!text) return null;
  const contentType = resp.headers.get('content-type') || '';
  if (!contentType.includes('application/json')) {
    return text;
  }
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export function getState() {
  return jsonRequest('/api/state');
}

export function getQueue() {
  return jsonRequest('/api/queue');
}

export function getDevices() {
  return jsonRequest('/api/devices');
}

export function play(payload = {}) {
  return jsonRequest('/api/play', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function pause() {
  return jsonRequest('/api/pause', { method: 'POST' });
}

export function nextTrack() {
  return jsonRequest('/api/next', { method: 'POST' });
}

export function previousTrack() {
  return jsonRequest('/api/previous', { method: 'POST' });
}

export function setShuffle(state) {
  return jsonRequest('/api/shuffle', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ state }),
  });
}

export function setRepeat(state) {
  return jsonRequest('/api/repeat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ state }),
  });
}

export function setVolume(volumePercent) {
  return jsonRequest('/api/volume', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ volume_percent: volumePercent }),
  });
}

export function seek(positionMs) {
  return jsonRequest('/api/seek', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ position_ms: positionMs }),
  });
}

export function addToQueue(uri, deviceId) {
  return jsonRequest('/api/queue/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ uri, device_id: deviceId }),
  });
}

export function transferPlayback(deviceId, playOnTransfer = true) {
  return jsonRequest('/api/transfer', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device_id: deviceId, play: playOnTransfer }),
  });
}

export function searchTracks(query) {
  const params = new URLSearchParams({ q: query });
  return jsonRequest(`/api/search?${params.toString()}`);
}
