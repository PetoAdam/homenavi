import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBolt,
  faCalendarDays,
  faChevronLeft,
  faChevronDown,
  faClock,
  faCloud,
  faCloudRain,
  faCloudSun,
  faGear,
  faLocationCrosshairs,
  faLocationDot,
  faSmog,
  faSnowflake,
  faSun,
  faTemperatureHalf,
  faWind,
  faSpinner,
} from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../context/AuthContext';
import { getWeather, reverseGeocode, searchLocations } from '../../../services/dashboardService';
import GlassSelect from '../../common/GlassSelect/GlassSelect';
import SearchBar from '../../common/SearchBar/SearchBar';
import './WeatherDetailView.css';

function getWeatherVariant(iconKey) {
  switch ((iconKey || '').toString()) {
    case 'sun':
    case 'cloud_sun':
      return 'sun';
    case 'rain':
      return 'rain';
    case 'storm':
      return 'storm';
    case 'snow':
      return 'snow';
    case 'fog':
      return 'fog';
    case 'wind':
      return 'wind';
    case 'cloud':
    default:
      return 'cloud';
  }
}

const WEATHER_ICON_MAP = {
  sun: faSun,
  cloud_sun: faCloudSun,
  cloud: faCloud,
  rain: faCloudRain,
  snow: faSnowflake,
  storm: faBolt,
  wind: faWind,
  fog: faSmog,
};

function getWeatherIcon(iconKey) {
  return WEATHER_ICON_MAP[(iconKey || '').toString()] || faSun;
}

