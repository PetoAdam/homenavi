const NODE_WIDTH = 260;
const NODE_HEADER_HEIGHT = 40;

function safeJsonParse(value) {
  if (value == null) return null;
  if (typeof value === 'object') return value;
  if (typeof value !== 'string') return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

export function defaultWorkflowName() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, '0');
  return `Workflow ${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function normalizeTargets(raw) {
  const t = (raw && typeof raw === 'object') ? raw : {};
  const type = String(t.type || '').trim().toLowerCase();
  if (type === 'selector') {
    return { type: 'selector', selector: String(t.selector || '').trim(), ids: [] };
  }
  const ids = Array.isArray(t.ids) ? t.ids : [];
  const out = [];
  const seen = new Set();
  ids.forEach((x) => {
    const v = String(x || '').trim();
    if (!v) return;
    if (seen.has(v)) return;
    seen.add(v);
    out.push(v);
  });
  return { type: 'device', ids: out, selector: '' };
}

function describeTargets(targets) {
  const t = normalizeTargets(targets);
  if (t.type === 'selector') return t.selector ? `selector ${t.selector}` : 'selector —';
  if (t.ids.length === 1) return `device ${t.ids[0]}`;
  if (t.ids.length > 1) return `${t.ids.length} devices`;
  return 'device —';
}

export function defaultNodeData(kind) {
  const k = String(kind || '').toLowerCase();
  if (k === 'trigger.manual') {
    return {};
  }
  if (k === 'trigger.device_state') {
    return {
      targets: { type: 'device', ids: [], selector: '' },
      key: '',
      op: 'exists',
      value: null,
      cooldown_sec: 2,
      ignore_retained: true,
      ui: {
        value_mode: 'builder',
        value_type: 'boolean',
        value_bool: true,
        value_number: '',
        value_string: '',
        value_text: '',
      },
    };
  }
  if (k === 'trigger.schedule') {
    return {
      cron: '0 */5 * * * *',
      cooldown_sec: 1,
      ui: {
        schedule_mode: 'simple',
        schedule_preset: 'every_n_minutes',
        every_minutes: 5,
        at_minute: 0,
        at_time: '08:00',
        weekday: 1,
      },
    };
  }
  if (k === 'action.send_command') {
    return {
      targets: { type: 'device', ids: [], selector: '' },
      command: 'set_state',
      args: { state: 'ON' },
      wait_for_result: false,
      result_timeout_sec: 15,
      ui: {
        command_mode: 'set_state',
        args_mode: 'builder',
        args_text: '{\n  "state": "ON"\n}',
        state: 'ON',
        brightness: '',
        transition_ms: '',
        color_mode: 'none', // none|hs|color_temp
        hue: '',
        saturation: '',
        color_temp: '',
      },
    };
  }
  if (k === 'action.notify_email') {
    return {
      user_ids: [],
      target_roles: [],
      subject: '',
      message: '',
    };
  }
  if (k === 'logic.sleep') {
    return { duration_sec: 5 };
  }
  if (k === 'logic.if') {
    return {
      path: '',
      op: 'exists',
      value: null,
      ui: { value_text: '' },
    };
  }
  if (k === 'logic.for') {
    return { count: 3 };
  }
  return {};
}

export function parseWorkflowIntoEditor(wf) {
  const editor = {
    workflowName: defaultWorkflowName(),
    nodes: [],
    edges: [],
  };
  if (!wf) return editor;

  editor.workflowName = String(wf?.name || '').trim();

  const raw = safeJsonParse(wf?.definition) || {};
  const version = String(raw?.version || '').trim();

  const isDefinitionShape = Array.isArray(raw?.nodes) && Array.isArray(raw?.edges);
  if (version === 'automation' && isDefinitionShape) {
    editor.nodes = raw.nodes.map((n) => ({
      id: String(n?.id || ''),
      kind: String(n?.kind || ''),
      x: Number(n?.x || 0),
      y: Number(n?.y || 0),
      data: (n?.data && typeof n.data === 'object') ? n.data : {},
    })).filter((n) => n.id && n.kind);

    editor.edges = raw.edges
      .map((e) => ({ from: String(e?.from || ''), to: String(e?.to || '') }))
      .filter((e) => e.from && e.to);

    // Ensure defaults exist for known node kinds.
    editor.nodes = editor.nodes.map((n) => {
      const base = defaultNodeData(n.kind);
      const data = (n.data && typeof n.data === 'object') ? n.data : {};
      return { ...n, data: { ...base, ...data, ui: { ...(base.ui || {}), ...(data.ui || {}) } } };
    });

    return editor;
  }
  return editor;
}

export function buildDefinitionFromEditor(editor) {
  const nodes = Array.isArray(editor?.nodes) ? editor.nodes : [];
  const edges = Array.isArray(editor?.edges) ? editor.edges : [];

  const nodeById = new Map(nodes.map((n) => [String(n.id), n]));
  const triggers = nodes.filter((n) => String(n.kind || '').toLowerCase().startsWith('trigger.'));
  if (triggers.length === 0) {
    throw new Error('Add at least one trigger (drag a trigger onto the canvas)');
  }

  // Validate edges endpoints exist.
  const normEdges = edges
    .map((e) => ({ from: String(e?.from || ''), to: String(e?.to || '') }))
    .filter((e) => e.from && e.to && e.from !== e.to)
    .filter((e) => nodeById.has(e.from) && nodeById.has(e.to));

  // Prevent edges into triggers.
  for (const e of normEdges) {
    const to = nodeById.get(e.to);
    if (to && String(to.kind || '').toLowerCase().startsWith('trigger.')) {
      throw new Error('Cannot connect into trigger nodes');
    }
  }

  // Ensure action targets required.
  for (const n of nodes) {
    const kind = String(n.kind || '').toLowerCase();
    if (kind === 'action.send_command') {
      const targets = normalizeTargets(n?.data?.targets);
      if (targets.type === 'selector') {
        if (!targets.selector) throw new Error('Send Command node requires selector');
        if (n?.data?.wait_for_result) throw new Error('wait_for_result is not supported for selector targets');
      } else {
        if (targets.ids.length === 0) throw new Error('Send Command node requires a target device');
        if (n?.data?.wait_for_result && targets.ids.length !== 1) throw new Error('wait_for_result requires exactly one target device');
      }
    }
    if (kind === 'action.notify_email') {
      const ids = Array.isArray(n?.data?.user_ids) ? n.data.user_ids : [];
      const userIds = ids.map((x) => String(x || '').trim()).filter(Boolean);
      const roles = Array.isArray(n?.data?.target_roles) ? n.data.target_roles : [];
      const targetRoles = roles.map((x) => String(x || '').trim().toLowerCase()).filter(Boolean);
      const hasAnyTarget = userIds.length > 0 || targetRoles.length > 0;
      if (!hasAnyTarget) throw new Error('Notify Email node requires at least one user or group');
      const subject = String(n?.data?.subject || '').trim();
      const message = String(n?.data?.message || '').trim();
      if (!subject) throw new Error('Notify Email node requires subject');
      if (!message) throw new Error('Notify Email node requires message');
    }
    if (kind === 'trigger.device_state') {
      const targets = normalizeTargets(n?.data?.targets);
      if (targets.type === 'selector') {
        if (!targets.selector) throw new Error('Device state trigger requires selector');
      } else if (targets.ids.length === 0) {
        throw new Error('Device state trigger requires a target device');
      }
    }
    if (kind === 'trigger.schedule') {
      const cron = String(n?.data?.cron || '').trim();
      if (!cron) {
        throw new Error('Schedule trigger requires cron');
      }
    }
    if (kind === 'logic.if') {
      const path = String(n?.data?.path || '').trim();
      if (!path) {
        throw new Error('If node requires path');
      }
    }
  }

  // Ensure wait_for_result only on leaf nodes (matches backend rule).
  const outCount = {};
  normEdges.forEach((e) => {
    outCount[e.from] = (outCount[e.from] || 0) + 1;
  });
  for (const n of nodes) {
    const kind = String(n.kind || '').toLowerCase();
    if (kind !== 'action.send_command') continue;
    if (!n?.data?.wait_for_result) continue;
    if ((outCount[n.id] || 0) !== 0) {
      throw new Error('wait_for_result is only supported on leaf Send Command nodes');
    }
  }

  const normNodes = nodes.map((n) => {
    const base = defaultNodeData(n.kind);
    const data = (n.data && typeof n.data === 'object') ? n.data : {};

    // Normalize certain UI-backed fields into canonical keys.
    if (String(n.kind).toLowerCase() === 'action.send_command') {
      data.targets = normalizeTargets(data.targets);
      // If UI has raw json args text, parse into args.
      const mode = String(data?.ui?.args_mode || 'builder').toLowerCase();
      if (mode === 'json') {
        const txt = String(data?.ui?.args_text || '').trim();
        if (txt) {
          const parsed = safeJsonParse(txt);
          if (parsed == null || typeof parsed !== 'object' || Array.isArray(parsed)) {
            throw new Error('Action args must be a JSON object');
          }
          // Support either:
          // - args-only object: {"state":"ON"}
          // - full payload object: {"command":"set_state","args":{...}}
          const maybeCommand = typeof parsed.command === 'string' ? String(parsed.command).trim() : '';
          const maybeArgs = parsed.args;
          if (maybeCommand || Object.prototype.hasOwnProperty.call(parsed, 'args')) {
            if (maybeCommand) data.command = maybeCommand;
            if (maybeArgs != null) {
              if (typeof maybeArgs !== 'object' || Array.isArray(maybeArgs)) {
                throw new Error('Action args must be a JSON object');
              }
              data.args = maybeArgs;
            } else {
              data.args = {};
            }
          } else {
            data.args = parsed;
          }
        }
      }

      // Builder: derive args for set_state.
      const cmd = String(data?.command || '').trim() || 'set_state';
      data.command = cmd;
      if (mode === 'builder' && cmd === 'set_state') {
        const args = {};
        const st = String(data?.ui?.state || '').trim();
        if (st) args.state = st;
        const b = String(data?.ui?.brightness || '').trim();
        if (b !== '') {
          const bn = Number(b);
          if (Number.isFinite(bn)) args.brightness = bn;
        }
        const transitionMs = String(data?.ui?.transition_ms || '').trim();
        if (transitionMs !== '') {
          const tn = Number(transitionMs);
          if (Number.isFinite(tn)) args.transition_ms = Math.max(0, Math.floor(tn));
        }
        const colorMode = String(data?.ui?.color_mode || 'none').toLowerCase();
        if (colorMode === 'color_temp') {
          const ct = String(data?.ui?.color_temp || '').trim();
          if (ct !== '') {
            const ctn = Number(ct);
            if (Number.isFinite(ctn)) args.color_temp = Math.floor(ctn);
          }
        }
        if (colorMode === 'hs') {
          const h = String(data?.ui?.hue || '').trim();
          const s = String(data?.ui?.saturation || '').trim();
          const hn = h === '' ? NaN : Number(h);
          const sn = s === '' ? NaN : Number(s);
          if (Number.isFinite(hn) || Number.isFinite(sn)) {
            args.color = {};
            if (Number.isFinite(hn)) args.color.h = Math.max(0, Math.min(360, Math.floor(hn)));
            if (Number.isFinite(sn)) args.color.s = Math.max(0, Math.min(100, Math.floor(sn)));
          }
        }
        data.args = args;
      }
    }

    if (String(n.kind).toLowerCase() === 'logic.if') {
      const txt = String(data?.ui?.value_text || '').trim();
      if (txt) {
        const parsed = safeJsonParse(txt);
        if (parsed == null) {
          throw new Error('If value must be valid JSON');
        }
        data.value = parsed;
      } else {
        data.value = null;
      }
    }

    if (String(n.kind).toLowerCase() === 'trigger.device_state') {
      data.targets = normalizeTargets(data.targets);
      data.key = String(data?.key || '').trim();
      const op = String(data?.op || 'exists').trim().toLowerCase() || 'exists';
      data.op = op;
      const cooldown = Number(data?.cooldown_sec ?? 0);
      if (Number.isFinite(cooldown)) data.cooldown_sec = Math.max(0, Math.floor(cooldown));
      data.ignore_retained = !!data?.ignore_retained;

      // Support Builder mode (typed) by synthesizing JSON into ui.value_text.
      const ui = (data.ui && typeof data.ui === 'object') ? data.ui : {};
      const valueMode = String(ui.value_mode || (ui.value_text ? 'json' : 'builder')).toLowerCase();
      ui.value_mode = valueMode;
      if (op !== 'exists' && valueMode === 'builder') {
        const t = String(ui.value_type || 'boolean').toLowerCase();
        ui.value_type = t;
        if (t === 'boolean') {
          ui.value_text = JSON.stringify(!!ui.value_bool);
        } else if (t === 'number') {
          const raw = String(ui.value_number ?? '').trim();
          if (!raw) throw new Error('Trigger value is required');
          const n = Number(raw);
          if (!Number.isFinite(n)) throw new Error('Trigger value must be a valid number');
          ui.value_text = JSON.stringify(n);
        } else {
          ui.value_text = JSON.stringify(String(ui.value_string ?? ''));
        }
      }
      data.ui = ui;

      const txt = String(data?.ui?.value_text || '').trim();
      if (op !== 'exists' && txt) {
        const parsed = safeJsonParse(txt);
        if (parsed == null) {
          throw new Error('Trigger value must be valid JSON');
        }
        data.value = parsed;
      } else {
        data.value = null;
      }
    }

    if (String(n.kind).toLowerCase() === 'trigger.schedule') {
      const ui = (data.ui && typeof data.ui === 'object') ? data.ui : {};
      const mode = String(ui.schedule_mode || (data.cron ? 'cron' : 'simple')).toLowerCase();
      ui.schedule_mode = mode;

      const clampInt = (raw, min, max, fallback) => {
        const n = Number(raw);
        if (!Number.isFinite(n)) return fallback;
        const i = Math.floor(n);
        return Math.max(min, Math.min(max, i));
      };
      const parseHHMM = (raw, fallbackH = 8, fallbackM = 0) => {
        const v = String(raw || '').trim();
        const m = v.match(/^(\d{1,2}):(\d{2})$/);
        if (!m) return { h: fallbackH, min: fallbackM };
        const h = clampInt(m[1], 0, 23, fallbackH);
        const min = clampInt(m[2], 0, 59, fallbackM);
        return { h, min };
      };
      const buildCronFromSimple = (u) => {
        const preset = String(u?.schedule_preset || 'every_n_minutes');
        // sec min hour dom month dow
        if (preset === 'every_n_minutes') {
          const every = clampInt(u?.every_minutes, 1, 59, 5);
          return `0 */${every} * * * *`;
        }
        if (preset === 'hourly_at') {
          const minute = clampInt(u?.at_minute, 0, 59, 0);
          return `0 ${minute} * * * *`;
        }
        if (preset === 'daily_at') {
          const { h, min } = parseHHMM(u?.at_time, 8, 0);
          return `0 ${min} ${h} * * *`;
        }
        if (preset === 'weekly_at') {
          const { h, min } = parseHHMM(u?.at_time, 8, 0);
          const dow = clampInt(u?.weekday, 0, 6, 1);
          return `0 ${min} ${h} * * ${dow}`;
        }
        return '0 */5 * * * *';
      };

      if (mode === 'simple') {
        data.cron = buildCronFromSimple(ui);
      } else {
        data.cron = String(data?.cron || '').trim();
      }
      data.ui = ui;
      const cooldown = Number(data?.cooldown_sec ?? 0);
      if (Number.isFinite(cooldown)) data.cooldown_sec = Math.max(0, Math.floor(cooldown));
    }

    return {
      id: String(n.id),
      kind: String(n.kind),
      x: Number.isFinite(Number(n.x)) ? Number(n.x) : 0,
      y: Number.isFinite(Number(n.y)) ? Number(n.y) : 0,
      data: { ...base, ...data, ui: { ...(base.ui || {}), ...(data.ui || {}) } },
    };
  });

  return {
    version: 'automation',
    nodes: normNodes,
    edges: normEdges,
  };
}

export function editorSnapshotForSave(editor) {
  const def = buildDefinitionFromEditor(editor);
  return JSON.stringify({ name: String(editor?.workflowName || '').trim(), def });
}

export function nodeTitle(kind) {
  const k = String(kind || '').toLowerCase();
  if (k.startsWith('trigger.')) return 'Trigger';
  if (k === 'action.send_command') return 'Action';
  if (k === 'action.notify_email') return 'Action';
  if (k === 'logic.if') return 'If';
  if (k === 'logic.sleep') return 'Sleep';
  if (k === 'logic.for') return 'For';
  return 'Node';
}

export function nodeSubtitle(node) {
  const kind = String(node?.kind || '').toLowerCase();
  if (kind === 'trigger.manual') return 'Manual';
  if (kind === 'trigger.device_state') {
    return `Device state • ${describeTargets(node?.data?.targets)}`;
  }
  if (kind === 'trigger.schedule') {
    const c = String(node?.data?.cron || '').trim();
    return c ? `Cron: ${c}` : 'Schedule';
  }
  if (kind === 'action.send_command') {
    const c = String(node?.data?.command || '').trim();
    return `${describeTargets(node?.data?.targets)}${c ? ` • ${c}` : ''}`;
  }
  if (kind === 'action.notify_email') {
    const ids = Array.isArray(node?.data?.user_ids) ? node.data.user_ids : [];
    const n = ids.map((x) => String(x || '').trim()).filter(Boolean).length;
    return n ? `Email ${n} user${n === 1 ? '' : 's'}` : 'Notify email';
  }
  if (kind === 'logic.if') {
    const p = String(node?.data?.path || '').trim();
    const op = String(node?.data?.op || 'exists').trim() || 'exists';
    return p ? `if ${p} ${op}` : 'Condition';
  }
  if (kind === 'logic.sleep') {
    const s = Number(node?.data?.duration_sec || 0);
    return `Sleep ${s || 0}s`;
  }
  if (kind === 'logic.for') {
    const c = Number(node?.data?.count || 0);
    return `Repeat ${c || 0}x`;
  }
  return '';
}

export function nodeBodyText(node) {
  const kind = String(node?.kind || '').toLowerCase();
  if (kind === 'trigger.manual') return 'Click Run to fire';
  if (kind === 'trigger.device_state') {
    const key = String(node?.data?.key || '').trim();
    const op = String(node?.data?.op || 'exists').trim() || 'exists';
    return `${describeTargets(node?.data?.targets)}${key ? ` • ${key} ${op}` : ''}`;
  }
  if (kind === 'trigger.schedule') {
    const c = String(node?.data?.cron || '').trim();
    return c ? `cron: ${c}` : 'cron: —';
  }
  if (kind === 'action.send_command') {
    const c = String(node?.data?.command || '').trim() || 'set_state';
    return `${describeTargets(node?.data?.targets)} • cmd: ${c}`;
  }
  if (kind === 'action.notify_email') {
    const subject = String(node?.data?.subject || '').trim();
    return subject ? `subject: ${subject}` : 'subject: —';
  }
  if (kind === 'logic.sleep') {
    const s = Number(node?.data?.duration_sec || 0);
    return `wait: ${s || 0} sec`;
  }
  if (kind === 'logic.if') {
    const p = String(node?.data?.path || '—');
    const op = String(node?.data?.op || 'exists');
    return `if: ${p} ${op}`;
  }
  if (kind === 'logic.for') {
    const c = Number(node?.data?.count || 0);
    return `loop: ${c || 0}`;
  }
  return '';
}

export function isTriggerNode(node) {
  return String(node?.kind || '').toLowerCase().startsWith('trigger.');
}

export function nodeSize() {
  return { width: NODE_WIDTH, headerHeight: NODE_HEADER_HEIGHT };
}

export function groupPaletteItems() {
  return [
    {
      title: 'Triggers (drag onto canvas)',
      items: [
        { kind: 'trigger.manual', label: 'Manual', dragOnly: true },
        { kind: 'trigger.device_state', label: 'Device state', dragOnly: true },
        { kind: 'trigger.schedule', label: 'Schedule (cron)', dragOnly: true },
      ],
    },
    {
      title: 'Logic',
      items: [
        { kind: 'logic.if', label: 'If' },
        { kind: 'logic.sleep', label: 'Sleep' },
        { kind: 'logic.for', label: 'For loop' },
      ],
    },
    {
      title: 'Actions',
      items: [
        { kind: 'action.send_command', label: 'Device command' },
        { kind: 'action.notify_email', label: 'Notify email' },
      ],
    },
  ];
}

export function canConnectInto(node) {
  return !isTriggerNode(node);
}

export function normalizeEdges(edges) {
  return (Array.isArray(edges) ? edges : [])
    .map((e) => ({ from: String(e?.from || ''), to: String(e?.to || '') }))
    .filter((e) => e.from && e.to);
}

export function removeEdge(edges, edgeToRemove) {
  const from = String(edgeToRemove?.from || '');
  const to = String(edgeToRemove?.to || '');
  return normalizeEdges(edges).filter((e) => !(e.from === from && e.to === to));
}

export function addEdge(edges, edgeToAdd) {
  const from = String(edgeToAdd?.from || '');
  const to = String(edgeToAdd?.to || '');
  if (!from || !to || from === to) return normalizeEdges(edges);
  const norm = normalizeEdges(edges);
  const exists = norm.some((e) => e.from === from && e.to === to);
  if (exists) return norm;
  return [...norm, { from, to }];
}
