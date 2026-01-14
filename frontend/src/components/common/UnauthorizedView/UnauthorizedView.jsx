import React from 'react';
import { faRightToBracket, faLock } from '@fortawesome/free-solid-svg-icons';
import PageHeader from '../PageHeader/PageHeader';
import GlassCard from '../GlassCard/GlassCard';
import GlassPill from '../GlassPill/GlassPill';
import './UnauthorizedView.css';

export default function UnauthorizedView({
  title = 'Unauthorized',
  message = 'You do not have permission to view this page.',
  className = '',
  hideHeader = false,
}) {
  return (
    <div className={['unauthorized-page', className].filter(Boolean).join(' ')}>
      {!hideHeader && <PageHeader title={title} subtitle="Sign in with a resident account to continue." />}
      <GlassCard interactive={false} className="unauthorized-card">
        <div className="unauthorized-body">
          <div className="unauthorized-icon" aria-hidden="true">
            <span className="unauthorized-icon-badge">🔒</span>
            <span className="unauthorized-pulse" />
          </div>
          <div className="unauthorized-message">{message}</div>
          <div className="unauthorized-actions">
            <GlassPill
              icon={faRightToBracket}
              text="Log in"
              tone="success"
              onClick={() => window.dispatchEvent(new CustomEvent('homenavi:open-auth'))}
              className="unauthorized-login-pill"
            />
            <GlassPill
              icon={faLock}
              text="Why?"
              tone="default"
              onClick={() => window.dispatchEvent(new CustomEvent('homenavi:open-auth'))}
              title="Access is limited to residents/admins."
              className="unauthorized-why-pill"
            />
          </div>
          <div className="unauthorized-note">
            Tip: click the profile icon in the top-right to sign in.
          </div>
        </div>
      </GlassCard>
    </div>
  );
}
