import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSun, faCloudSun, faCloudRain, faCloud, faLocationDot, faChevronLeft, faChevronRight } from '@fortawesome/free-solid-svg-icons';
import React, { useRef, useState } from 'react';
import GlassCard from '../../common/GlassCard/GlassCard';
import './WeatherCard.css';

export default function WeatherCard() {
  // Mock weather data
  const weather = {
    temp: 22,
    city: 'Budapest',
    desc: 'Sunny',
    icon: faSun,
  };
  const [tab, setTab] = useState('today');
  // Mock forecast data
  const daily = [
    { hour: '09', icon: faSun, temp: 20 },
    { hour: '12', icon: faCloudSun, temp: 22 },
    { hour: '15', icon: faCloud, temp: 21 },
    { hour: '18', icon: faCloudRain, temp: 18 },
    { hour: '21', icon: faCloud, temp: 16 },
  ];
  const weekly = [
    { day: 'Fri', icon: faSun, temp: 22 },
    { day: 'Sat', icon: faCloudSun, temp: 21 },
    { day: 'Sun', icon: faCloud, temp: 19 },
    { day: 'Mon', icon: faCloudRain, temp: 17 },
    { day: 'Tue', icon: faCloud, temp: 18 },
    { day: 'Wed', icon: faSun, temp: 20 },
    { day: 'Thu', icon: faCloudSun, temp: 21 },
  ];

  const forecastRowRef = useRef(null);
  // Drag-to-scroll logic
  const [isDragging, setIsDragging] = useState(false);
  const [startX, setStartX] = useState(0);
  const [scrollLeft, setScrollLeft] = useState(0);

  const handleMouseDown = (e) => {
    setIsDragging(true);
    setStartX(e.pageX - forecastRowRef.current.offsetLeft);
    setScrollLeft(forecastRowRef.current.scrollLeft);
  };
  const handleMouseLeave = () => setIsDragging(false);
  const handleMouseUp = () => setIsDragging(false);
  const handleMouseMove = (e) => {
    if (!isDragging) return;
    e.preventDefault();
    const x = e.pageX - forecastRowRef.current.offsetLeft;
    const walk = (x - startX) * 1.2; // scroll speed
    forecastRowRef.current.scrollLeft = scrollLeft - walk;
  };

  // Carousel scroll buttons
  const scrollByAmount = 80;
  const scrollLeftBtn = () => {
    if (forecastRowRef.current) forecastRowRef.current.scrollBy({ left: -scrollByAmount, behavior: 'smooth' });
  };
  const scrollRightBtn = () => {
    if (forecastRowRef.current) forecastRowRef.current.scrollBy({ left: scrollByAmount, behavior: 'smooth' });
  };

  return (
    <GlassCard className="weather-card">
      <div className="weather-cc-vertical">
        <div className="weather-cc-toprow">
          <FontAwesomeIcon icon={weather.icon} size="3x" className="weather-cc-icon" />
          <div className="weather-cc-tempblock">
            <div className="weather-cc-temp">{weather.temp}&deg;C</div>
          </div>
        </div>
        <div className="weather-cc-descrow">
          <span className="weather-cc-desc">{weather.desc}</span>
          <span className="weather-cc-hilo">H: 24°C  L: 15°C</span>
        </div>
        <div className="weather-cc-location">
          <FontAwesomeIcon icon={faLocationDot} className="weather-cc-location-icon" />
          <span>{weather.city}</span>
        </div>
        <div className="weather-cc-carousel">
          <div className="weather-cc-tabs">
            <button className={tab === 'today' ? 'active' : ''} onClick={() => setTab('today')}>Today</button>
            <button className={tab === 'week' ? 'active' : ''} onClick={() => setTab('week')}>Week</button>
          </div>
          <div className="weather-cc-carousel-rowwrap">
            <button className="weather-cc-arrow left" onClick={scrollLeftBtn} tabIndex={-1} aria-label="Scroll left">
              <FontAwesomeIcon icon={faChevronLeft} />
            </button>
            <div
              className="weather-cc-forecast-row scrollable"
              ref={forecastRowRef}
              onMouseDown={handleMouseDown}
              onMouseLeave={handleMouseLeave}
              onMouseUp={handleMouseUp}
              onMouseMove={handleMouseMove}
            >
              {(tab === 'today' ? daily : weekly).map((f, i) => (
                <div className="weather-cc-forecast-item" key={i}>
                  <span className="weather-cc-forecast-label">{tab === 'today' ? `${f.hour}:00` : f.day}</span>
                  <FontAwesomeIcon icon={f.icon} className="weather-cc-forecast-icon" />
                  <span className="weather-cc-forecast-temp">{f.temp}&deg;</span>
                </div>
              ))}
            </div>
            <button className="weather-cc-arrow right" onClick={scrollRightBtn} tabIndex={-1} aria-label="Scroll right">
              <FontAwesomeIcon icon={faChevronRight} />
            </button>
          </div>
        </div>
      </div>
    </GlassCard>
  );
}
