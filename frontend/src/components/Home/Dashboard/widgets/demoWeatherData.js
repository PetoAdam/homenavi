export const DEMO_WEATHER_CITY = 'Budapest';
export const DEMO_WEATHER_LOCATION_NAME = 'Budapest, Hungary';

export function getDemoWeatherPayload() {
  return {
    city: DEMO_WEATHER_LOCATION_NAME,
    current: {
      temp_c: 18,
      hi_c: 19,
      lo_c: 17,
      desc: 'overcast clouds',
      icon: 'cloud',
    },
    daily: [
      { hour: '00', temp_c: 18, icon: 'cloud' },
      { hour: '03', temp_c: 17, icon: 'cloud' },
      { hour: '06', temp_c: 18, icon: 'cloud_sun' },
      { hour: '09', temp_c: 23, icon: 'sun' },
      { hour: '12', temp_c: 27, icon: 'sun' },
      { hour: '15', temp_c: 26, icon: 'cloud_sun' },
      { hour: '18', temp_c: 22, icon: 'cloud' },
      { hour: '21', temp_c: 19, icon: 'cloud' },
    ],
    weekly: [
      { day: 'Fri', temp_c: 18, icon: 'cloud' },
      { day: 'Sat', temp_c: 22, icon: 'cloud_sun' },
      { day: 'Sun', temp_c: 24, icon: 'sun' },
      { day: 'Mon', temp_c: 21, icon: 'cloud' },
      { day: 'Tue', temp_c: 19, icon: 'rain' },
      { day: 'Wed', temp_c: 20, icon: 'cloud_sun' },
      { day: 'Thu', temp_c: 23, icon: 'sun' },
    ],
  };
}