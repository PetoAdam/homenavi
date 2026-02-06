import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBackwardStep,
  faForwardStep,
  faPause,
  faPlay,
  faPlus,
  faRepeat,
  faShuffle,
  faVolumeHigh,
  faHeadphones,
  faMagnifyingGlass,
  faListUl,
  faXmark,
} from '@fortawesome/free-solid-svg-icons';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import {
  getState,
  getQueue,
  getDevices,
  play,
  pause,
  nextTrack,
  previousTrack,
  setShuffle,
  setRepeat,
  setVolume,
  seek,
  addToQueue,
  transferPlayback,
  searchTracks,
} from './api';

function formatMs(ms = 0) {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}

const repeatCycle = ['off', 'context', 'track'];

export default function Player({ variant = 'tab', showSearch = false, showQueue = true }) {
  const [state, setState] = React.useState(null);
  const [queue, setQueue] = React.useState(null);
  const [devices, setDevices] = React.useState([]);
  const [error, setError] = React.useState('');
  const [loading, setLoading] = React.useState(true);
  const [scrub, setScrub] = React.useState(null);
  const [volume, setVolumeState] = React.useState(null);
  const [searchQuery, setSearchQuery] = React.useState('');
  const [searchResults, setSearchResults] = React.useState([]);
  const [searching, setSearching] = React.useState(false);
  const [canShowQueue, setCanShowQueue] = React.useState(showQueue);
  const [bgCover, setBgCover] = React.useState('');
  const [bgPrev, setBgPrev] = React.useState('');
  const [coverIsBright, setCoverIsBright] = React.useState(false);
  const [isTitleMarquee, setIsTitleMarquee] = React.useState(false);
  const containerRef = React.useRef(null);
  const fadeTimerRef = React.useRef(null);
  const titleRef = React.useRef(null);
  const debounceRef = React.useRef(null);
  const optimisticRef = React.useRef({});
  const setOptimistic = React.useCallback((fields) => {
    const nextFreeze = Date.now() + 3000;
    const prevFreeze = optimisticRef.current?.frozenUntil || 0;
    optimisticRef.current = {
      ...optimisticRef.current,
      ...fields,
      frozenUntil: Math.max(nextFreeze, prevFreeze),
    };
  }, []);

  const activeItem = state?.item || state?.currently_playing || null;
  const cover = activeItem?.album?.images?.[0]?.url || activeItem?.images?.[0]?.url || '';
  const artist = activeItem?.artists?.map((a) => a.name).join(', ') || 'No artist';
  const title = activeItem?.name || 'Nothing playing';
  const durationMs = activeItem?.duration_ms || 0;
  const progressMs = scrub !== null ? scrub : (state?.progress_ms || 0);
  const isPlaying = Boolean(state?.is_playing);
  const shuffleOn = Boolean(state?.shuffle_state);
  const repeatState = state?.repeat_state || 'off';
  const configError = React.useMemo(() => {
    if (!error) return false;
    const trimmed = String(error).trim();
    if (!trimmed) return false;
    if (trimmed.includes('spotify integration is not configured')) return true;
    if (trimmed.includes('missing SPOTIFY_CLIENT_ID')) return true;
    if (trimmed.startsWith('{')) {
      try {
        const parsed = JSON.parse(trimmed);
        const message = String(parsed?.error || parsed?.message || '').toLowerCase();
        return message.includes('spotify integration is not configured') || message.includes('missing spotify_client_id');
      } catch {
        return false;
      }
    }
    return false;
  }, [error]);

  const refresh = React.useCallback(async () => {
    try {
      const [nextState, nextQueue, nextDevices] = await Promise.all([
        getState(),
        showQueue ? getQueue() : Promise.resolve(null),
        getDevices(),
      ]);
      const optimistic = optimisticRef.current;
      const now = Date.now();
      const frozen = Boolean(optimistic?.frozenUntil && optimistic.frozenUntil > now);
      const optimisticState = { ...nextState };
      if (frozen) {
        if (optimistic?.is_playing != null) {
          optimisticState.is_playing = optimistic.is_playing;
        }
        if (optimistic?.repeat_state != null) {
          optimisticState.repeat_state = optimistic.repeat_state;
        }
        if (optimistic?.shuffle_state != null) {
          optimisticState.shuffle_state = optimistic.shuffle_state;
        }
        if (optimistic?.volume_percent != null) {
          optimisticState.device = {
            ...(optimisticState.device || {}),
            volume_percent: optimistic.volume_percent,
          };
        }
        if (optimistic?.progress_ms != null) {
          optimisticState.progress_ms = optimistic.progress_ms;
        }
      } else if (optimistic?.frozenUntil) {
        optimisticRef.current = {};
      }
      setState(optimisticState);
      if (showQueue) setQueue(nextQueue);
      setDevices(nextDevices?.devices || []);
      setError('');
      if (scrub === null) {
        const nextVolume = frozen && optimistic?.volume_percent != null
          ? optimistic.volume_percent
          : nextState?.device?.volume_percent;
        if (nextVolume != null) {
          setVolumeState(nextVolume);
        }
      }
    } catch (err) {
      setError(err?.message || 'Unable to load Spotify');
    } finally {
      setLoading(false);
    }
  }, [showQueue, scrub]);


  React.useEffect(() => {
    if (!cover) return;
    if (!bgCover) {
      setBgCover(cover);
      return;
    }
    if (cover === bgCover) return;
    setBgPrev(bgCover);
    setBgCover(cover);
    if (fadeTimerRef.current) {
      clearTimeout(fadeTimerRef.current);
    }
    fadeTimerRef.current = setTimeout(() => {
      setBgPrev('');
    }, 700);
    return () => {
      if (fadeTimerRef.current) {
        clearTimeout(fadeTimerRef.current);
      }
    };
  }, [cover, bgCover]);

  React.useEffect(() => {
    if (variant !== 'widget' || !containerRef.current) return undefined;
    const node = containerRef.current;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const height = entry.contentRect?.height || 0;
        const allow = height >= 300;
        setCanShowQueue(allow);
      }
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [variant]);

  React.useEffect(() => {
    if (!cover) {
      setCoverIsBright(false);
      return;
    }
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.onload = () => {
      try {
        const canvas = document.createElement('canvas');
        const ctx = canvas.getContext('2d', { willReadFrequently: true });
        if (!ctx) return;
        const size = 24;
        canvas.width = size;
        canvas.height = size;
        ctx.drawImage(img, 0, 0, size, size);
        const data = ctx.getImageData(0, 0, size, size).data;
        let total = 0;
        let count = 0;
        for (let i = 0; i < data.length; i += 4) {
          const r = data[i];
          const g = data[i + 1];
          const b = data[i + 2];
          const luma = 0.2126 * r + 0.7152 * g + 0.0722 * b;
          total += luma;
          count += 1;
        }
        const avg = total / count;
        setCoverIsBright(avg > 165);
      } catch {
        setCoverIsBright(false);
      }
    };
    img.onerror = () => setCoverIsBright(false);
    img.src = cover;
  }, [cover]);

  React.useEffect(() => {
    if (!titleRef.current) return;
    const el = titleRef.current;
    const shouldMarquee = el.scrollWidth > el.clientWidth + 8;
    setIsTitleMarquee(shouldMarquee);
  }, [title, variant, cover]);

  React.useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 8000);
    return () => clearInterval(timer);
  }, [refresh]);

  React.useEffect(() => {
    if (!isPlaying || scrub !== null) return undefined;
    const interval = setInterval(() => {
      setState((prev) => {
        if (!prev) return prev;
        return { ...prev, progress_ms: Math.min((prev.progress_ms || 0) + 1000, durationMs) };
      });
    }, 1000);
    return () => clearInterval(interval);
  }, [isPlaying, scrub, durationMs]);

  const handlePlayPause = async () => {
    const next = !isPlaying;
    setOptimistic({ is_playing: next });
    setState((prev) => (prev ? { ...prev, is_playing: next } : prev));
    try {
      if (isPlaying) {
        await pause();
      } else {
        await play();
      }
      refresh();
    } catch (err) {
      setState((prev) => (prev ? { ...prev, is_playing: !next } : prev));
      setError(err?.message || 'Failed to update playback');
    }
  };

  const handleRepeat = async () => {
    const idx = repeatCycle.indexOf(repeatState);
    const next = repeatCycle[(idx + 1) % repeatCycle.length];
    setOptimistic({ repeat_state: next });
    setState((prev) => (prev ? { ...prev, repeat_state: next } : prev));
    try {
      await setRepeat(next);
      refresh();
    } catch (err) {
      setError(err?.message || 'Failed to update repeat');
      refresh();
    }
  };

  const handleSearch = async (value) => {
    const query = value ?? searchQuery;
    if (!query.trim()) {
      setSearchResults([]);
      return;
    }
    setSearching(true);
    try {
      const res = await searchTracks(query.trim());
      setSearchResults(res?.tracks?.items || []);
    } catch (err) {
      setError(err?.message || 'Search failed');
    } finally {
      setSearching(false);
    }
  };

  React.useEffect(() => {
    if (!showSearch) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      handleSearch(searchQuery);
    }, 450);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [searchQuery, showSearch]);

  const deviceId = state?.device?.id;
  const showQueueBlock = showQueue && (variant !== 'widget' || canShowQueue);
  const handleOpenTab = () => {
    if (variant !== 'widget') return;
    try {
      if (window.top && window.top !== window.self) {
        window.top.location.assign('/apps/spotify');
      } else {
        window.location.assign('/apps/spotify');
      }
    } catch {
      window.location.assign('/apps/spotify');
    }
  };

  if (configError) {
    return (
      <div className={['spotify-shell', variant === 'widget' ? 'spotify-compact' : '', coverIsBright ? 'cover-bright' : 'cover-dark'].join(' ')}>
        <div className="spotify-card">
          <div className="spotify-card-content">
            <div className="spotify-header">
              <div className="spotify-widget-header">
                <FontAwesomeIcon icon={faSpotify} className="spotify-widget-icon" />
                <span className="spotify-widget-title">Spotify</span>
              </div>
              <span className="spotify-pill">Config</span>
            </div>
            <div className="spotify-subtitle">Spotify is not configured.</div>
            <div className="spotify-subtitle">Set SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, and SPOTIFY_REFRESH_TOKEN.</div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      className={['spotify-shell', variant === 'widget' ? 'spotify-compact' : '', coverIsBright ? 'cover-bright' : 'cover-dark'].join(' ')}
    >
      {showSearch && variant === 'tab' ? (
        <div className="spotify-search-overlay">
          <div className="spotify-card">
            {bgCover ? (
              <div className="spotify-card-bg current" style={{ backgroundImage: `url(${bgCover})` }} />
            ) : null}
            <div className="spotify-card-overlay" />
            <div className="spotify-card-content spotify-search">
              <div className="spotify-header">
                <div className="spotify-title">
                  <FontAwesomeIcon icon={faMagnifyingGlass} style={{ marginRight: 8 }} />
                  Search
                </div>
                <span className="spotify-pill">search</span>
              </div>
              <div className="spotify-search-row">
                <div className="spotify-search-input">
                  <input
                    className="spotify-input"
                    placeholder="Search for a song, artist, or album"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                  />
                  {searchQuery ? (
                    <button
                      className="spotify-btn spotify-icon-btn"
                      onClick={() => {
                        setSearchQuery('');
                        setSearchResults([]);
                        setSearching(false);
                      }}
                      title="Clear"
                      type="button"
                    >
                      <FontAwesomeIcon icon={faXmark} />
                    </button>
                  ) : null}
                </div>
                <button className="spotify-btn" onClick={() => handleSearch()} disabled={searching || !searchQuery.trim()}>
                  {searching ? 'Searching…' : 'Search'}
                </button>
              </div>

              {searchResults.length ? (
                <div className="spotify-search-results">
                  {searchResults.slice(0, 8).map((track) => (
                    <div className="spotify-search-item" key={track.id}>
                      {track.album?.images?.[2]?.url ? (
                        <img className="spotify-search-cover" src={track.album.images[2].url} alt="" aria-hidden="true" />
                      ) : (
                        <div className="spotify-search-cover spotify-search-cover-empty" aria-hidden="true" />
                      )}
                      <div className="spotify-search-meta">
                        <div className="spotify-search-title">{track.name}</div>
                        <div className="spotify-search-artist">{track.artists?.map((a) => a.name).join(', ')}</div>
                      </div>
                      <div className="spotify-search-actions">
                        <button
                          className="spotify-btn"
                          onClick={async () => {
                            setOptimistic({ is_playing: true });
                            setState((prev) => (prev ? { ...prev, is_playing: true } : prev));
                            await play({ uris: [track.uri] });
                            setSearchQuery('');
                            setSearchResults([]);
                            refresh();
                          }}
                        >
                          <FontAwesomeIcon icon={faPlay} />
                          <span className="spotify-btn-label">Play now</span>
                        </button>
                        <button
                          className="spotify-btn"
                          onClick={async () => {
                            await addToQueue(track.uri, deviceId);
                            setSearchQuery('');
                            setSearchResults([]);
                            refresh();
                          }}
                        >
                          <FontAwesomeIcon icon={faPlus} />
                          <span className="spotify-btn-label">Add to queue</span>
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="spotify-empty">{loading ? 'Loading…' : 'Search results appear here.'}</div>
              )}
            </div>
          </div>
        </div>
      ) : null}

      <div className={`spotify-card ${bgCover ? 'has-cover' : 'no-cover'}`}>
        {bgPrev ? (
          <div className="spotify-card-bg prev" style={{ backgroundImage: `url(${bgPrev})` }} />
        ) : null}
        {bgCover ? (
          <div className="spotify-card-bg current" style={{ backgroundImage: `url(${bgCover})` }} />
        ) : null}
        <div className="spotify-card-overlay" />
        <div className="spotify-card-content">
          <div className="spotify-header">
            {variant === 'widget' ? (
              <div className="spotify-widget-header" onClick={handleOpenTab} role="button" tabIndex={0}>
                <FontAwesomeIcon icon={faSpotify} className="spotify-widget-icon" />
                <span className="spotify-widget-title">Spotify</span>
                <span className={`spotify-eq ${isPlaying ? 'is-playing' : ''}`} aria-hidden="true">
                  <span />
                  <span />
                  <span />
                </span>
              </div>
            ) : (
              <div>
                <div className="spotify-widget-header">
                  <FontAwesomeIcon icon={faSpotify} className="spotify-widget-icon" />
                  <span className="spotify-widget-title">Spotify</span>
                  <span className={`spotify-eq ${isPlaying ? 'is-playing' : ''}`} aria-hidden="true">
                    <span />
                    <span />
                    <span />
                  </span>
                </div>
              </div>
            )}
            <span className="spotify-pill">{isPlaying ? 'Playing' : 'Paused'}</span>
          </div>

          {error ? <div className="spotify-subtitle">{error}</div> : null}

          <div className="spotify-main">
            <div
              className={`spotify-cover ${variant === 'widget' ? 'spotify-tap' : ''}`}
              onClick={handleOpenTab}
              role={variant === 'widget' ? 'button' : undefined}
              tabIndex={variant === 'widget' ? 0 : undefined}
            >
              {cover ? <img src={cover} alt="Album cover" /> : <span className="spotify-subtitle">No cover</span>}
            </div>
            <div className="spotify-track">
              <div
                className={`spotify-track-head ${variant === 'widget' ? 'spotify-tap' : ''}`}
                onClick={handleOpenTab}
                role={variant === 'widget' ? 'button' : undefined}
                tabIndex={variant === 'widget' ? 0 : undefined}
              >
                <div ref={titleRef} className={`spotify-track-title ${isTitleMarquee ? 'marquee' : ''}`}>
                  <span>{title}</span>
                </div>
                <div className="spotify-track-artist">{artist}</div>
              </div>
              <div className="spotify-range-row">
                <span>{formatMs(progressMs)}</span>
                <input
                  className="spotify-range"
                  type="range"
                  min={0}
                  max={durationMs || 0}
                  value={durationMs ? progressMs : 0}
                  onChange={(e) => setScrub(Number(e.target.value))}
                  onMouseUp={async (e) => {
                    const nextValue = Number(e.currentTarget.value);
                    setScrub(null);
                    setOptimistic({ progress_ms: nextValue });
                    setState((prev) => (prev ? { ...prev, progress_ms: nextValue } : prev));
                    await seek(nextValue);
                    refresh();
                  }}
                  onTouchEnd={async (e) => {
                    const nextValue = Number(e.currentTarget.value);
                    setScrub(null);
                    setOptimistic({ progress_ms: nextValue });
                    setState((prev) => (prev ? { ...prev, progress_ms: nextValue } : prev));
                    await seek(nextValue);
                    refresh();
                  }}
                />
                <span>{formatMs(durationMs)}</span>
              </div>
              <div className={`spotify-controls ${variant === 'tab' ? 'spotify-controls-compact' : ''}`} onClick={(e) => e.stopPropagation()}>
                <button className={`spotify-btn ${shuffleOn ? 'active' : ''}`} onClick={async () => {
                  const nextShuffle = !shuffleOn;
                  setOptimistic({ shuffle_state: nextShuffle });
                  setState((prev) => (prev ? { ...prev, shuffle_state: nextShuffle } : prev));
                  try {
                    await setShuffle(nextShuffle);
                    refresh();
                  } catch (err) {
                    setError(err?.message || 'Failed to update shuffle');
                    refresh();
                  }
                }} title="Shuffle">
                  <FontAwesomeIcon icon={faShuffle} />
                </button>
                <button className="spotify-btn" onClick={async () => {
                  await previousTrack();
                  refresh();
                }} title="Previous">
                  <FontAwesomeIcon icon={faBackwardStep} />
                </button>
                <button className="spotify-btn play" onClick={handlePlayPause} title={isPlaying ? 'Pause' : 'Play'}>
                  <FontAwesomeIcon icon={isPlaying ? faPause : faPlay} />
                </button>
                <button className="spotify-btn" onClick={async () => {
                  await nextTrack();
                  refresh();
                }} title="Next">
                  <FontAwesomeIcon icon={faForwardStep} />
                </button>
                <button className={`spotify-btn ${repeatState !== 'off' ? 'active' : ''}`} onClick={handleRepeat} title={`Loop: ${repeatState}`}>
                  <FontAwesomeIcon icon={faRepeat} />
                  {repeatState === 'track' ? <span className="spotify-icon-badge">1</span> : null}
                </button>
              </div>
            </div>
          </div>

          <div className="spotify-row">
            <div className="spotify-row" style={{ flex: 1 }}>
              <span className="spotify-subtitle"><FontAwesomeIcon icon={faVolumeHigh} style={{ marginRight: 6 }} />Volume</span>
              <input
                className="spotify-range"
                type="range"
                min={0}
                max={100}
                value={volume ?? state?.device?.volume_percent ?? 0}
                onChange={(e) => setVolumeState(Number(e.target.value))}
                onMouseUp={async (e) => {
                  const nextValue = Number(e.currentTarget.value);
                  setOptimistic({ volume_percent: nextValue });
                  setState((prev) => (prev ? { ...prev, device: { ...(prev.device || {}), volume_percent: nextValue } } : prev));
                  await setVolume(nextValue);
                  refresh();
                }}
                onTouchEnd={async (e) => {
                  const nextValue = Number(e.currentTarget.value);
                  setOptimistic({ volume_percent: nextValue });
                  setState((prev) => (prev ? { ...prev, device: { ...(prev.device || {}), volume_percent: nextValue } } : prev));
                  await setVolume(nextValue);
                  refresh();
                }}
              />
            </div>
            <div className="spotify-row spotify-device-row">
              <span className="spotify-subtitle">Device</span>
              <div className="spotify-device-select">
                <select
                  className="spotify-select"
                  value={deviceId || ''}
                  onChange={async (e) => {
                    const nextDevice = e.target.value;
                    if (!nextDevice) return;
                    await transferPlayback(nextDevice, true);
                    refresh();
                  }}
                >
                  <option value="">Select…</option>
                  {devices.map((device) => (
                    <option key={device.id} value={device.id}>{device.name}</option>
                  ))}
                </select>
                <span className="spotify-select-icon" aria-hidden="true">
                  <FontAwesomeIcon icon={faHeadphones} />
                </span>
              </div>
            </div>
          </div>

          {showQueueBlock ? (
            <div className="spotify-queue">
              <div className="spotify-subtitle"><FontAwesomeIcon icon={faListUl} style={{ marginRight: 6 }} />Queue</div>
              {queue?.queue?.length ? (
                queue.queue.slice(0, variant === 'widget' ? 4 : 8).map((track) => (
                  <div className="spotify-queue-item" key={track.id || track.uri}>
                    <div className="spotify-queue-meta">
                      <div className="spotify-queue-title">{track.name}</div>
                      <div className="spotify-queue-artist">{track.artists?.map((a) => a.name).join(', ')}</div>
                    </div>
                  </div>
                ))
              ) : (
                <div className="spotify-empty">Queue is empty.</div>
              )}
            </div>
          ) : (variant === 'widget' && showQueue ? (
            <div className="spotify-queue spotify-queue-placeholder" aria-hidden="true" />
          ) : null)}
        </div>
      </div>
    </div>
  );
}
