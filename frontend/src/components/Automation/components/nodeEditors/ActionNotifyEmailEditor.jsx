import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';

export default class ActionNotifyEmailEditor extends BaseNodeEditor {
  state = { query: '' };

  splitUserLabel(label) {
    const raw = String(label || '').trim();
    if (!raw) return { primary: '', secondary: '' };

    if (raw.includes('\n')) {
      const parts = raw.split(/\n+/).map((p) => p.trim()).filter(Boolean);
      return { primary: parts[0] || raw, secondary: parts.slice(1).join(' · ') };
    }

    if (raw.includes(' — ')) {
      const [primary, ...rest] = raw.split(' — ');
      return { primary: (primary || '').trim() || raw, secondary: rest.join(' — ').trim() };
    }

    if (raw.includes(' - ')) {
      const [primary, ...rest] = raw.split(' - ');
      return { primary: (primary || '').trim() || raw, secondary: rest.join(' - ').trim() };
    }

    // Common pattern: "Name · email"
    const dotIdx = raw.indexOf('·');
    if (dotIdx >= 0) {
      const primary = raw.slice(0, dotIdx).trim();
      const secondary = raw.slice(dotIdx + 1).trim();
      return { primary: primary || raw, secondary };
    }

    // "Name <email>"
    const m = raw.match(/^(.*)\s*<([^>]+)>\s*$/);
    if (m) return { primary: (m[1] || '').trim() || raw, secondary: (m[2] || '').trim() };

    return { primary: raw, secondary: '' };
  }

  componentDidMount() {
    this.ensureResidentDefault();
  }

  componentDidUpdate(prevProps) {
    const prevNodeId = String(prevProps.selectedNode?.id || '');
    const nextNodeId = String(this.props.selectedNode?.id || '');
    if (prevNodeId !== nextNodeId) {
      this.ensureResidentDefault();
      return;
    }
    if (prevProps.isAdmin !== this.props.isAdmin || prevProps.currentUserId !== this.props.currentUserId) {
      this.ensureResidentDefault();
    }
  }

  ensureResidentDefault() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;

    const { isAdmin, currentUserId } = this.props;
    if (isAdmin) return;
    const currentId = String(currentUserId || '').trim();
    if (!currentId) return;

    const ids = Array.isArray(selectedNode.data?.user_ids)
      ? selectedNode.data.user_ids.map((x) => String(x || '')).filter(Boolean)
      : [];

