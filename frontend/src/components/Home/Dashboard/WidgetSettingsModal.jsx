import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faCheck,
  faTrashAlt,
  faSun,
  faMap,
  faLightbulb,
  faBolt,
  faLayerGroup,
  faChartLine,
  faQuestionCircle,
  faXmark,
  faSliders,
  faPalette,
  faToggleOn,
  faGaugeHigh,
  faThermometerHalf,
  faDroplet,
  faLocationCrosshairs,
  faSearch,
  faSpinner,
} from '@fortawesome/free-solid-svg-icons';
import BaseModal from '../../common/BaseModal/BaseModal';
import { useAuth } from '../../../context/AuthContext';
import useDeviceHubDevices from '../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../hooks/useErsInventory';
import { listWorkflows } from '../../../services/automationService';
import { searchLocations, reverseGeocode } from '../../../services/dashboardService';
import './WidgetSettingsModal.css';
import GlassSelect from '../../common/GlassSelect/GlassSelect';
import { collectDeviceStateFieldKeys } from '../../../utils/deviceFields';

// Icon mapping for widget types
const WIDGET_ICONS = {
  'homenavi.weather': faSun,
  'homenavi.map': faMap,
  'homenavi.device': faLightbulb,
  'homenavi.device.graph': faChartLine,
  'homenavi.automation.manual_trigger': faBolt,
  'homenavi.device.multi': faLayerGroup,
};

const WIDGET_NAMES = {
  'homenavi.weather': 'Weather',
  'homenavi.map': 'Map',
  'homenavi.device': 'Device',
  'homenavi.device.graph': 'Device Graph',
  'homenavi.automation.manual_trigger': 'Automation',
  'homenavi.device.multi': 'Quick Controls',
};

function getWidgetIcon(widgetType) {
  return WIDGET_ICONS[widgetType] || faQuestionCircle;
}

function getWidgetDisplayName(widgetType) {
  return WIDGET_NAMES[widgetType] || widgetType;
}

