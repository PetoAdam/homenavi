import { describe, expect, it } from 'vitest';
import {
  integrationsAdminOperationsInitialState,
  integrationsAdminOperationsReducer,
} from './integrationsAdminOperationsReducer';

describe('integrationsAdminOperationsReducer', () => {
  it('queues install with expected status defaults', () => {
    const next = integrationsAdminOperationsReducer(integrationsAdminOperationsInitialState, {
      type: 'queue-install',
      id: 'spotify',
    });

    expect(next.installing.spotify).toBe(true);
    expect(next.installStatus.spotify).toMatchObject({
      id: 'spotify',
      stage: 'queued',
      progress: 5,
      message: 'Queued',
    });
  });

  it('queues restart all and marks all ids restarting', () => {
    const next = integrationsAdminOperationsReducer(integrationsAdminOperationsInitialState, {
      type: 'queue-restart-all',
      ids: ['a', 'b'],
    });

    expect(next.restartingAll).toBe(true);
    expect(next.restartAllTargets).toEqual(['a', 'b']);
    expect(next.restarting.a).toBe(true);
    expect(next.restarting.b).toBe(true);
    expect(next.installStatus.a?.stage).toBe('queued');
    expect(next.installStatus.b?.stage).toBe('queued');
  });

  it('sets and merges operation status incrementally', () => {
    const queued = integrationsAdminOperationsReducer(integrationsAdminOperationsInitialState, {
      type: 'queue-update',
      id: 'x',
      progress: 15,
      message: 'Queued update',
    });
    const merged = integrationsAdminOperationsReducer(queued, {
      type: 'set-install-status',
      id: 'x',
      status: { stage: 'in_progress', progress: 60 },
    });

    expect(merged.updating.x).toBe(true);
    expect(merged.installStatus.x).toMatchObject({
      id: 'x',
      stage: 'in_progress',
      progress: 60,
      message: 'Queued update',
    });
  });

  it('supports map-entry toggles for flags', () => {
    const withInstall = integrationsAdminOperationsReducer(integrationsAdminOperationsInitialState, {
      type: 'set-installing',
      id: 'spotify',
      value: true,
    });
    const cleared = integrationsAdminOperationsReducer(withInstall, {
      type: 'set-installing',
      id: 'spotify',
      value: false,
    });

    expect(withInstall.installing.spotify).toBe(true);
    expect(cleared.installing.spotify).toBe(false);
  });
});
