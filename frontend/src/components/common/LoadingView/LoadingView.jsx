import React from 'react';
import PageHeader from '../PageHeader/PageHeader';
import GlassCard from '../GlassCard/GlassCard';
import './LoadingView.css';

export default function LoadingView({
  title = 'Loading',
  message = 'Loading…',
  className = '',
  showHeader = true,
  variant = 'default',
}) {
  const isMapSkeleton = variant === 'map-skeleton';
  const isAutomationSkeleton = variant === 'automation-skeleton';
  const showSpinner = !isMapSkeleton && !isAutomationSkeleton;

  return (
    <div className={['loading-page', `loading-page--${variant}`, className].filter(Boolean).join(' ')}>
      {showHeader ? <PageHeader title={title} subtitle="Just a moment." /> : null}
      <GlassCard interactive={false} className="loading-card">
        <div className="loading-body">
          {isMapSkeleton ? (
            <div className="loading-map-skeleton" aria-hidden="true">
              <div className="loading-map-skeleton__room loading-map-skeleton__room--primary" />
              <div className="loading-map-skeleton__room loading-map-skeleton__room--secondary" />
              <div className="loading-map-skeleton__room loading-map-skeleton__room--accent" />
              <div className="loading-map-skeleton__device loading-map-skeleton__device--one" />
              <div className="loading-map-skeleton__device loading-map-skeleton__device--two" />
              <div className="loading-map-skeleton__device loading-map-skeleton__device--three" />
            </div>
          ) : null}
          {isAutomationSkeleton ? (
            <div className="loading-automation-skeleton" aria-hidden="true">
              <div className="loading-automation-skeleton__toolbar" />
              <div className="loading-automation-skeleton__node loading-automation-skeleton__node--one" />
              <div className="loading-automation-skeleton__node loading-automation-skeleton__node--two" />
              <div className="loading-automation-skeleton__node loading-automation-skeleton__node--three" />
              <div className="loading-automation-skeleton__edge loading-automation-skeleton__edge--one" />
              <div className="loading-automation-skeleton__edge loading-automation-skeleton__edge--two" />
            </div>
          ) : null}
          {showSpinner ? <span className="loading-spinner" aria-label="Loading" /> : null}
          <div className="loading-message">{message}</div>
        </div>
      </GlassCard>
    </div>
  );
}