export default function WeatherDetailView({
  instanceId,
  initialSettings,
  onClose,
  onSaveSettings,
}) {
  const { accessToken } = useAuth();
  const [closing, setClosing] = useState(false);
  const panelRef = useRef(null);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const [settings, setSettings] = useState(() => ({ ...(initialSettings || {}) }));
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);
  const [searching, setSearching] = useState(false);
  const [showDropdown, setShowDropdown] = useState(false);
  const blurTimeoutRef = useRef(null);
  const searchAnchorRef = useRef(null);
  const resultsPortalRef = useRef(null);
  const [dropdownStyle, setDropdownStyle] = useState(null);

  useEffect(() => {
    setSettings({ ...(initialSettings || {}) });
  }, [initialSettings]);

  const unit = (settings.unit || 'c').toString().toLowerCase() === 'f' ? 'f' : 'c';
  const unitSuffix = unit === 'f' ? '°F' : '°C';

  const lat = settings.lat;
  const lon = settings.lon;
  const city = settings.city || settings.location || '';
  const locationName = settings.location_name;

  const hasLocation =
    (Number.isFinite(Number(lat)) && Number.isFinite(Number(lon))) ||
    (typeof city === 'string' && city.trim().length > 0);

  const formatTemp = useCallback((tempC) => {
    const valueC = Number(tempC);
    if (!Number.isFinite(valueC)) return '--';
    const value = unit === 'f' ? (valueC * 9) / 5 + 32 : valueC;
    return Math.round(value);
  }, [unit]);

  const displayCity = useMemo(() => {
    return locationName || data?.city || city || 'Weather';
  }, [city, data?.city, locationName]);

  const variant = getWeatherVariant(data?.current?.icon);

  // Search results dropdown positioning + outside click handling.
  useEffect(() => {
    if (!showDropdown || !results.length) {
      setDropdownStyle(null);
      return undefined;
    }

    const update = () => {
      const el = searchAnchorRef.current;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      const margin = 10;
      const left = Math.max(margin, rect.left);
      const width = Math.min(rect.width, window.innerWidth - margin * 2);
      const top = Math.min(rect.bottom + 8, window.innerHeight - margin);
      const maxHeight = Math.max(120, window.innerHeight - top - margin);
      setDropdownStyle({ left, top, width, maxHeight });
    };

    const onPointerDown = (e) => {
      const anchor = searchAnchorRef.current;
      const portal = resultsPortalRef.current;
      const t = e.target;
      if (anchor && anchor.contains(t)) return;
      if (portal && portal.contains(t)) return;
      setShowDropdown(false);
    };

    update();
    window.addEventListener('resize', update);
    window.addEventListener('scroll', update, true);
    window.addEventListener('pointerdown', onPointerDown);
    return () => {
      window.removeEventListener('resize', update);
      window.removeEventListener('scroll', update, true);
      window.removeEventListener('pointerdown', onPointerDown);
    };
  }, [results.length, showDropdown]);

  // Fetch weather
  useEffect(() => {
    let cancelled = false;

    const fetchData = async () => {
      if (!accessToken) return;
      if (!hasLocation) {
        setData(null);
        setError(null);
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);

      try {
        const res = await getWeather({ lat, lon, city }, accessToken);
        if (cancelled) return;
        if (!res.success) {
          setError(res.error || 'Failed to load weather');
          setData(null);
        } else {
          setData(res.data);
        }
      } catch (e) {
        if (!cancelled) {
          setError(e?.message || 'Failed to load weather');
          setData(null);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    fetchData();
    return () => {
      cancelled = true;
    };
  }, [accessToken, city, hasLocation, lat, lon]);

  // Search locations
  useEffect(() => {
    let cancelled = false;

    const run = async () => {
      const q = (query || '').trim();
      if (!accessToken) return;
      if (q.length < 2) {
        setResults([]);
        setSearching(false);
        return;
      }

      setSearching(true);
      try {
        const res = await searchLocations(q, accessToken);
        if (cancelled) return;
        if (res.success) {
          setResults(Array.isArray(res.data) ? res.data : []);
          setShowDropdown(true);
        } else {
          setResults([]);
        }
      } finally {
        if (!cancelled) setSearching(false);
      }
    };

    run();
    return () => {
      cancelled = true;
    };
  }, [accessToken, query]);

  const persist = useCallback((next) => {
    setSettings(next);
    onSaveSettings?.(instanceId, next);
  }, [instanceId, onSaveSettings]);

  const handleSelectLocation = useCallback((loc) => {
    const next = {
      ...(settings || {}),
      lat: loc?.lat,
      lon: loc?.lon,
      city: loc?.name || loc?.display_name || loc?.city || '',
      location_name: loc?.display_name || loc?.name || '',
    };
    setQuery('');
    setResults([]);
    setShowDropdown(false);
    persist(next);
  }, [persist, settings]);

  const handleGeolocate = useCallback(() => {
    if (!accessToken) return;
    if (!navigator?.geolocation) return;

    navigator.geolocation.getCurrentPosition(async (pos) => {
      const nextLat = pos?.coords?.latitude;
      const nextLon = pos?.coords?.longitude;
      if (!Number.isFinite(nextLat) || !Number.isFinite(nextLon)) return;

      let name = '';
      try {
        const res = await reverseGeocode(nextLat, nextLon, accessToken);
        if (res?.success && res?.data) {
          name = res.data.name || res.data.display_name || '';
        }
      } catch {
        // ignore
      }

      persist({
        ...(settings || {}),
        lat: nextLat,
        lon: nextLon,
        city: name || (settings?.city || ''),
        location_name: name || settings?.location_name,
      });
    });
  }, [accessToken, persist, settings]);

  const requestClose = useCallback(() => {
    setClosing(true);
    setTimeout(() => onClose?.(), 180);
  }, [onClose]);

  // Match Device overlay behavior: prevent body scroll + allow Escape to close.
  useEffect(() => {
    document.body.classList.add('overlay-open');
    const onKeyDown = (e) => {
      if (e.key === 'Escape' && !closing) {
        e.preventDefault();
        requestClose();
      }
    };
    window.addEventListener('keydown', onKeyDown);

    // Focus the panel for accessibility.
    try {
      panelRef.current?.focus?.();
    } catch {
      // ignore
    }

    return () => {
      document.body.classList.remove('overlay-open');
      window.removeEventListener('keydown', onKeyDown);
    };
  }, [closing, requestClose]);

  useEffect(() => () => {
    if (blurTimeoutRef.current) {
      clearTimeout(blurTimeoutRef.current);
      blurTimeoutRef.current = null;
    }
  }, []);

  const current = data?.current || {};
  const hourly = Array.isArray(data?.daily) ? data.daily : [];
  const weekly = Array.isArray(data?.weekly) ? data.weekly : [];

  return (
    <div className={[
      'weather-detail',
      `weather-detail--${variant}`,
      closing ? 'weather-detail--closing' : '',
    ].filter(Boolean).join(' ')}>
      <div className="weather-detail__backdrop" onClick={requestClose} aria-hidden="true" />
      <div
        className="weather-detail__panel"
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        ref={panelRef}
      >
        <div className="weather-detail__background" aria-hidden="true" />

        <div className="weather-detail__header">
          <button type="button" className="weather-detail__back" onClick={requestClose}>
            <FontAwesomeIcon icon={faChevronLeft} />
            <span>Back</span>
          </button>

          <div className="weather-detail__title">
            <div className="weather-detail__title-main">{displayCity}</div>
            <div className="weather-detail__title-sub">
              <FontAwesomeIcon
                icon={getWeatherIcon(current.icon || data?.current?.icon)}
                className={[
                  'weather-detail__current-icon',
                  `weather-detail__current-icon--${variant}`,
                ].join(' ')}
              />
              <span>{current.desc || 'Weather'}</span>
            </div>
          </div>
        </div>

        <div className="weather-detail__content">
          <div className="weather-detail__hero">
            {loading ? (
              <div className="weather-detail__loading">
                <FontAwesomeIcon icon={faSpinner} spin />
                <span>Loading…</span>
              </div>
            ) : error ? (
              <div className="weather-detail__error">{error}</div>
            ) : !hasLocation ? (
              <div className="weather-detail__empty">Select a location to view weather.</div>
            ) : (
              <>
                <div className="weather-detail__hero-top">
                  <FontAwesomeIcon
                    icon={getWeatherIcon(current.icon || data?.current?.icon)}
                    className={[
                      'weather-detail__hero-icon',
                      `weather-detail__hero-icon--${variant}`,
                    ].join(' ')}
                    aria-hidden="true"
                  />
                  <div className="weather-detail__temp">{formatTemp(current.temp_c)}{unitSuffix}</div>
                </div>
                <div className="weather-detail__hero-desc">{current.desc || 'Weather'}</div>
                <div className="weather-detail__hilo">
                  H: {formatTemp(current.hi_c)}{unitSuffix} · L: {formatTemp(current.lo_c)}{unitSuffix}
                </div>
              </>
            )}
          </div>

          <div className="weather-detail__section">
            <div className="weather-detail__section-title">
              <FontAwesomeIcon icon={faClock} />
              <span>Next 24 hours</span>
            </div>
            <div className="weather-detail__hourly">
              {hourly.slice(0, 24).map((h, idx) => (
                <div className="weather-detail__hour" key={`${h.hour}-${idx}`}>
                  <div className="weather-detail__hour-time">{h.hour}:00</div>
                  <div className="weather-detail__hour-icon" aria-hidden="true">
                    <FontAwesomeIcon icon={getWeatherIcon(h.icon)} />
                  </div>
                  <div className="weather-detail__hour-temp">{formatTemp(h.temp_c)}{unitSuffix}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="weather-detail__section">
            <div className="weather-detail__section-title">
              <FontAwesomeIcon icon={faCalendarDays} />
              <span>Next 7 days</span>
            </div>
            <div className="weather-detail__weekly">
              {weekly.slice(0, 7).map((d, idx) => (
                <div className="weather-detail__day" key={`${d.day}-${idx}`}>
                  <div className="weather-detail__day-name">{d.day}</div>
                  <div className="weather-detail__day-icon" aria-hidden="true">
                    <FontAwesomeIcon icon={getWeatherIcon(d.icon)} />
                  </div>
                  <div className="weather-detail__day-temp">{formatTemp(d.temp_c)}{unitSuffix}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="weather-detail__section">
            <div className="weather-detail__section-title weather-detail__section-title--settings">
              <span className="weather-detail__section-title-left">
                <FontAwesomeIcon icon={faGear} />
                <span>Settings</span>
              </span>
              <button
                type="button"
                className={[
                  'weather-detail__settings-toggle',
                  settingsOpen ? 'open' : '',
                ].filter(Boolean).join(' ')}
                aria-expanded={settingsOpen ? 'true' : 'false'}
                onClick={() => setSettingsOpen(prev => !prev)}
                title={settingsOpen ? 'Hide settings' : 'Show settings'}
              >
                <FontAwesomeIcon icon={faChevronDown} />
              </button>
            </div>

            {settingsOpen ? (
              <div className="weather-detail__settings">
                <div className="weather-detail__settings-card">
                  <div className="weather-detail__settings-card-title">
                    <FontAwesomeIcon icon={faLocationDot} />
                    <span>Location</span>
                  </div>

                  <div className="weather-detail__settings-row weather-detail__settings-row--current">
                    <div className="weather-detail__settings-label">Current</div>
                    <div className="weather-detail__settings-value" title={displayCity}>
                      <div className="weather-detail__settings-value-main">{displayCity}</div>
                      {Number.isFinite(Number(lat)) && Number.isFinite(Number(lon)) ? (
                        <div className="weather-detail__settings-value-sub">
                          {Number(lat).toFixed(4)}, {Number(lon).toFixed(4)}
                        </div>
                      ) : null}
                    </div>
                    <button
                      type="button"
                      className="weather-detail__geo"
                      onClick={handleGeolocate}
                      title="Use my location"
                    >
                      <FontAwesomeIcon icon={faLocationCrosshairs} />
                    </button>
                  </div>

                  <div className="weather-detail__settings-row weather-detail__settings-row--stack">
                    <div className="weather-detail__settings-label">Search</div>
                    <div
                      className="weather-detail__location-search"
                      ref={searchAnchorRef}
                      onFocus={() => results.length > 0 && setShowDropdown(true)}
                    >
                      <SearchBar
                        value={query}
                        onChange={setQuery}
                        debounceMs={250}
                        placeholder="Search for a city…"
                        onClear={() => {
                          setQuery('');
                          setResults([]);
                          setShowDropdown(false);
                        }}
                        ariaLabel="Search for a city"
                      />
                      {searching ? (
                        <div className="weather-detail__searching">
                          <FontAwesomeIcon icon={faSpinner} spin />
                        </div>
                      ) : null}
                    </div>
                  </div>
                </div>

                <div className="weather-detail__settings-card">
                  <div className="weather-detail__settings-card-title">
                    <FontAwesomeIcon icon={faTemperatureHalf} />
                    <span>Units</span>
                  </div>

                  <div className="weather-detail__settings-row">
                    <div className="weather-detail__settings-label">Temperature unit</div>
                    <div className="weather-detail__settings-control">
                      <GlassSelect
                        value={unit}
                        options={[
                          { value: 'c', label: 'Celsius (°C)' },
                          { value: 'f', label: 'Fahrenheit (°F)' },
                        ]}
                        placeholder="Select unit"
                        ariaLabel="Temperature unit"
                        onChange={(next) => persist({ ...(settings || {}), unit: next })}
                      />
                    </div>
                  </div>
                </div>
              </div>
            ) : null}
          </div>
        </div>
      </div>

      {showDropdown && results.length > 0 && dropdownStyle ? createPortal(
        <div
          className="weather-detail__results weather-detail__results--portal"
          ref={resultsPortalRef}
          style={{
            left: `${dropdownStyle.left}px`,
            top: `${dropdownStyle.top}px`,
            width: `${dropdownStyle.width}px`,
            maxHeight: `${dropdownStyle.maxHeight}px`,
          }}
          role="listbox"
          onMouseDown={(e) => {
            // Keep focus on the SearchBar while interacting with the dropdown.
            e.preventDefault();
          }}
        >
          {results.map((loc, i) => (
            <button
              type="button"
              key={`${loc.lat}-${loc.lon}-${i}`}
              className="weather-detail__result"
              onClick={() => handleSelectLocation(loc)}
              role="option"
            >
              <div className="weather-detail__result-main">
                <span className="weather-detail__result-name">{loc.display_name || loc.name}</span>
                {loc.country ? <span className="weather-detail__country-pill">{loc.country}</span> : null}
              </div>
            </button>
          ))}
        </div>,
        document.body,
      ) : null}
    </div>
  );
}
