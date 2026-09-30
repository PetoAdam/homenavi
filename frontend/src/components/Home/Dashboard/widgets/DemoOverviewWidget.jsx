import React from 'react';
import { Link } from 'react-router-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faCompass,
  faMapLocationDot,
  faCircleInfo,
  faWandMagicSparkles,
} from '@fortawesome/free-solid-svg-icons';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import './DemoOverviewWidget.css';

const DEMO_LINKS = [
  {
    to: '/map',
    label: 'Map',
    eyebrow: 'Layout',
    description: 'Live apartment',
    icon: faMapLocationDot,
  },
  {
    to: '/automation',
    label: 'Scenes',
    eyebrow: 'Actions',
    description: 'Run presets',
    icon: faWandMagicSparkles,
  },
  {
    to: '/about',
    label: 'About',
    eyebrow: 'Guide',
    description: 'Product tour',
    icon: faCircleInfo,
  },
];

export default function DemoOverviewWidget({ editMode, onRemove }) {
  return (
    <WidgetShell
      className="demo-overview-widget"
      editMode={editMode}
      onRemove={onRemove}
      interactive={false}
      showHeader={false}
    >
      <div className="demo-overview-widget__content">
        <div className="demo-overview-widget__hero">
          <div className="demo-overview-widget__hero-top">
            <div className="demo-overview-widget__badge">
              <FontAwesomeIcon icon={faCompass} />
              <span>Start here</span>
            </div>
            <div className="demo-overview-widget__kicker">Demo home</div>
          </div>
          <div className="demo-overview-widget__hero-copy">
            <div className="demo-overview-widget__headline">Explore the seeded smart home.</div>
            <div className="demo-overview-widget__body">
              Live rooms, device state, scenes, and demo-safe controls are already wired in.
            </div>
          </div>
        </div>

        <div className="demo-overview-widget__actions" aria-label="Homenavi overview links">
          {DEMO_LINKS.map((item) => (
            <Link key={item.to} to={item.to} className="demo-overview-widget__link">
              <div className="demo-overview-widget__link-icon">
                <FontAwesomeIcon icon={item.icon} />
              </div>
              <div className="demo-overview-widget__link-copy">
                <span className="demo-overview-widget__link-eyebrow">{item.eyebrow}</span>
                <span className="demo-overview-widget__link-title">{item.label}</span>
                <span className="demo-overview-widget__link-description">{item.description}</span>
              </div>
            </Link>
          ))}
        </div>
      </div>
    </WidgetShell>
  );
}

DemoOverviewWidget.defaultHeight = 3;