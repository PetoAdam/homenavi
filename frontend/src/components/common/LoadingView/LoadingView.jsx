import React from 'react';
import PageHeader from '../PageHeader/PageHeader';
import GlassCard from '../GlassCard/GlassCard';
import './LoadingView.css';

export default function LoadingView({
  title = 'Loading',
  message = 'Loading…',
  className = '',
}) {
  return (
    <div className={['loading-page', className].filter(Boolean).join(' ')}>
      <PageHeader title={title} subtitle="Just a moment." />
      <GlassCard interactive={false} className="loading-card">
        <div className="loading-body">
          <span className="loading-spinner" aria-label="Loading" />
          <div className="loading-message">{message}</div>
        </div>
      </GlassCard>
    </div>
  );
}