export default function WidgetSettingsModal({
  open,
  onClose,
  widgetItem,
  onSave,
  onRemove,
}) {
  const [settings, setSettings] = useState({});
  const [title, setTitle] = useState('');

  // ERS inventory for device selection
  const { accessToken, user } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const { devices: realtimeDevices } = useDeviceHubDevices({
    enabled: Boolean(isResidentOrAdmin),
    metadataMode: 'ws',
  });
  const { devices: ersDevices, loading: ersLoading } = useErsInventory({
    enabled: Boolean(isResidentOrAdmin && accessToken),
    accessToken,
    realtimeDevices,
  });

  // Compute default title based on widget type
  const computeDefaultTitle = useCallback((widgetType, widgetSettings) => {
    if (widgetSettings?.title) return widgetSettings.title;

    switch (widgetType) {
      case 'homenavi.device': {
        const deviceId = widgetSettings?.device_id;
        if (deviceId) {
          const device = [...(ersDevices || []), ...(realtimeDevices || [])].find(
            (d) => d.id === deviceId || d.ersId === deviceId || d.hdpId === deviceId
          );
          if (device) {
            return device.displayName || device.name || device.hdpId || '';
          }
        }
        return '';
      }
      case 'homenavi.map':
        return 'Map';
      case 'homenavi.weather':
        return widgetSettings?.city || 'Weather';
      case 'homenavi.device.graph': {
        const metric = widgetSettings?.metric_key || widgetSettings?.metric || '';
        const deviceId = widgetSettings?.device_id || widgetSettings?.hdp_device_id || '';
        const device = [...(ersDevices || []), ...(realtimeDevices || [])].find(
          (d) => d.id === deviceId || d.ersId === deviceId || d.hdpId === deviceId
        );
        const deviceName = device?.displayName || device?.name || device?.hdpId || '';
        if (deviceName && metric) return `${deviceName}: ${metric}`;
        if (metric) return `Graph: ${metric}`;
        return '';
      }
      case 'homenavi.device.multi':
        return 'Quick Controls';
      case 'homenavi.automation.manual_trigger':
        // Will be set when workflow is selected
        return '';
      default:
        return '';
    }
  }, [ersDevices, realtimeDevices]);

  // Initialize from widgetItem
  useEffect(() => {
    if (widgetItem) {
      const widgetSettings = widgetItem.settings || {};
      setSettings(widgetSettings);
      setTitle(computeDefaultTitle(widgetItem.widget_type, widgetSettings));
    }
  }, [widgetItem, computeDefaultTitle]);

  // Close on backdrop click
  const handleBackdropClick = (e) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  // Save handler
  const handleSave = () => {
    const newSettings = { ...settings };
    if (title.trim()) {
      newSettings.title = title.trim();
    } else {
      delete newSettings.title;
    }

    // Remove legacy height settings (height is stored in layouts).
    delete newSettings.height_mode;
    delete newSettings.height_rows;

    onSave(widgetItem.instance_id, newSettings);
    onClose();
  };

  // Update a specific setting
  const updateSetting = (key, value) => {
    setSettings((prev) => ({
      ...prev,
      [key]: value,
    }));
  };

  // Render widget-specific settings
  const renderWidgetSettings = () => {
    if (!widgetItem) return null;

    switch (widgetItem.widget_type) {
      case 'homenavi.weather':
        return <WeatherSettings settings={settings} updateSetting={updateSetting} />;
      case 'homenavi.device':
        return (
          <DeviceSettings
            settings={settings}
            updateSetting={updateSetting}
            ersDevices={ersDevices}
            ersLoading={ersLoading}
            realtimeDevices={realtimeDevices}
          />
        );
      case 'homenavi.device.graph':
        return (
          <DeviceGraphSettings
            settings={settings}
            updateSetting={updateSetting}
            ersDevices={ersDevices}
            ersLoading={ersLoading}
            realtimeDevices={realtimeDevices}
          />
        );
      case 'homenavi.device.multi':
        return (
          <MultiDeviceSettings
            settings={settings}
            updateSetting={updateSetting}
            ersDevices={ersDevices}
            ersLoading={ersLoading}
            realtimeDevices={realtimeDevices}
          />
        );
      case 'homenavi.map':
        return <MapSettings settings={settings} updateSetting={updateSetting} />;
      case 'homenavi.automation.manual_trigger':
        return <AutomationSettings settings={settings} updateSetting={updateSetting} accessToken={accessToken} />;
      default:
        return (
          <div className="widget-settings__no-settings">
            No configurable settings for this widget type.
          </div>
        );
    }
  };

  if (!open || !widgetItem) return null;

  return (
    <BaseModal
      open={open}
      onClose={onClose}
      backdropClassName="widget-settings__backdrop"
      dialogClassName="widget-settings-modal"
      closeAriaLabel="Close"
      onBackdropMouseDown={handleBackdropClick}
    >
      <div className="widget-settings__header">
        <div className="widget-settings__icon">
          <FontAwesomeIcon icon={getWidgetIcon(widgetItem.widget_type)} />
        </div>
        <div className="widget-settings__header-text">
          <h2 className="widget-settings__title">Widget Settings</h2>
          <span className="widget-settings__type">
            {getWidgetDisplayName(widgetItem.widget_type)}
          </span>
        </div>
      </div>

      {/* Content */}
      <div className="widget-settings__content">
        {/* Common: Title */}
        <div className="widget-settings__field">
          <label className="widget-settings__label" htmlFor="widget-title">
            Display Title
          </label>
          <input
            id="widget-title"
            type="text"
            className="widget-settings__input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={getWidgetDisplayName(widgetItem.widget_type)}
          />
        </div>

        {/* Height is managed via grid resize handles in edit mode. */}

        {/* Widget-specific settings */}
        {renderWidgetSettings()}
      </div>

      {/* Footer */}
      <div className="widget-settings__footer">
        <button
          className="widget-settings__btn widget-settings__btn--remove"
          onClick={() => {
            onRemove(widgetItem.instance_id);
            onClose();
          }}
        >
          <FontAwesomeIcon icon={faTrashAlt} />
          <span>Remove</span>
        </button>
        <button
          className="widget-settings__btn widget-settings__btn--save"
          onClick={handleSave}
        >
          <FontAwesomeIcon icon={faCheck} />
          <span>Save</span>
        </button>
      </div>
    </BaseModal>
  );
}

function normalizePreset(value) {
  const v = (value || '').toString().toLowerCase();
  if (['1h', '6h', '24h', '7d'].includes(v)) return v;
  return '24h';
}

