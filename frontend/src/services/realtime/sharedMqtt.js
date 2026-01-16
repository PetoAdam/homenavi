import Paho from 'paho-mqtt';

const GLOBAL_KEY = '__homenaviSharedMqtt__';

function getStore() {
  const g = globalThis;
  if (!g[GLOBAL_KEY]) {
    g[GLOBAL_KEY] = new Map();
  }
  return g[GLOBAL_KEY];
}

function resolveGatewayUrl() {
  const override = import.meta.env.VITE_GATEWAY_ORIGIN;
  try {
    if (override) {
      return new URL(override);
    }
  } catch (err) {
    console.warn('Invalid VITE_GATEWAY_ORIGIN, falling back to window.origin', err);
  }
  return new URL(window.location.origin);
}

function joinPath(basePath, suffix) {
  const base = (!basePath || basePath === '/') ? '' : basePath.replace(/\/$/, '');
  const next = String(suffix || '').startsWith('/') ? String(suffix || '') : `/${String(suffix || '')}`;
  return `${base}${next}`;
}

function buildWsConfig(path) {
  const gateway = resolveGatewayUrl();
  const useSSL = gateway.protocol === 'https:' || gateway.protocol === 'wss:';
  const port = gateway.port ? Number(gateway.port) : (useSSL ? 443 : 80);
  const fullPath = joinPath(gateway.pathname, path);
  return {
    host: gateway.hostname,
    port,
    path: fullPath,
    useSSL,
  };
}

function computeBackoffMs(attempt) {
  const capped = Math.min(Math.max(0, attempt), 6);
  return Math.min(30_000, 1500 * (2 ** capped));
}

function mqttTopicMatches(filter, topic) {
  const f = String(filter || '');
  const t = String(topic || '');
  if (!f || !t) return false;
  if (f === t) return true;

  const fp = f.split('/');
  const tp = t.split('/');

  for (let i = 0, j = 0; i < fp.length; i += 1, j += 1) {
    const seg = fp[i];
    if (seg === '#') {
      return true;
    }
    if (j >= tp.length) {
      return false;
    }
    if (seg === '+') {
      continue;
    }
    if (seg !== tp[j]) {
      return false;
    }
  }

  return fp[fp.length - 1] === '#' || fp.length === tp.length;
}

class SharedMqttConnection {
  constructor({ path, clientIdPrefix }) {
    this.cfg = buildWsConfig(path);
    this.path = path;
    this.clientIdPrefix = clientIdPrefix || 'rt';

    this.client = null;
    this.status = 'idle'; // idle|connecting|connected|disconnected|error
    this.reconnectAttempt = 0;
    this.reconnectTimer = null;
    this.idleDisconnectTimer = null;

    this.subscriptions = new Map(); // filter -> Set<handler>
    this.statusListeners = new Set();
  }

  _emitStatus(next, detail) {
    this.status = next;
    for (const cb of this.statusListeners) {
      try {
        cb({ status: next, connected: next === 'connected', detail });
      } catch {
        // ignore
      }
    }
  }

  _hasActiveListeners() {
    for (const set of this.subscriptions.values()) {
      if (set && set.size) return true;
    }
    return false;
  }

