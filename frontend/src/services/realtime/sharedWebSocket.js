const GLOBAL_KEY = '__homenaviSharedWebSockets__';

function getStore() {
  const g = globalThis;
  if (!g[GLOBAL_KEY]) {
    g[GLOBAL_KEY] = new Map();
  }
  return g[GLOBAL_KEY];
}

export function wsUrlForPath(path) {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const normalized = String(path || '').startsWith('/') ? String(path || '') : `/${String(path || '')}`;
  return `${proto}://${window.location.host}${normalized}`;
}

function computeBackoffMs(attempt) {
  const capped = Math.min(Math.max(0, attempt), 6);
  return Math.min(30_000, 1000 * (2 ** capped));
}

class SharedWebSocket {
  constructor(url) {
    this.url = url;
    this.ws = null;
    this.status = 'idle'; // idle|connecting|open|closed|error
    this.reconnectAttempt = 0;
    this.reconnectTimer = null;
    this.idleCloseTimer = null;

    this.messageListeners = new Set();
    this.statusListeners = new Set();
  }

  _emitStatus(next, detail) {
    this.status = next;
    for (const cb of this.statusListeners) {
      try {
        cb({ status: next, detail, url: this.url });
      } catch {
        // ignore
      }
    }
  }

  _clearReconnectTimer() {
    if (this.reconnectTimer) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  _clearIdleCloseTimer() {
    if (this.idleCloseTimer) {
      window.clearTimeout(this.idleCloseTimer);
      this.idleCloseTimer = null;
    }
  }

  _scheduleReconnect() {
    this._clearReconnectTimer();
    if (this.messageListeners.size === 0) {
      return;
    }
    const delay = computeBackoffMs(this.reconnectAttempt);
    this.reconnectAttempt = Math.min(this.reconnectAttempt + 1, 6);
    this.reconnectTimer = window.setTimeout(() => this._connect(), delay);
  }

  _connect() {
    if (this.messageListeners.size === 0) {
      return;
    }
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    this._clearIdleCloseTimer();
    this._emitStatus('connecting');

    let ws;
    try {
      ws = new WebSocket(this.url);
    } catch (err) {
      this._emitStatus('error', err);
      this._scheduleReconnect();
      return;
    }

    this.ws = ws;

    ws.onopen = () => {
      this.reconnectAttempt = 0;
      this._emitStatus('open');
    };

    ws.onmessage = (ev) => {
      for (const cb of this.messageListeners) {
        try {
          cb(ev);
        } catch {
          // ignore
        }
      }
    };

    ws.onerror = (err) => {
      // Some browsers fire onerror with no useful info; onclose handles retry.
      this._emitStatus('error', err);
      try {
        ws.close();
      } catch {
        // ignore
      }
    };

    ws.onclose = () => {
      this._emitStatus('closed');
      this.ws = null;
      this._scheduleReconnect();
    };
  }

  subscribe(onMessage) {
    if (typeof onMessage !== 'function') {
      return () => {};
    }

    this.messageListeners.add(onMessage);
    this._clearIdleCloseTimer();
    this._connect();

    return () => {
      this.messageListeners.delete(onMessage);
      if (this.messageListeners.size === 0) {
        this._scheduleIdleClose();
      }
    };
  }

  onStatus(onStatus) {
    if (typeof onStatus !== 'function') {
      return () => {};
    }
    this.statusListeners.add(onStatus);
    try {
      onStatus({ status: this.status, url: this.url });
    } catch {
      // ignore
    }
    return () => {
      this.statusListeners.delete(onStatus);
    };
  }

  _scheduleIdleClose() {
    this._clearIdleCloseTimer();
    this._clearReconnectTimer();

    // Give React route transitions a bit of time so we don't thrash sockets.
    this.idleCloseTimer = window.setTimeout(() => {
      if (this.messageListeners.size !== 0) return;
      try {
        this.ws?.close();
      } catch {
        // ignore
      }
      this.ws = null;
      this._emitStatus('idle');
      // Manager will delete us when asked.
    }, 10_000);
  }

  send(data) {
    const ws = this.ws;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      return false;
    }
    try {
      ws.send(data);
      return true;
    } catch {
      return false;
    }
  }
}

export function getSharedWebSocket(url) {
  const key = String(url || '').trim();
  if (!key) {
    throw new Error('getSharedWebSocket requires a url');
  }

  const store = getStore();
  let entry = store.get(key);
  if (!entry) {
    entry = new SharedWebSocket(key);
    store.set(key, entry);
  }
  return entry;
}
