import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faSun,
  faCloudSun,
  faCloudRain,
  faCloud,
  faLocationDot,
  faSnowflake,
  faBolt,
  faWind,
  faSmog,
  faChevronLeft,
  faChevronRight,
} from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../../context/AuthContext';
import { getWeather } from '../../../../services/dashboardService';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import WeatherDetailView from '../WeatherDetailView';
import './WeatherWidget.css';

// Icon mapping from backend icon strings
const ICON_MAP = {
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
  return ICON_MAP[iconKey] || faSun;
}

function getWeatherVariant(iconKey) {
  switch (iconKey) {
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
    case 'wind':
    case 'cloud':
    default:
      return 'cloud';
  }
}

function WeatherWidget({
  instanceId,
  settings = {},
  // enabled is unused
  editMode,
  onSettings,
  onSaveSettings,
  onRemove,
}) {
  const { accessToken } = useAuth();
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [status, setStatus] = useState(null);
  const [tab, setTab] = useState('day');
  const [detailOpen, setDetailOpen] = useState(false);

  const forecastRowRef = useRef(null);
  const [isDragging, setIsDragging] = useState(false);
  const [startX, setStartX] = useState(0);
  const [scrollLeft, setScrollLeft] = useState(0);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  // Extract location from settings
  const lat = settings.lat;
  const lon = settings.lon;
  const city = settings.city || settings.location || '';
  const locationName = settings.location_name;

  const unit = (settings.unit || 'c').toString().toLowerCase() === 'f' ? 'f' : 'c';
  const unitSuffix = unit === 'f' ? '°F' : '°C';

  const hasLocation =
    (Number.isFinite(Number(lat)) && Number.isFinite(Number(lon))) ||
    (typeof city === 'string' && city.trim().length > 0);

  const formatTemp = (tempC) => {
    const valueC = Number(tempC);
    if (!Number.isFinite(valueC)) return '--';
    const value = unit === 'f' ? (valueC * 9) / 5 + 32 : valueC;
    return Math.round(value);
  };

  // Fetch weather data
  useEffect(() => {
    let cancelled = false;

    const fetchData = async () => {
      if (!accessToken) return;

      if (!hasLocation) {
        setData(null);
        setError(null);
        setStatus(null);
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);
      setStatus(null);

      try {
        const res = await getWeather({ lat, lon, city }, accessToken);

        if (cancelled) return;

        if (!res.success) {
          if (res.status === 401) {
            setStatus(401);
          } else if (res.status === 403) {
            setStatus(403);
          } else {
            setError(res.error || 'Failed to load weather');
          }
        } else {
          setData(res.data);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err.message || 'Failed to load weather');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchData();

    return () => {
      cancelled = true;
    };
  }, [accessToken, hasLocation, lat, lon, city]);

  // Drag-to-scroll logic
  const handleMouseDown = useCallback((e) => {
    if (editMode) return;
    setIsDragging(true);
    setStartX(e.pageX - forecastRowRef.current.offsetLeft);
    setScrollLeft(forecastRowRef.current.scrollLeft);
  }, [editMode]);

  const handleMouseLeave = useCallback(() => setIsDragging(false), []);
  const handleMouseUp = useCallback(() => setIsDragging(false), []);

  const handleMouseMove = useCallback((e) => {
    if (!isDragging || editMode) return;
    e.preventDefault();
    const x = e.pageX - forecastRowRef.current.offsetLeft;
    const walk = (x - startX) * 1.2;
    forecastRowRef.current.scrollLeft = scrollLeft - walk;
  }, [isDragging, editMode, startX, scrollLeft]);

  const updateScrollState = useCallback(() => {
    const el = forecastRowRef.current;
    if (!el) return;
    const max = Math.max(0, el.scrollWidth - el.clientWidth);
    const x = el.scrollLeft;
    setCanScrollLeft(x > 2);
    setCanScrollRight(x < max - 2);
  }, []);

  useEffect(() => {
    updateScrollState();
    const el = forecastRowRef.current;
    if (!el) return undefined;

    let ro;
    try {
      ro = new ResizeObserver(() => updateScrollState());
      ro.observe(el);
    } catch {
      // ignore
    }

    return () => {
      try {
        ro?.disconnect();
      } catch {
        // ignore
      }
    };
  }, [tab, data, updateScrollState]);

  const scrollForecastBy = useCallback((delta) => {
    const el = forecastRowRef.current;
    if (!el) return;
    el.scrollBy({ left: delta, behavior: 'smooth' });
  }, []);


  // Parse data
  const current = data?.current || {};
  const daily = Array.isArray(data?.daily) ? data.daily : [];
  const weekly = Array.isArray(data?.weekly) ? data.weekly : [];
  const displayCity = locationName || data?.city || city || 'Unknown';
  const variant = getWeatherVariant(current.icon);

  return (
    <WidgetShell
      className={`weather-widget weather-widget--${variant}`}
      loading={loading}
      error={error}
      status={status}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      onClick={() => {
        if (editMode) return;
        setDetailOpen(true);
      }}
    >
      <div className="weather-widget__content">
        {!hasLocation ? (
          <div className="weather-widget__empty">
            <div className="weather-widget__empty-title">No location selected</div>
            <div className="weather-widget__empty-subtitle">Open settings to choose a city or use geolocation.</div>
            <button
              type="button"
              className="weather-widget__empty-btn"
              onClick={(e) => {
                e.stopPropagation();
                onSettings?.();
              }}
              disabled={editMode}
            >
              Select location
            </button>
          </div>
        ) : (
          <>
            {/* Top row: icon + temp */}
            <div className="weather-widget__top-row">
              <FontAwesomeIcon
                icon={getWeatherIcon(current.icon)}
                className="weather-widget__main-icon"
              />
              <div className="weather-widget__temp-block">
                <div className="weather-widget__temp">{formatTemp(current.temp_c)}{unitSuffix}</div>
              </div>
            </div>

            {/* Description row */}
            <div className="weather-widget__desc-row">
              <span className="weather-widget__desc">{current.desc || 'Weather'}</span>
              <span className="weather-widget__hilo">
                H: {formatTemp(current.hi_c)}{unitSuffix}  L: {formatTemp(current.lo_c)}{unitSuffix}
              </span>
            </div>

            {/* Location */}
            <div className="weather-widget__location">
              <FontAwesomeIcon icon={faLocationDot} className="weather-widget__location-icon" />
              <span>{displayCity}</span>
            </div>

            {/* Forecast section */}
            <div className="weather-widget__forecast-section">
              <div className="weather-widget__forecast-controls">
                <div className="weather-widget__tabs">
                  <button
                    className={tab === 'day' ? 'active' : ''}
                    onClick={(e) => {
                      e.stopPropagation();
                      if (!editMode) setTab('day');
                    }}
                    disabled={editMode}
                  >
                    Day
                  </button>
                  <button
                    className={tab === 'week' ? 'active' : ''}
                    onClick={(e) => {
                      e.stopPropagation();
                      if (!editMode) setTab('week');
                    }}
                    disabled={editMode}
                  >
                    Week
                  </button>
                </div>
              </div>

              <div className="weather-widget__forecast-carousel">
                {!editMode && (
                  <>
                    <button
                      type="button"
                      className="weather-widget__scroll-btn weather-widget__scroll-btn--left"
                      onClick={(e) => {
                        e.stopPropagation();
                        scrollForecastBy(-160);
                      }}
                      disabled={!canScrollLeft}
                      aria-label="Scroll left"
                      title="Scroll left"
                    >
                      <FontAwesomeIcon icon={faChevronLeft} />
                    </button>
                    <button
                      type="button"
                      className="weather-widget__scroll-btn weather-widget__scroll-btn--right"
                      onClick={(e) => {
                        e.stopPropagation();
                        scrollForecastBy(160);
                      }}
                      disabled={!canScrollRight}
                      aria-label="Scroll right"
                      title="Scroll right"
                    >
                      <FontAwesomeIcon icon={faChevronRight} />
                    </button>
                  </>
                )}

                <div
                  className="weather-widget__forecast-row"
                  ref={forecastRowRef}
                  onClick={(e) => e.stopPropagation()}
                  onScroll={updateScrollState}
                  onMouseDown={(e) => {
                    e.stopPropagation();
                    handleMouseDown(e);
                  }}
                  onMouseLeave={(e) => {
                    e.stopPropagation();
                    handleMouseLeave(e);
                  }}
                  onMouseUp={(e) => {
                    e.stopPropagation();
                    handleMouseUp(e);
                  }}
                  onMouseMove={(e) => {
                    e.stopPropagation();
                    handleMouseMove(e);
                  }}
                >
                  {(tab === 'day' ? daily : weekly).map((f, i) => (
                    <div className="weather-widget__forecast-item" key={i}>
                      <span className="weather-widget__forecast-label">
                        {tab === 'day' ? `${f.hour ?? ''}:00` : (f.day || '')}
                      </span>
                      <FontAwesomeIcon
                        icon={getWeatherIcon(f.icon)}
                        className="weather-widget__forecast-icon"
                      />
                      <span className="weather-widget__forecast-temp">{formatTemp(f.temp_c)}{unitSuffix}</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </>
        )}
      </div>

      {detailOpen && createPortal(
        <WeatherDetailView
          instanceId={instanceId}
          initialSettings={settings}
          onSaveSettings={onSaveSettings}
          onClose={() => setDetailOpen(false)}
        />,
        document.body,
      )}
    </WidgetShell>
  );
}

WeatherWidget.defaultHeight = 5;

export default WeatherWidget;