  _clearReconnectTimer() {
    if (this.reconnectTimer) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  _clearIdleDisconnectTimer() {
    if (this.idleDisconnectTimer) {
      window.clearTimeout(this.idleDisconnectTimer);
      this.idleDisconnectTimer = null;
    }
  }

  _scheduleReconnect() {
    this._clearReconnectTimer();
    if (!this._hasActiveListeners()) return;
    const delay = computeBackoffMs(this.reconnectAttempt);
    this.reconnectAttempt = Math.min(this.reconnectAttempt + 1, 6);
    this.reconnectTimer = window.setTimeout(() => this._connect(), delay);
  }

  _ensureClient() {
    if (this.client) return this.client;
    const id = `${this.clientIdPrefix}-${Date.now()}-${Math.floor(Math.random() * 1000)}`;
    const client = new Paho.Client(this.cfg.host, Number(this.cfg.port), this.cfg.path, id);
    this.client = client;

    client.onConnectionLost = (response) => {
      this._emitStatus('disconnected', response);
      if (this._hasActiveListeners()) {
        this._scheduleReconnect();
      }
    };

    client.onMessageArrived = (message) => {
      if (!message || !message.destinationName) return;
      const topic = message.destinationName;
      const payloadString = typeof message.payloadString === 'string' ? message.payloadString : '';
      const payloadBytes = message.payloadBytes;

      for (const [filter, handlers] of this.subscriptions.entries()) {
        if (!handlers || handlers.size === 0) continue;
        if (!mqttTopicMatches(filter, topic)) continue;
        for (const handler of handlers) {
          try {
            handler({ topic, payloadString, payloadBytes, raw: message });
          } catch {
            // ignore
          }
        }
      }
    };

    return client;
  }

  _connect() {
    this._clearIdleDisconnectTimer();
    if (!this._hasActiveListeners()) {
      return;
    }

    if (this.status === 'connecting') {
      return;
    }

    const client = this._ensureClient();
    if (typeof client.isConnected === 'function' && client.isConnected()) {
      this._emitStatus('connected');
      this._ensureBrokerSubscriptions();
      return;
    }

    this._emitStatus('connecting');

    try {
      client.connect({
        useSSL: this.cfg.useSSL,
        timeout: 6,
        mqttVersion: 4,
        cleanSession: true,
        onSuccess: () => {
          this.reconnectAttempt = 0;
          this._emitStatus('connected');
          this._ensureBrokerSubscriptions();
        },
        onFailure: (err) => {
          this._emitStatus('error', err);
          try {
            client.disconnect();
          } catch {
            // ignore
          }
          if (this._hasActiveListeners()) {
            this._scheduleReconnect();
          }
        },
      });
    } catch (err) {
      this._emitStatus('error', err);
      if (this._hasActiveListeners()) {
        this._scheduleReconnect();
      }
    }
  }

  _ensureBrokerSubscriptions() {
    const client = this.client;
    if (!client || typeof client.isConnected !== 'function' || !client.isConnected()) return;

    for (const [filter, handlers] of this.subscriptions.entries()) {
      if (!handlers || handlers.size === 0) continue;
      try {
        client.subscribe(filter, { qos: 0 });
      } catch {
        // ignore
      }
    }
  }

  onStatus(cb) {
    if (typeof cb !== 'function') return () => {};
    this.statusListeners.add(cb);
    try {
      cb({ status: this.status, connected: this.status === 'connected' });
    } catch {
      // ignore
    }
    return () => {
      this.statusListeners.delete(cb);
    };
  }

  subscribe(filter, handler) {
    const topicFilter = String(filter || '').trim();
    if (!topicFilter || typeof handler !== 'function') {
      return () => {};
    }

    this._clearIdleDisconnectTimer();

    if (!this.subscriptions.has(topicFilter)) {
      this.subscriptions.set(topicFilter, new Set());
    }
    const set = this.subscriptions.get(topicFilter);
    set.add(handler);

    this._connect();

    // If we are already connected, subscribe immediately.
    this._ensureBrokerSubscriptions();

    return () => {
      const current = this.subscriptions.get(topicFilter);
      if (current) {
        current.delete(handler);
        if (current.size === 0) {
          this.subscriptions.delete(topicFilter);
          try {
            this.client?.unsubscribe?.(topicFilter);
          } catch {
            // ignore
          }
        }
      }

      if (!this._hasActiveListeners()) {
        this._scheduleIdleDisconnect();
      }
    };
  }

  _scheduleIdleDisconnect() {
    this._clearReconnectTimer();
    this._clearIdleDisconnectTimer();

    // Keep it around briefly to avoid connect/disconnect thrash on route changes.
    this.idleDisconnectTimer = window.setTimeout(() => {
      if (this._hasActiveListeners()) return;
      try {
        this.client?.disconnect?.();
      } catch {
        // ignore
      }
      this.client = null;
      this._emitStatus('idle');
    }, 60_000);
  }

  publish(topic, payloadString, { qos = 0, retained = false } = {}) {
    const client = this.client;
    if (!client || typeof client.isConnected !== 'function' || !client.isConnected()) {
      throw new Error('Not connected');
    }
    const message = new Paho.Message(String(payloadString ?? ''));
    message.destinationName = String(topic || '');
    message.qos = qos;
    message.retained = retained;
    client.send(message);
  }

  isConnected() {
    const c = this.client;
    return Boolean(c && typeof c.isConnected === 'function' && c.isConnected());
  }
}

export function getSharedMqttConnection({ path, clientIdPrefix } = {}) {
  const p = String(path || '').trim();
  if (!p) throw new Error('getSharedMqttConnection requires { path }');

  const cfg = buildWsConfig(p);
  const key = `${cfg.host}:${cfg.port}${cfg.path}|ssl=${cfg.useSSL}`;

  const store = getStore();
  let entry = store.get(key);
  if (!entry) {
    entry = new SharedMqttConnection({ path: p, clientIdPrefix });
    store.set(key, entry);
  }
  return entry;
}
