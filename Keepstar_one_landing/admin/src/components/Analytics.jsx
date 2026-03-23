import React, { useState, useEffect } from 'react';
import { api } from '../api';

export default function Analytics() {
  const [data, setData] = useState(null);

  useEffect(() => {
    api.getAnalytics().then(setData).catch(console.error);
  }, []);

  if (!data) return <div className="admin-loading">Loading analytics...</div>;

  const maxViews = Math.max(...(data.viewsByDay.map((d) => d.count)), 1);

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">Analytics</h1>
      </div>

      <div className="stats-grid">
        <div className="stat-card-admin">
          <div className="stat-value">{data.totalPosts}</div>
          <div className="stat-label-admin">Total Posts</div>
        </div>
        <div className="stat-card-admin">
          <div className="stat-value">{data.published}</div>
          <div className="stat-label-admin">Published</div>
        </div>
        <div className="stat-card-admin">
          <div className="stat-value">{data.totalViews}</div>
          <div className="stat-label-admin">Total Views</div>
        </div>
        <div className="stat-card-admin">
          <div className="stat-value">{data.viewsToday}</div>
          <div className="stat-label-admin">Views Today</div>
        </div>
      </div>

      <div className="analytics-section">
        <h3>Views (Last 30 Days)</h3>
        <div className="chart-placeholder">
          {data.viewsByDay.length === 0 ? (
            <p style={{ color: '#5F6368', margin: 'auto', fontSize: 14 }}>No data yet</p>
          ) : (
            data.viewsByDay.map((d, i) => (
              <div key={i} className="chart-bar" style={{ height: `${(d.count / maxViews) * 100}%` }} title={`${d.day}: ${d.count} views`} />
            ))
          )}
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 24 }}>
        <div className="analytics-section">
          <h3>Top Posts</h3>
          <div className="analytics-list">
            {data.topPosts.length === 0 ? (
              <p style={{ color: '#5F6368', fontSize: 14, padding: '16px 0' }}>No posts yet</p>
            ) : (
              data.topPosts.map((p, i) => (
                <div key={i} className="analytics-list-item">
                  <span>{p.title || p.slug}</span>
                  <span className="item-value">{p.views}</span>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="analytics-section">
          <h3>Top Referrers</h3>
          <div className="analytics-list">
            {data.topReferrers.length === 0 ? (
              <p style={{ color: '#5F6368', fontSize: 14, padding: '16px 0' }}>No referrer data yet</p>
            ) : (
              data.topReferrers.map((r, i) => (
                <div key={i} className="analytics-list-item">
                  <span>{r.referrer}</span>
                  <span className="item-value">{r.count}</span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </>
  );
}
