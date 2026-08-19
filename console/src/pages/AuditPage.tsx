import { Download, Filter, ScrollText, Search } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import { Button, Empty, StatusBadge } from '../components/UI';
import type { AuditEvent } from '../types';
import { formatDate } from '../utils';

export function AuditPage() {
  const [events, setEvents] = useState<AuditEvent[]>([]); const [query, setQuery] = useState(''); const [filter, setFilter] = useState('all');
  useEffect(() => { void api.listAuditEvents().then(setEvents); }, []);
  const visible = useMemo(() => events.filter((item) => (filter === 'all' || item.severity === filter) && `${item.action} ${item.target} ${item.actor}`.toLowerCase().includes(query.toLowerCase())), [events, query, filter]);
  return <><div className="actions-row"><label className="search"><Search size={16} /><input aria-label="Search audit logs" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search action, actor, or target…" /></label><label className="select-inline"><Filter size={15} /><select aria-label="Filter severity" value={filter} onChange={(event) => setFilter(event.target.value)}><option value="all">All events</option><option value="success">Success</option><option value="warning">Warning</option><option value="info">Info</option></select></label><Button variant="secondary"><Download size={15} /> Export CSV</Button></div><section className="panel audit-timeline">{visible.map((event) => <article key={event.id}><div className={`timeline-mark ${event.severity}`}><span /></div><div><div className="audit-title"><strong>{event.action}</strong><StatusBadge value={event.severity} /></div><p>{event.target}</p><small>{event.actor} · {formatDate(event.at, true)}</small></div></article>)}{visible.length === 0 && <Empty icon={<ScrollText />} title="No matching activity" text="Adjust the search or event filter." />}</section></>;
}