    if (ids.length === 0) {
      this.setSelectedNodeData({ user_ids: [currentId] });
    }
  }

  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    const { userOptions, isAdmin, currentUserId } = this.props;
    const currentId = String(currentUserId || '');
    const query = String(this.state.query || '');

    const ids = Array.isArray(selectedNode.data?.user_ids)
      ? selectedNode.data.user_ids.map((x) => String(x || '')).filter(Boolean)
      : [];

    const roles = Array.isArray(selectedNode.data?.target_roles)
      ? selectedNode.data.target_roles.map((x) => String(x || '').trim().toLowerCase()).filter(Boolean)
      : [];


    const canEditUserId = (id) => {
      if (isAdmin) return true;
      return currentId && String(id) === currentId;
    };

    const toggleUserId = (id) => {
      const uid = String(id || '');
      if (!uid) return;
      if (!canEditUserId(uid)) return;

      const set = new Set(ids);
      if (set.has(uid)) set.delete(uid);
      else set.add(uid);
      this.setSelectedNodeData({ user_ids: Array.from(set) });
    };

    const toggleRole = (role) => {
      if (!isAdmin) return;
      const r = String(role || '').trim().toLowerCase();
      if (!r) return;
      const set = new Set(roles);
      if (set.has(r)) set.delete(r);
      else set.add(r);
      this.setSelectedNodeData({ target_roles: Array.from(set) });
    };

    const normalizedUsers = (Array.isArray(userOptions) ? userOptions : []).map((u) => {
      const id = String(u?.id || '');
      const label = String(u?.label || id);
      const { primary, secondary } = this.splitUserLabel(label);
      const hay = `${primary} ${secondary} ${label}`.toLowerCase();
      return { id, label, primary: primary || label, secondary, hay };
    }).filter((u) => u.id);

    const q = query.trim().toLowerCase();
    const filtered = q
      ? normalizedUsers.filter((u) => u.hay.includes(q) || u.id.toLowerCase().includes(q))
      : normalizedUsers;

    // Selected users pinned at the top of the list.
    const selectedSet = new Set(ids);
    const sortedUsers = [...filtered].sort((a, b) => {
      const as = selectedSet.has(a.id) ? 0 : 1;
      const bs = selectedSet.has(b.id) ? 0 : 1;
      if (as !== bs) return as - bs;
      return a.primary.localeCompare(b.primary);
    });

    const roleCount = roles.length;
    const userCount = ids.length;

    return (
      <div className="automation-props">
        <div className="automation-props-section">
          <div className="automation-props-section-header">
            <div className="automation-props-section-title">Recipients</div>
          </div>
          <div className="automation-props-section-subtitle">
            {isAdmin
              ? 'Choose specific people and/or target a whole group. The final expanded list is stored when you save the workflow.'
              : 'You can see all recipients, but only add/remove yourself.'}
          </div>

          {isAdmin ? (
            <div className="automation-toggle-pill-group" role="group" aria-label="Recipient groups">
              <button
                type="button"
                className={`automation-toggle-pill${roles.includes('resident') ? ' active' : ''}`}
                aria-pressed={roles.includes('resident')}
                onClick={() => toggleRole('resident')}
              >
                All residents
              </button>
              <button
                type="button"
                className={`automation-toggle-pill${roles.includes('admin') ? ' active' : ''}`}
                aria-pressed={roles.includes('admin')}
                onClick={() => toggleRole('admin')}
              >
                All admins
              </button>
            </div>
          ) : null}

          <div className="automation-props-kv" aria-label="Recipient count">
            <div className="muted">Targets</div>
            <div className="muted">{userCount} user{userCount === 1 ? '' : 's'}{roleCount ? ` · ${roleCount} group${roleCount === 1 ? '' : 's'}` : ''}</div>
          </div>

          <div className="field" style={{ marginTop: 6 }}>
            <label className="label">Users</label>
            <input
              className="input"
              value={query}
              onChange={(e) => this.setState({ query: e.target.value })}
              placeholder="Search users…"
              aria-label="Search users"
            />
            <div className="muted">Search updates as you type. Selected users appear first.</div>
            <div className="automation-scrollbox" role="list" aria-label="User list">
              {sortedUsers.length === 0 ? (
                <div className="muted" style={{ padding: 10 }}>No matching users.</div>
              ) : (
                sortedUsers.map((u) => {
                  const uid = String(u.id);
                  const checked = ids.includes(uid);
                  const you = currentId && uid === currentId;
                  const disabled = !canEditUserId(uid);
                  return (
                    <label
                      key={uid}
                      className={`automation-user-row${disabled ? ' disabled' : ''}`}
                      role="listitem"
                      title={you ? `${u.label} (you)` : u.label}
                    >
                      <div className="automation-user-row-main">
                        <div className="automation-user-row-title">{you ? `${u.primary} (you)` : u.primary}</div>
                        {u.secondary ? <div className="automation-user-row-subtitle">{u.secondary}</div> : null}
                      </div>
                      <div className="automation-user-row-actions">
                        <input
                          type="checkbox"
                          checked={checked}
                          disabled={disabled}
                          onChange={() => toggleUserId(uid)}
                        />
                      </div>
                    </label>
                  );
                })
              )}
            </div>
          </div>
        </div>

        <div className="automation-props-section">
          <div className="automation-props-section-header">
            <div className="automation-props-section-title">Email</div>
          </div>

          <div className="field">
            <label className="label">Subject</label>
            <input
              className="input"
              value={selectedNode.data?.subject || ''}
              onChange={(e) => this.setSelectedNodeData({ subject: e.target.value })}
              placeholder="e.g. Door opened"
            />
          </div>

          <div className="field">
            <label className="label">Message</label>
            <textarea
              className="input textarea"
              rows={6}
              value={selectedNode.data?.message || ''}
              onChange={(e) => this.setSelectedNodeData({ message: e.target.value })}
              placeholder="What should the email say?"
            />
          </div>
        </div>
      </div>
    );
  }
}
