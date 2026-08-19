import { ArrowUpRight, Boxes, Fingerprint, KeyRound, Timer, TrendingUp } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api';
import { StatusBadge } from '../components/UI';
import type { DashboardSnapshot } from '../types';
import { formatDate } from '../utils';

export function DashboardPage() {
  const [data, setData] = useState<DashboardSnapshot>();
  useEffect(() => { void api.dashboard().then(setData); }, []);
  if (!data) return <div className="page-loading">Loading control plane…</div>;
  const max = Math.max(...data.activationSeries);
  const stats = [
    ['Products', data.products, 'Configured applications', Boxes, 'violet'],
    ['Active licenses', data.activeLicenses, '+18 this month', KeyRound, 'green'],
    ['Bound devices', data.activeDevices, '98.7% healthy', Fingerprint, 'blue'],
    ['Expiring soon', data.expiringSoon, 'Next 30 days', Timer, 'amber'],
  ] as const;
  return (
    <div className="page-grid">
      <section className="stats-grid">
        {stats.map(([label, value, note, Icon, tone]) => <article className="stat-card" key={label}><div className={`stat-icon ${tone}`}><Icon /></div><div><span>{label}</span><strong>{value}</strong><small>{note}</small></div></article>)}
      </section>
      <section className="panel chart-panel">
        <div className="panel-head"><div><h2>Activation volume</h2><p>Successful activations over the last 7 days</p></div><span className="trend"><TrendingUp size={15} /> 12.4%</span></div>
        <div className="chart" aria-label="Weekly activation chart">
          {data.activationSeries.map((value, index) => <div className="chart-column" key={index}><div className="bar" style={{ height: `${Math.max(12, value / max * 100)}%` }}><span>{value}</span></div><small>{['Thu', 'Fri', 'Sat', 'Sun', 'Mon', 'Tue', 'Wed'][index]}</small></div>)}
        </div>
      </section>
      <section className="panel events-panel">
        <div className="panel-head"><div><h2>Recent activity</h2><p>Live control-plane events</p></div><Link to="/audit" className="text-link">View all <ArrowUpRight size={14} /></Link></div>
        <div className="event-list">
          {data.recentEvents.map((event) => <article key={event.id}><span className={`event-dot ${event.severity}`} /><div><strong>{event.action}</strong><p>{event.target}</p></div><div className="event-meta"><StatusBadge value={event.severity} /><small>{formatDate(event.at, true)}</small></div></article>)}
        </div>
      </section>
    </div>
  );
}
