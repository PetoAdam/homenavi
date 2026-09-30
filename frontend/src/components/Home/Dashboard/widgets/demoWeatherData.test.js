import { describe, expect, it } from 'vitest';

import { DEMO_WEATHER_CITY, DEMO_WEATHER_LOCATION_NAME, getDemoWeatherPayload } from './demoWeatherData';

describe('getDemoWeatherPayload', () => {
  it('returns a stable Budapest weather fixture for demo mode', () => {
    const payload = getDemoWeatherPayload();

    expect(DEMO_WEATHER_CITY).toBe('Budapest');
    expect(DEMO_WEATHER_LOCATION_NAME).toBe('Budapest, Hungary');
    expect(payload.city).toBe(DEMO_WEATHER_LOCATION_NAME);
    expect(payload.current.temp_c).toBe(18);
    expect(payload.daily).toHaveLength(8);
    expect(payload.weekly).toHaveLength(7);
  });
});