// Device graph widget settings
function DeviceGraphSettings({ settings, updateSetting, ersDevices, ersLoading, realtimeDevices }) {
  const deviceOptions = useMemo(() => (Array.isArray(ersDevices) ? ersDevices : []), [ersDevices]);
  const deviceId = settings.device_id || settings.hdp_device_id || settings.ers_device_id || '';

  const selectedDevice = useMemo(() => {
    if (!deviceId) return null;
    const ersDevice = deviceOptions.find((d) => d.ersId === deviceId || d.id === deviceId || d.hdpId === deviceId);
    if (ersDevice) return ersDevice;
    const rtDevices = Array.isArray(realtimeDevices) ? realtimeDevices : [];
    return rtDevices.find((d) => d.id === deviceId || d.hdpId === deviceId) || null;
  }, [deviceId, deviceOptions, realtimeDevices]);

  const metricOptions = useMemo(() => {
    return collectDeviceStateFieldKeys(selectedDevice);
  }, [selectedDevice]);

  return (
    <>
      <div className="widget-settings__section">
        <h3 className="widget-settings__section-title">
          <FontAwesomeIcon icon={faChartLine} />
          Graph
        </h3>

        <div className="widget-settings__field">
          <label className="widget-settings__label">Device</label>
          {ersLoading ? (
            <div className="widget-settings__loading">
              <FontAwesomeIcon icon={faSpinner} spin />
              <span>Loading devices…</span>
            </div>
          ) : (
            <GlassSelect
              value={deviceId}
              options={deviceOptions.map((d) => ({
                value: d.id || d.ersId || d.hdpId,
                label: d.displayName || d.name || d.hdpId || d.ersId || d.id,
              }))}
              placeholder="— Select a device —"
              ariaLabel="Select a device"
              onChange={(next) => {
                updateSetting('device_id', next);
                updateSetting('metric_key', '');
              }}
            />
          )}
          <div className="widget-settings__hint">
            Uses the same history data as the Device detail page.
          </div>
        </div>

        <div className="widget-settings__field">
          <label className="widget-settings__label">Metric</label>
          {metricOptions.length > 0 ? (
            <GlassSelect
              value={(settings.metric_key || '').toString()}
              options={metricOptions.map((k) => ({ value: k, label: k }))}
              placeholder="— Select a metric —"
              ariaLabel="Select a metric"
              onChange={(next) => updateSetting('metric_key', next)}
            />
          ) : (
            <input
              type="text"
              className="widget-settings__input"
              value={(settings.metric_key || '').toString()}
              onChange={(e) => updateSetting('metric_key', e.target.value)}
              placeholder="e.g. temperature"
            />
          )}
        </div>

        <div className="widget-settings__field">
          <label className="widget-settings__label">Range</label>
          <GlassSelect
            value={normalizePreset(settings.range_preset)}
            options={[
              { value: '1h', label: 'Last 1 hour' },
              { value: '6h', label: 'Last 6 hours' },
              { value: '24h', label: 'Last 24 hours' },
              { value: '7d', label: 'Last 7 days' },
            ]}
            placeholder="Select range"
            ariaLabel="Select range"
            onChange={(next) => updateSetting('range_preset', next)}
          />
        </div>
      </div>
    </>
  );
}

