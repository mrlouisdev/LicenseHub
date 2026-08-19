import { Fingerprint, RotateCcw, Search } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import { Button, Empty, StatusBadge, Toast } from '../components/UI';
import type { Device } from '../types';
import { formatDate } from '../utils';

export function DevicesPage() {
  const [devices, setDevices] = useState<Device[]>([]); const [query, setQuery] = useState(''); const [notice, setNotice] = useState('');
  useEffect(() => { void api.listDevices().then(setDevices); }, []);
  const visible = useMemo(() => devices.filter((item) => `${item.name} ${item.productName} ${item.fingerprint}`.toLowerCase().includes(query.toLowerCase())), [devices, query]);
  const reset = async (item: Device) => { if (!window.confirm(`Reset ${item.name}? This frees its activation slot.`)) return; await api.resetDevice(item.id); setDevices((items) => items.filter((device) => device.id !== item.id)); setNotice(`${item.name} binding reset`); };
  return <><div className="actions-row"><label className="search"><Search size={16} /><input aria-label="Search devices" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search machine, product, fingerprint…" /></label><div className="summary-pill"><Fingerprint size={16} /><strong>{devices.length}</strong> device bindings</div></div>
    <section className="panel table-panel"><div className="table-wrap"><table><thead><tr><th>Machine</th><th>Product</th><th>Fingerprint</th><th>State</th><th>Activated</th><th>Last seen</th><th /></tr></thead><tbody>{visible.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small>{item.licenseKey}</small></td><td>{item.productName}</td><td><code>{item.fingerprint}</code></td><td><StatusBadge value={item.state} /></td><td>{formatDate(item.activatedAt)}</td><td>{formatDate(item.lastSeenAt, true)}</td><td><Button variant="ghost" onClick={() => void reset(item)}><RotateCcw size={15} /> Reset</Button></td></tr>)}</tbody></table>{visible.length === 0 && <Empty icon={<Fingerprint />} title="No matching devices" text="Try a different search term." />}</div></section>{notice && <Toast>{notice}</Toast>}</>;
}
