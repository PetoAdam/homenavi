import React from 'react';
import GlassCard from '../../common/GlassCard/GlassCard';
import Button from '../../common/Button/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faArrowsRotate } from '@fortawesome/free-solid-svg-icons';

function formatDurationMs(ms) {
  if (!Number.isFinite(ms) || ms < 0) return '—';
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rem = s % 60;
  return `${m}m ${rem}s`;
}

function durationText(run) {
  const started = run?.started_at ? new Date(run.started_at) : null;
  if (!started || Number.isNaN(started.getTime())) return '—';
  const finished = run?.finished_at ? new Date(run.finished_at) : null;
  const end = finished && !Number.isNaN(finished.getTime()) ? finished : (run?.status === 'running' ? new Date() : null);
  if (!end || Number.isNaN(end.getTime())) return '—';
  return formatDurationMs(end.getTime() - started.getTime());
}

export default function AutomationRuns({
  selectedWorkflow,
  runs,
  runsLoading,
  runsLimit,
  setRunsLimit,
  fetchRuns,
}) {
  return (
    <GlassCard interactive={false} className="fade-in" key="automation-runs">
      <div className="card-body">
        <div className="automation-list-header">
          <div className="title">Recent runs</div>
          <div className="muted">{selectedWorkflow ? selectedWorkflow.name : 'Select a workflow'}</div>
        </div>

        {!selectedWorkflow && <div className="muted">Pick a workflow to see its latest runs.</div>}

        {selectedWorkflow && (
          <>
            <div className="actions compact" style={{ marginBottom: 12 }}>
              <Button
                variant="secondary"
                type="button"
                disabled={runsLoading}
                onClick={() => fetchRuns(selectedWorkflow.id, runsLimit)}
                title="Refresh runs"
              >
                <span className="btn-icon"><FontAwesomeIcon icon={faArrowsRotate} /></span>
                {runsLoading ? 'Loading…' : 'Refresh'}
              </Button>
            </div>

            <div className="table-wrapper">
              <table className="table automation-runs-table">
                <thead>
                  <tr>
                    <th style={{ width: 240 }}>Started</th>
                    <th style={{ width: 240 }}>Finished</th>
                    <th style={{ width: 110 }}>Duration</th>
                    <th style={{ width: 120 }}>Status</th>
                    <th>Error</th>
                  </tr>
                </thead>
                <tbody>
                  {!runsLoading && runs.length === 0 && (
                    <tr>
                      <td colSpan={5} className="muted">No runs yet.</td>
                    </tr>
                  )}
                  {runs.map((run) => (
                    <tr key={run.id}>
                      <td className="muted">
                        <div>{run.started_at ? new Date(run.started_at).toLocaleString() : '—'}</div>
                        {run.id ? <div className="automation-runs-runid">{String(run.id).slice(0, 8)}</div> : null}
                      </td>
                      <td className="muted">{run.finished_at ? new Date(run.finished_at).toLocaleString() : '—'}</td>
                      <td className="muted">{durationText(run)}</td>
                      <td>
                        <span className={`badge ${run.status === 'success' ? 'success' : (run.status === 'failed' ? 'error' : 'muted')}`}>{run.status}</span>
                      </td>
                      <td className="muted">{run.error || ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="automation-runs-footer">
              <button
                type="button"
                className="link-btn automation-runs-view-more"
                disabled={runsLoading}
                onClick={() => {
                  setRunsLimit((n) => {
                    const next = Math.min(200, Number(n || 0) + 5);
                    fetchRuns(selectedWorkflow.id, next);
                    return next;
                  });
                }}
              >
                View more
              </button>
            </div>
          </>
        )}
      </div>
    </GlassCard>
  );
}