// Weather-specific settings with location search and geolocation
function WeatherSettings({ settings, updateSetting }) {
  const { accessToken } = useAuth();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);
  const [searching, setSearching] = useState(false);
  const [geoLoading, setGeoLoading] = useState(false);
  const [showDropdown, setShowDropdown] = useState(false);
  const searchTimeoutRef = useRef(null);
  const inputRef = useRef(null);

  // Current location display
  const locationDisplay = settings.location_name || 
    (settings.lat && settings.lon ? `${settings.lat.toFixed(2)}, ${settings.lon.toFixed(2)}` : '');

  // Debounced search
  useEffect(() => {
    if (query.length < 2) {
      setResults([]);
      setShowDropdown(false);
      return;
    }

    clearTimeout(searchTimeoutRef.current);
    searchTimeoutRef.current = setTimeout(async () => {
      setSearching(true);
      try {
        const res = await searchLocations(query, accessToken);
        if (res.success && Array.isArray(res.data)) {
          setResults(res.data);
          setShowDropdown(true);
        }
      } catch {
        setResults([]);
      } finally {
        setSearching(false);
      }
    }, 300);

    return () => clearTimeout(searchTimeoutRef.current);
  }, [query, accessToken]);

  // Handle location selection
  const selectLocation = (loc) => {
    updateSetting('lat', loc.lat);
    updateSetting('lon', loc.lon);
    updateSetting('location_name', loc.display_name || loc.name);
    // Clear city if set
    updateSetting('city', undefined);
    setQuery('');
    setResults([]);
    setShowDropdown(false);
  };

  // Use browser geolocation
  const handleGeolocate = async () => {
    if (!navigator.geolocation) {
      return;
    }

    setGeoLoading(true);
    navigator.geolocation.getCurrentPosition(
      async (position) => {
        const { latitude, longitude } = position.coords;
        updateSetting('lat', latitude);
        updateSetting('lon', longitude);
        
        // Reverse geocode to get location name
        try {
          const res = await reverseGeocode(latitude, longitude, accessToken);
          if (res.success && res.data?.name) {
            updateSetting('location_name', res.data.name);
          } else {
            updateSetting('location_name', `${latitude.toFixed(2)}, ${longitude.toFixed(2)}`);
          }
        } catch {
          updateSetting('location_name', `${latitude.toFixed(2)}, ${longitude.toFixed(2)}`);
        }
        updateSetting('city', undefined);
        setGeoLoading(false);
      },
      () => {
        setGeoLoading(false);
      },
      { enableHighAccuracy: false, timeout: 10000 }
    );
  };

  // Clear location
  const handleClear = () => {
    updateSetting('lat', undefined);
    updateSetting('lon', undefined);
    updateSetting('location_name', undefined);
    updateSetting('city', undefined);
  };

  return (
    <>
      <div className="widget-settings__field">
        <label className="widget-settings__label">
          Location
        </label>
        {locationDisplay && (
          <div className="widget-settings__location-current">
            <span>{locationDisplay}</span>
            <button
              type="button"
              className="widget-settings__location-clear"
              onClick={handleClear}
              aria-label="Clear location"
            >
              <FontAwesomeIcon icon={faXmark} />
            </button>
          </div>
        )}
        <div className="widget-settings__location-search">
          <div className="widget-settings__search-input-wrapper">
            <FontAwesomeIcon icon={faSearch} className="widget-settings__search-icon" />
            <input
              ref={inputRef}
              type="text"
              className="widget-settings__input widget-settings__input--search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onFocus={() => results.length > 0 && setShowDropdown(true)}
              onBlur={() => setTimeout(() => setShowDropdown(false), 200)}
              placeholder="Search for a city..."
            />
            {searching && (
              <FontAwesomeIcon icon={faSpinner} spin className="widget-settings__search-spinner" />
            )}
          </div>
          <button
            type="button"
            className="widget-settings__btn widget-settings__btn--geo"
            onClick={handleGeolocate}
            disabled={geoLoading}
            title="Use my location"
          >
            <FontAwesomeIcon icon={geoLoading ? faSpinner : faLocationCrosshairs} spin={geoLoading} />
          </button>
        </div>
        {showDropdown && results.length > 0 && (
          <ul className="widget-settings__search-results">
            {results.map((loc, idx) => (
              <li key={`${loc.lat}-${loc.lon}-${idx}`}>
                <button
                  type="button"
                  onClick={() => selectLocation(loc)}
                >
                  {loc.display_name || loc.name}
                  {loc.country && <span className="widget-settings__search-country">{loc.country}</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
        <div className="widget-settings__hint">
          Search for a city or use your current location.
        </div>
      </div>

      <div className="widget-settings__field">
        <label className="widget-settings__label">
          Temperature Unit
        </label>
        <GlassSelect
          value={(settings.unit || 'c').toString().toLowerCase() === 'f' ? 'f' : 'c'}
          options={[
            { value: 'c', label: 'Celsius (°C)' },
            { value: 'f', label: 'Fahrenheit (°F)' },
          ]}
          placeholder="Select unit"
          ariaLabel="Temperature unit"
          onChange={(next) => updateSetting('unit', next)}
        />
      </div>
    </>
  );
}

// Device-specific settings with HDP metadata-based control picker
function DeviceSettings({ settings, updateSetting, ersDevices, ersLoading, realtimeDevices }) {
  const deviceOptions = useMemo(() => (Array.isArray(ersDevices) ? ersDevices : []), [ersDevices]);
  const deviceId = settings.device_id || '';

  // Find selected device and get its inputs
  const selectedDevice = useMemo(() => {
    if (!deviceId) return null;
    // Check ERS devices first
    const ersDevice = deviceOptions.find((d) => 
      d.ersId === deviceId || d.id === deviceId || d.hdpId === deviceId
    );
    if (ersDevice) return ersDevice;
    // Fallback to realtime devices
    const rtDevices = Array.isArray(realtimeDevices) ? realtimeDevices : [];
    return rtDevices.find((d) => d.id === deviceId || d.hdpId === deviceId);
  }, [deviceId, deviceOptions, realtimeDevices]);

  // Get available inputs from device
  const availableInputs = useMemo(() => {
    if (!selectedDevice) return [];
    const inputs = Array.isArray(selectedDevice.inputs) ? selectedDevice.inputs : [];
    return inputs.filter((inp) => {
      const type = (inp.type || '').toLowerCase();
      return ['toggle', 'slider', 'number', 'select', 'color'].includes(type);
    });
  }, [selectedDevice]);

  // Get available state fields
  const availableFields = useMemo(() => {
    if (!selectedDevice) return [];
    const state = selectedDevice.state || {};
    return Object.keys(state).filter((key) => 
      !['state', 'on', 'brightness', 'color', 'color_temp'].includes(key.toLowerCase())
    );
  }, [selectedDevice]);

  // Currently selected controls and fields
  const selectedControls = settings.controls || [];
  const selectedFields = settings.fields || [];

  // Get icon for input type
  const getInputIcon = (type) => {
    switch ((type || '').toLowerCase()) {
      case 'toggle': return faToggleOn;
      case 'slider': return faSliders;
      case 'color': return faPalette;
      case 'number': return faGaugeHigh;
      default: return faSliders;
    }
  };

  // Get icon for field key
  const getFieldIcon = (key) => {
    const lower = (key || '').toLowerCase();
    if (lower.includes('temp')) return faThermometerHalf;
    if (lower.includes('humid')) return faDroplet;
    if (lower.includes('power') || lower.includes('voltage')) return faBolt;
    return faGaugeHigh;
  };

  // Add control
  const handleAddControl = (inputKey) => {
    if (!selectedControls.includes(inputKey)) {
      updateSetting('controls', [...selectedControls, inputKey]);
    }
  };

  // Remove control
  const handleRemoveControl = (inputKey) => {
    updateSetting('controls', selectedControls.filter((k) => k !== inputKey));
  };

  // Add field
  const handleAddField = (fieldKey) => {
    if (!selectedFields.includes(fieldKey)) {
      updateSetting('fields', [...selectedFields, fieldKey]);
    }
  };

  // Remove field
  const handleRemoveField = (fieldKey) => {
    updateSetting('fields', selectedFields.filter((k) => k !== fieldKey));
  };

  // Get input display name
  const getInputDisplayName = (inp) => {
    return inp.name || inp.label || inp.property || inp.id || 'Unknown';
  };

  // Controls not yet added
  const availableControlsToAdd = availableInputs.filter((inp) => {
    const key = inp.id || inp.property || inp.name;
    return key && !selectedControls.includes(key);
  });

  // Fields not yet added
  const availableFieldsToAdd = availableFields.filter((f) => !selectedFields.includes(f));

  return (
    <>
      <div className="widget-settings__field">
        <label className="widget-settings__label" htmlFor="device-id">
          Device
        </label>
        <GlassSelect
          value={deviceId}
          options={deviceOptions.map((device) => ({
            value: device.id || device.ersId || device.hdpId,
            label: device.displayName || device.name || device.hdpId || device.ersId || device.id,
          }))}
          placeholder={ersLoading ? 'Loading…' : '— Select a device —'}
          ariaLabel="Select a device"
          onChange={(next) => {
            updateSetting('device_id', next);
            updateSetting('controls', []);
            updateSetting('fields', []);
          }}
        />
      </div>

      {selectedDevice && (
        <>
          <div className="widget-settings__field">
            <label className="widget-settings__label">Field View</label>
            <div className="widget-settings__toggle-row" role="radiogroup" aria-label="Field view">
              {[
                { value: 'cards', label: 'Cards' },
                { value: 'list', label: 'List' },
              ].map((opt) => {
                const active = (settings.fields_layout || 'cards') === opt.value;
                return (
                  <button
                    key={opt.value}
                    type="button"
                    className={`widget-settings__toggle-btn${active ? ' active' : ''}`}
                    onClick={() => updateSetting('fields_layout', opt.value)}
                  >
                    {active && <FontAwesomeIcon icon={faCheck} className="widget-settings__toggle-check" />}
                    {opt.label}
                  </button>
                );
              })}
            </div>
          </div>

          {/* Controls Section */}
          <div className="widget-settings__field">
            <label className="widget-settings__label">Controls</label>
            
            {/* Selected controls list */}
            <div className="widget-settings__picker-list">
              {selectedControls.length === 0 && (
                <div className="widget-settings__picker-empty">
                  No controls selected. Add controls below.
                </div>
              )}
              {selectedControls.map((key) => {
                const input = availableInputs.find((inp) => 
                  (inp.id || inp.property || inp.name) === key
                );
                return (
                  <div key={key} className="widget-settings__picker-item">
                    <FontAwesomeIcon 
                      icon={getInputIcon(input?.type)} 
                      className="widget-settings__picker-item-icon" 
                    />
                    <span className="widget-settings__picker-item-name">
                      {input ? getInputDisplayName(input) : key}
                    </span>
                    <button
                      type="button"
                      className="widget-settings__picker-item-remove"
                      onClick={() => handleRemoveControl(key)}
                      title="Remove"
                    >
                      <FontAwesomeIcon icon={faXmark} />
                    </button>
                  </div>
                );
              })}
            </div>

            {/* Add control dropdown */}
            {availableControlsToAdd.length > 0 && (
              <div className="widget-settings__picker-add">
                <GlassSelect
                  value={''}
                  options={availableControlsToAdd.map((inp) => {
                    const key = inp.id || inp.property || inp.name;
                    return { value: key, label: `${getInputDisplayName(inp)} (${inp.type})` };
                  })}
                  placeholder="+ Add control…"
                  ariaLabel="Add control"
                  onChange={(next) => {
                    if (next) handleAddControl(next);
                  }}
                />
              </div>
            )}

            {availableInputs.length === 0 && (
              <div className="widget-settings__hint">
                This device has no controllable inputs.
              </div>
            )}
          </div>

          {/* Fields Section */}
          <div className="widget-settings__field">
            <label className="widget-settings__label">State Fields to Display</label>
            
            {/* Selected fields list */}
            <div className="widget-settings__picker-list">
              {selectedFields.length === 0 && (
                <div className="widget-settings__picker-empty">
                  No fields selected. Add fields below.
                </div>
              )}
              {selectedFields.map((key) => (
                <div key={key} className="widget-settings__picker-item">
                  <FontAwesomeIcon 
                    icon={getFieldIcon(key)} 
                    className="widget-settings__picker-item-icon" 
                  />
                  <span className="widget-settings__picker-item-name">{key}</span>
                  <button
                    type="button"
                    className="widget-settings__picker-item-remove"
                    onClick={() => handleRemoveField(key)}
                    title="Remove"
                  >
                    <FontAwesomeIcon icon={faXmark} />
                  </button>
                </div>
              ))}
            </div>

            {/* Add field dropdown */}
            {availableFieldsToAdd.length > 0 && (
              <div className="widget-settings__picker-add">
                <GlassSelect
                  value={''}
                  options={availableFieldsToAdd.map((key) => ({ value: key, label: key }))}
                  placeholder="+ Add field…"
                  ariaLabel="Add field"
                  onChange={(next) => {
                    if (next) handleAddField(next);
                  }}
                />
              </div>
            )}

            {availableFields.length === 0 && (
              <div className="widget-settings__hint">
                This device has no state fields to display.
              </div>
            )}
          </div>
        </>
      )}
    </>
  );
}

function MultiDeviceSettings({ settings, updateSetting, ersDevices, ersLoading, realtimeDevices }) {
  const selectedIds = Array.isArray(settings.device_ids) ? settings.device_ids : [];

  const mergedDevices = useMemo(() => {
    const list = [...(Array.isArray(ersDevices) ? ersDevices : []), ...(Array.isArray(realtimeDevices) ? realtimeDevices : [])];
    const map = new Map();
    list.forEach((device) => {
      const id = device?.id || device?.ersId || device?.hdpId;
      if (!id || map.has(id)) return;
      map.set(id, device);
    });
    return Array.from(map.values()).filter((device) => {
      const inputs = Array.isArray(device?.inputs) ? device.inputs : [];
      if (inputs.some((inp) => (inp?.type || '').toLowerCase() === 'toggle')) return Boolean(device?.id);
      const state = device?.state || {};
      return Boolean(device?.id) && ['on', 'state', 'power'].some((key) => key in state);
    });
  }, [ersDevices, realtimeDevices]);

  const toggleDevice = (id) => {
    const next = selectedIds.includes(id)
      ? selectedIds.filter((item) => item !== id)
      : [...selectedIds, id];
    updateSetting('device_ids', next);
  };

  return (
    <>
      <div className="widget-settings__field">
        <label className="widget-settings__label">Devices</label>
        {ersLoading ? (
          <div className="widget-settings__loading">
            <FontAwesomeIcon icon={faSpinner} spin />
            <span>Loading devices…</span>
          </div>
        ) : (
          <div className="widget-settings__checkboxes">
            {mergedDevices.map((device) => {
              const id = device.id || device.ersId || device.hdpId;
              if (!id) return null;
              const label = device.displayName || device.name || device.hdpId || device.ersId || device.id;
              return (
                <label key={id} className="widget-settings__checkbox">
                  <input
                    type="checkbox"
                    checked={selectedIds.includes(id)}
                    onChange={() => toggleDevice(id)}
                  />
                  <span>{label}</span>
                </label>
              );
            })}
          </div>
        )}
        <div className="widget-settings__hint">
          Select devices with on/off controls. Read-only devices are hidden.
        </div>
      </div>
    </>
  );
}

// Map-specific settings
function MapSettings({ settings, updateSetting }) {
  return (
    <>
      <div className="widget-settings__field">
        <label className="widget-settings__label" htmlFor="map-floor">
          Floor (optional)
        </label>
        <input
          id="map-floor"
          type="text"
          className="widget-settings__input"
          value={settings.floor || ''}
          onChange={(e) => updateSetting('floor', e.target.value || undefined)}
          placeholder="Default floor"
        />
      </div>
      <div className="widget-settings__field">
        <label className="widget-settings__label">Options</label>
        <div className="widget-settings__checkboxes">
          <label className="widget-settings__checkbox">
            <input
              type="checkbox"
              checked={settings.show_devices !== false}
              onChange={(e) => updateSetting('show_devices', e.target.checked)}
            />
            <span>Show Devices</span>
          </label>
          <label className="widget-settings__checkbox">
            <input
              type="checkbox"
              checked={settings.show_rooms !== false}
              onChange={(e) => updateSetting('show_rooms', e.target.checked)}
            />
            <span>Show Rooms</span>
          </label>
        </div>
      </div>
    </>
  );
}

// Automation-specific settings with workflow picker
function AutomationSettings({ settings, updateSetting, accessToken }) {
  const [workflows, setWorkflows] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    const fetchWorkflows = async () => {
      if (!accessToken) {
        setLoading(false);
        return;
      }

      try {
        const res = await listWorkflows(accessToken);
        if (cancelled) return;

        if (res.success) {
          const wfs = Array.isArray(res.data) ? res.data : (res.data?.workflows || []);
          // Filter to workflows with manual trigger
          const manualTriggerWorkflows = wfs.filter((wf) => {
            const def = wf.definition || {};
            const nodes = def.nodes || [];
            return nodes.some((n) => n.kind === 'trigger.manual' || n.type === 'trigger.manual');
          });
          setWorkflows(manualTriggerWorkflows);
        }
      } catch (err) {
        console.error('Failed to load workflows:', err);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    fetchWorkflows();
    return () => { cancelled = true; };
  }, [accessToken]);

  return (
    <>
      <div className="widget-settings__field">
        <label className="widget-settings__label" htmlFor="automation-workflow">
          Workflow
        </label>
        <GlassSelect
          value={settings.workflow_id || ''}
          options={workflows.map((wf) => ({ value: wf.id, label: wf.name || wf.id }))}
          placeholder={loading ? 'Loading workflows…' : '— Select a workflow —'}
          ariaLabel="Select a workflow"
          onChange={(next) => updateSetting('workflow_id', next || undefined)}
        />
        {!loading && workflows.length === 0 && (
          <div className="widget-settings__hint">
            No workflows with manual trigger found. Create one in the Automation page.
          </div>
        )}
      </div>
      <div className="widget-settings__field">
        <label className="widget-settings__label" htmlFor="automation-label">
          Button Label (optional)
        </label>
        <input
          id="automation-label"
          type="text"
          className="widget-settings__input"
          value={settings.button_label || ''}
          onChange={(e) => updateSetting('button_label', e.target.value || undefined)}
          placeholder="Run"
        />
      </div>
    </>
  );
}
