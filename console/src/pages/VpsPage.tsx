import { ArchiveRestore, CheckCircle2, CloudCog, Database, Download, Gauge, HardDrive, Plus, Server } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api } from '../api';
import { Button, StatusBadge, Toast } from '../components/UI';
import type { Backup } from '../types';
import { formatDate } from '../utils';

export function VpsPage() {
  const [backups, setBackups] = useState<Backup[]>([]);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [backupSupported, setBackupSupported] = useState(true);

  useEffect(() => {
    void api.listBackups().then(setBackups).catch((error: unknown) => {
      setBackupSupported(false);
      setNotice(error instanceof Error ? error.message : 'Backup API unavailable');
    });
  }, []);

  const backup = async () => {
    setBusy(true);
    try {
      const item = await api.createBackup();
      setBackups((items) => [item, ...items]);
      setNotice(`Backup ${item.id} verified`);
    } catch (error) {
      setBackupSupported(false);
      setNotice(error instanceof Error ? error.message : 'Backup failed');
    } finally {
      setBusy(false);
    }
  };

  return <div className="page-grid">
    <section className="infra-grid">
      <article><span className="infra-icon green"><Server /></span><div><small>Server</small><strong>Operational</strong><p>LicenseHub API</p></div></article>
      <article><span className="infra-icon blue"><Database /></span><div><small>PostgreSQL</small><strong>Connected</strong><p>Runtime health</p></div></article>
      <article><span className="infra-icon violet"><Gauge /></span><div><small>Admin API</small><strong>Ready</strong><p>Session protected</p></div></article>
      <article><span className="infra-icon amber"><HardDrive /></span><div><small>Backups</small><strong>{backupSupported ? 'Available' : 'External'}</strong><p>{backupSupported ? 'Managed by API' : 'Use deploy tooling'}</p></div></article>
    </section>
    <section className="panel table-panel">
      <div className="panel-head"><div><h2>Recovery points</h2><p>{backupSupported ? 'Encrypted database and server-key bundles' : 'The current server has no backup-management endpoint'}</p></div><Button disabled={!backupSupported} onClick={() => void backup()} busy={busy}><Plus size={16} /> Backup now</Button></div>
      <div className="table-wrap"><table><thead><tr><th>Backup ID</th><th>Created</th><th>Size</th><th>Integrity</th><th /></tr></thead><tbody>{backups.map((item) => <tr key={item.id}><td><code>{item.id}</code></td><td>{formatDate(item.createdAt, true)}</td><td>{item.size}</td><td><StatusBadge value={item.status} /></td><td><Button variant="ghost"><Download size={15} /> Download</Button></td></tr>)}</tbody></table></div>
    </section>
    <section className="migration-card"><div className="migration-art"><CloudCog /><span /><CheckCircle2 /></div><div><p className="eyebrow">DISASTER RECOVERY</p><h2>Ready to migrate VPS</h2><p>Domain-based discovery, database snapshots, and encrypted signing material keep client applications online during a move.</p></div><Button variant="secondary"><ArchiveRestore size={16} /> Open migration guide</Button></section>
    {notice && <Toast>{notice}</Toast>}
  </div>;
}
