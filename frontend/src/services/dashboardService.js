import http from './httpClient';

/**
 * Dashboard Service API
 * Handles all dashboard-related API calls
 */

// Fetch user's dashboard (creates from default if missing)
export async function getDashboard(token) {
  return http.get('/api/dashboard/me', { token });
}

// Update user's dashboard
export async function updateDashboard(layoutVersion, doc, token) {
  return http.put(
    '/api/dashboard/me',
    { layout_version: layoutVersion, doc },
    { token }
  );
}

// Fetch widget catalog
export async function getWidgetCatalog(token) {
  return http.get('/api/widgets/catalog', { token });
}

// Fetch weather data (supports lat/lon or city)
export async function getWeather({ lat, lon, city } = {}, token) {
  const params = {};
  if (lat !== undefined && lon !== undefined) {
    params.lat = lat;
    params.lon = lon;
  } else if (city) {
    params.city = city;
  }
  return http.get('/api/weather', { token, params: Object.keys(params).length ? params : undefined });
}

// Search locations by name
export async function searchLocations(query, token) {
  if (!query || query.trim().length < 2) {
    return { success: true, data: [] };
  }
  return http.get('/api/weather/search', { token, params: { q: query.trim() } });
}

// Reverse geocode lat/lon to location name
export async function reverseGeocode(lat, lon, token) {
  return http.get('/api/weather/reverse', { token, params: { lat, lon } });
}

// Admin: Get default dashboard
export async function getDefaultDashboard(token) {
  return http.get('/api/dashboard/default', { token });
}

// Admin: Update default dashboard
export async function updateDefaultDashboard(title, doc, token) {
  return http.put('/api/dashboard/default', { title, doc }, { token });
}
