import React, { useEffect, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faArrowRight, faCheck, faLock, faPlug, faServer, faShieldHalved, faUserCircle } from '@fortawesome/free-solid-svg-icons';
import '../AuthModal/AuthModal.css';
import './OAuthAuthorize.css';

const transactionURL = '/api/auth/oauth/transaction';

const scopeLabels = {
  'home.devices.read': 'View devices and their current status',
  'home.devices.write': 'Control devices',
  'home.inventory.read': 'View rooms and device groups',
  'home.inventory.write': 'Create device groups',
  'home.history.read': 'View device history',
  'home.automation.read': 'View automations',
  'home.automation.execute': 'Run automations',
};

async function transactionRequest(path = '', options = {}) {
  const response = await fetch(`${transactionURL}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error_description || 'This authorization session is no longer available.');
  }
  return payload;
}

export default function OAuthAuthorize() {
  const [transaction, setTransaction] = useState(null);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);

  const loadTransaction = async () => {
    try {
      setTransaction(await transactionRequest());
      setError('');
    } catch (requestError) {
      setError(requestError.message);
    }
  };

  useEffect(() => {
    loadTransaction();
  }, []);

  const submitLogin = async (event) => {
    event.preventDefault();
    setPending(true);
    try {
      await transactionRequest('/login', { method: 'POST', body: JSON.stringify({ email, password }) });
      setPassword('');
      await loadTransaction();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setPending(false);
    }
  };

  const submitVerification = async (event) => {
    event.preventDefault();
    setPending(true);
    try {
      await transactionRequest('/verify', { method: 'POST', body: JSON.stringify({ code }) });
      setCode('');
      await loadTransaction();
    } catch (requestError) {
      setError(requestError.message);
    } finally {
      setPending(false);
    }
  };

  const finish = async (action) => {
    setPending(true);
    try {
      const response = await transactionRequest(`/${action}`, { method: 'POST', body: '{}' });
      window.location.assign(response.redirect_uri);
    } catch (requestError) {
      setError(requestError.message);
      setPending(false);
    }
  };

  const clientName = transaction?.client_id || 'this application';
  const scopes = transaction?.scopes || [];
  const canControlHome = scopes.some((scope) => scope.endsWith('.write') || scope.endsWith('.execute'));
  return (
    <div className="oauth-authorize-page">
      <main className="oauth-authorize-panel" aria-live="polite">
        <header className="oauth-authorize-brand">
          <span className="oauth-authorize-brand-mark"><FontAwesomeIcon icon={faShieldHalved} /></span>
          <span>Homenavi</span>
          <span className="oauth-authorize-brand-label">MCP access</span>
        </header>
        {!transaction && error ? (
          <section className="oauth-authorize-expired" role="alert">
            <FontAwesomeIcon icon={faShieldHalved} />
            <h1>Authorization request expired</h1>
            <p>Return to your MCP client and start the connection again.</p>
          </section>
        ) : (
          <>
            {error && <div className="oauth-authorize-error" role="alert">{error}</div>}
            {!transaction ? <p className="oauth-authorize-muted">Loading authorization request...</p> : transaction.two_fa_required ? (
          <form className="auth-modal-form oauth-authorize-form" onSubmit={submitVerification}>
            <div className="auth-modal-header">
              <FontAwesomeIcon icon={faLock} className="auth-modal-avatar" />
              <span className="auth-modal-title">Verify your identity</span>
              <span className="auth-modal-subtitle">Enter the code from your authenticator or email.</span>
            </div>
            <div className="auth-modal-field">
              <input className="auth-modal-input" id="oauth-code" value={code} onChange={(event) => setCode(event.target.value)} autoComplete="one-time-code" inputMode="numeric" placeholder=" " required autoFocus />
              <label className="auth-modal-label" htmlFor="oauth-code">Verification code</label>
            </div>
            <button className="auth-modal-btn" disabled={pending}>{pending ? 'Verifying...' : 'Verify'}</button>
          </form>
        ) : !transaction.authenticated ? (
          <form className="auth-modal-form oauth-authorize-form" onSubmit={submitLogin}>
            <div className="auth-modal-header">
              <FontAwesomeIcon icon={faUserCircle} className="auth-modal-avatar" />
              <span className="auth-modal-title">Authenticate Homenavi MCP access</span>
              <span className="auth-modal-subtitle">Sign in to choose what this MCP client can access in your home.</span>
            </div>
            <div className="auth-modal-field">
              <input className="auth-modal-input" id="oauth-email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="username" placeholder=" " required autoFocus />
              <label className="auth-modal-label" htmlFor="oauth-email">Email</label>
            </div>
            <div className="auth-modal-field">
              <input className="auth-modal-input" id="oauth-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" placeholder=" " required />
              <label className="auth-modal-label" htmlFor="oauth-password">Password</label>
            </div>
            <button className="auth-modal-btn" disabled={pending}>{pending ? 'Signing in...' : 'Sign in'}</button>
          </form>
          ) : (
          <section className="oauth-authorize-consent">
            <div className="oauth-authorize-connection">
              <span className="oauth-authorize-connection-icon"><FontAwesomeIcon icon={faServer} /></span>
              <span className="oauth-authorize-connection-link"><span className="oauth-authorize-connection-line" /><FontAwesomeIcon className="oauth-authorize-connection-arrow" icon={faArrowRight} /></span>
              <span className="oauth-authorize-connection-icon client"><FontAwesomeIcon icon={faPlug} /></span>
            </div>
            <div className="oauth-authorize-consent-heading">
              <span className="oauth-authorize-eyebrow">Homenavi MCP authorization</span>
              <h1>Connect {clientName}</h1>
              <p>{canControlHome ? 'This client is requesting permission to read information and control the selected parts of your Homenavi home.' : 'This client is requesting permission to read information from your Homenavi home.'}</p>
            </div>
            <div className="oauth-authorize-permissions">
              <div className="oauth-authorize-permissions-title"><FontAwesomeIcon icon={faCheck} /> Requested permissions</div>
              <ul>{scopes.map((scope) => <li key={scope}>{scopeLabels[scope] || scope}</li>)}</ul>
            </div>
            <div className="oauth-authorize-security"><FontAwesomeIcon icon={faShieldHalved} /> You are authenticating access for the Homenavi MCP. Homenavi will only grant the permissions listed above.</div>
            <button className="auth-modal-btn" onClick={() => finish('approve')} disabled={pending}>{pending ? 'Connecting...' : 'Authorize MCP access'}</button>
            <button className="auth-modal-btn secondary" onClick={() => finish('deny')} disabled={pending}>Deny</button>
          </section>
            )}
          </>
        )}
      </main>
    </div>
  );
}