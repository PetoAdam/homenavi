import React, { useCallback, useEffect, useState } from 'react';
import { useAuth } from '../../context/AuthContext';
import { listUsers, patchUser as updateUser, lockoutUser } from '../../services/authService';
import MasonryDashboard from '../Home/MasonryDashboard/MasonryDashboard';
import GlassCard from '../common/GlassCard/GlassCard';
import Button from '../common/Button/Button';
import Snackbar from '../common/Snackbar/Snackbar';
import RoleSelect from '../common/RoleSelect/RoleSelect';
import UserAvatar from '../common/UserAvatar/UserAvatar';
import PageHeader from '../common/PageHeader/PageHeader';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import LoadingView from '../common/LoadingView/LoadingView';
import './Users.css';

const PageSizeOptions = [10, 20, 50, 100];

function Users() {
  const { accessToken, user, bootstrapping } = useAuth();
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(false);
  const [savingRoleId, setSavingRoleId] = useState(null);
  const [savingLockId, setSavingLockId] = useState(null);
  const [activeUserId, setActiveUserId] = useState(null);
  const [toast, setToast] = useState('');
  const [err, setErr] = useState('');
  const [data, setData] = useState({ users: [], page: 1, page_size: 20, total: 0, total_pages: 0 });
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const fetchUsers = useCallback(async () => {
    if (!accessToken) return;
    setLoading(true);
    setErr('');
    const res = await listUsers({ q: query, page, pageSize }, accessToken);
    setLoading(false);
    if (res.success) setData(res.data);
    else setErr(res.error || 'Failed to fetch users');
  }, [accessToken, page, pageSize, query]);
  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const onSearch = (e) => {
    e.preventDefault();
    setPage(1);
    fetchUsers();
  };

  const handlePageSizeChange = (val) => {
    const num = parseInt(val, 10);
    if (!isNaN(num) && num !== pageSize) {
      setPage(1);
      setPageSize(num);
    }
  };

  const canChangeRole = (targetRole) => {
    if (!user) return false;
    if (user.role === 'admin') return true;
    if (user.role === 'resident') return targetRole === 'resident';
    return false;
  };

  const roles = ['user', 'resident', 'admin'];

  const handleRoleChange = async (u, role) => {
    if (!canChangeRole(role) || savingRoleId) return;
    setSavingRoleId(u.id);
    const res = await updateUser(u.id, { role }, accessToken);
    setSavingRoleId(null);
    if (res.success) { setToast('Role updated'); fetchUsers(); }
    else setErr(res.error || 'Failed to update role');
  };

  const handleToggleLockout = async (u) => {
    if (savingLockId) return;
    setSavingLockId(u.id);
    const res = await lockoutUser(u.id, !u.lockout_enabled, accessToken);
    setSavingLockId(null);
    if (res.success) { setToast(u.lockout_enabled ? 'User unlocked' : 'User locked'); fetchUsers(); }
    else setErr(res.error || 'Failed to update lockout');
  };

  const toggleActiveUser = (userId) => {
    setActiveUserId((prev) => (prev === userId ? null : userId));
  };

  if (!isResidentOrAdmin) {
    if (bootstrapping) {
      return <LoadingView title="Users" message="Loading users…" />;
    }
    return (
      <UnauthorizedView
        title="Users"
        message="You do not have permission to view this page."
      />
    );
  }

  return (
    <div className="users-page">
      <PageHeader
        title="Users"
        subtitle={`Manage roles, lockouts, and search the directory · ${data.total} total`}
      />
      <MasonryDashboard>
  <GlassCard interactive={false} className="fade-in span-all" key="users-list">
          <div className="card-body">
            <form className="users-toolbar" onSubmit={onSearch}>
              <input
                className="input"
                placeholder="Search by name or email..."
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              <RoleSelect
                value={`${pageSize}/page`}
                options={PageSizeOptions.map(n => `${n}/page`)}
                disabled={loading}
                onChange={(val) => handlePageSizeChange(val.split('/')[0])}
              />
              <Button type="submit" disabled={loading}>{loading ? 'Searching…' : 'Search'}</Button>
            </form>
            {err && <div className="alert error" role="alert">{err}</div>}
            <div className="users-mobile-view" aria-label="Users list">
              {loading && data.users.length === 0 && Array.from({ length: 6 }).map((_, i) => (
                <div key={`msk-${i}`} className="users-mobile-item skeleton-row">
                  <div className="skeleton-line" style={{ width: '100%' }} />
                </div>
              ))}
              {!loading && data.users.map((u) => {
                const canAdminChange = user?.role === 'admin';
                const isActive = activeUserId === u.id;
                return (
                  <div key={u.id} className={`users-mobile-card${isActive ? ' active' : ''}`}>
                    <button
                      type="button"
                      className="users-mobile-item"
                      onClick={() => toggleActiveUser(u.id)}
                      aria-expanded={isActive}
                    >
                      <div className="user-cell">
                        <UserAvatar user={{ first_name: u.first_name, last_name: u.last_name, user_name: u.user_name, avatar: u.profile_picture_url }} size={32} />
                        <div className="users-mobile-meta">
                          <div className="name">{u.first_name} {u.last_name}</div>
                          <div className="muted">@{u.user_name}</div>
                        </div>
                      </div>
                      <div className="users-mobile-badges">
                        <span className={`badge ${u.email_confirmed ? 'success' : 'muted'}`}>{u.email_confirmed ? 'Verified' : 'Unverified'}</span>
                        <span className={`badge ${u.lockout_enabled ? 'error' : 'success'}`}>{u.lockout_enabled ? 'Locked' : 'Active'}</span>
                      </div>
                    </button>

                    {isActive && (
                      <div className="users-mobile-actions" aria-label="Manage user">
                        <div className="users-mobile-email muted">{u.email}</div>
                        <div className="users-mobile-actions-row">
                          <RoleSelect
                            value={u.role}
                            options={roles.filter(r => canAdminChange || canChangeRole(r))}
                            disabled={savingRoleId === u.id}
                            saving={savingRoleId === u.id}
                            onChange={(role) => handleRoleChange(u, role)}
                          />
                          <Button
                            variant="secondary"
                            type="button"
                            disabled={savingLockId === u.id}
                            onClick={() => handleToggleLockout(u)}
                          >
                            {savingLockId === u.id ? 'Saving…' : (u.lockout_enabled ? 'Unlock' : 'Lock')}
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>

            <div className="users-table-view table-wrapper">
              <table className="table">
                <thead>
                  <tr>
                    <th>User</th>
                    <th>Email</th>
                    <th>Role</th>
                    <th>Verified</th>
                    <th>Active</th>
                    <th style={{width: 220}}>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {loading && data.users.length === 0 && Array.from({ length: 6 }).map((_, i) => (
                    <tr key={`sk-${i}`} className="row-fade skeleton-row">
                      <td colSpan={6}><div className="skeleton-line" style={{width: '100%'}} /></td>
                    </tr>
                  ))}
                  {!loading && data.users.map((u) => {
                    const canAdminChange = user?.role === 'admin';
                    return (
                      <tr key={u.id} className="row-fade">
                        <td>
                          <div className="user-cell">
                            <UserAvatar user={{ first_name: u.first_name, last_name: u.last_name, user_name: u.user_name, avatar: u.profile_picture_url }} size={32} />
                            <div>
                              <div className="name">{u.first_name} {u.last_name}</div>
                              <div className="muted">@{u.user_name}</div>
                            </div>
                          </div>
                        </td>
                        <td>{u.email}</td>
                        <td>
                          <RoleSelect
                            value={u.role}
                            options={roles.filter(r => canAdminChange || canChangeRole(r))}
                            disabled={savingRoleId === u.id}
                            saving={savingRoleId === u.id}
                            onChange={(role) => handleRoleChange(u, role)}
                          />
                        </td>
                        <td>
                          <span className={`badge ${u.email_confirmed ? 'success' : 'muted'}`}>{u.email_confirmed ? 'Verified' : 'Unverified'}</span>
                        </td>
                        <td>
                          <span className={`badge ${u.lockout_enabled ? 'error' : 'success'}`}>{u.lockout_enabled ? 'Locked' : 'Active'}</span>
                        </td>
                        <td>
                          <div className="actions">
                            <Button variant="secondary" type="button" disabled={savingLockId === u.id} onClick={() => handleToggleLockout(u)}>
                              {savingLockId === u.id ? 'Saving…' : (u.lockout_enabled ? 'Unlock' : 'Lock')}
                            </Button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            <div className="pagination">
              <Button variant="secondary" disabled={page <= 1 || loading} onClick={() => setPage(page - 1)}>Prev</Button>
              <span className="muted">Page {data.page} of {data.total_pages}</span>
              <Button variant="secondary" disabled={page >= data.total_pages || loading} onClick={() => setPage(page + 1)}>Next</Button>
            </div>
          </div>
        </GlassCard>
      </MasonryDashboard>
      <Snackbar message={toast} onClose={() => setToast('')} />
    </div>
  );
}
export default Users;
