import React from 'react';
import { createRoot } from 'react-dom/client';
import '../shared/hn.css';
import { fetchCounter, incrementCounter } from '../shared/api';

function TabApp() {
  const [count, setCount] = React.useState(0);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    let alive = true;
    fetchCounter()
      .then((data) => { if (alive) setCount(data?.count ?? 0); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, []);

  return (
    <div className="hn-shell">
      <div className="hn-card">
        <div className="hn-row">
          <div>
            <h1 className="hn-title">Hello Homenavi</h1>
            <div className="hn-subtitle">Example integration tab using the shared UI style.</div>
          </div>
          <span className="hn-pill">integration</span>
        </div>

        <div className="hn-section">
          <div style={{ fontWeight: 600 }}>Counter demo</div>
          <div className="hn-subtitle">State is stored in memory for now.</div>
          <div className="hn-counter">
            <div className="hn-stat">
              <div className="hn-stat-label">Clicked</div>
              <div className="hn-stat-value">{loading ? '…' : count}</div>
            </div>
            <button
              className="hn-btn"
              onClick={async () => {
                const data = await incrementCounter(1);
                setCount(data?.count ?? count + 1);
              }}
            >
              Increase
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

createRoot(document.getElementById('root')).render(<TabApp />);
