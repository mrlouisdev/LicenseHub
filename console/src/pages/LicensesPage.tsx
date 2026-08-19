import { KeyRound, Plus, Search, ShieldOff } from 'lucide-react';
import { type FormEvent, useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import { Button, Modal, StatusBadge, Toast } from '../components/UI';
import type { License, Product } from '../types';
import { formatDate } from '../utils';

export function LicensesPage() {
  const [licenses, setLicenses] = useState<License[]>([]); const [products, setProducts] = useState<Product[]>([]);
  const [open, setOpen] = useState(false); const [busy, setBusy] = useState(false); const [query, setQuery] = useState(''); const [notice, setNotice] = useState(''); const [generated, setGenerated] = useState<License>();
  useEffect(() => { void Promise.all([api.listLicenses(), api.listProducts()]).then(([licenseItems, productItems]) => { setLicenses(licenseItems); setProducts(productItems); }); }, []);
  const visible = useMemo(() => licenses.filter((item) => `${item.key} ${item.customer} ${item.productName}`.toLowerCase().includes(query.toLowerCase())), [licenses, query]);
  const generate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); setBusy(true); const form = new FormData(event.currentTarget); const duration = String(form.get('duration'));
    try {
      const item = await api.generateLicense({ productId: String(form.get('productId')), customer: String(form.get('customer')), durationDays: duration === 'never' ? null : Number(duration), entitlements: String(form.get('entitlements')).split(',').map((value) => value.trim()).filter(Boolean) });
      setLicenses((items) => [item, ...items]); setGenerated(item);
    } catch (error) { setNotice(error instanceof Error ? error.message : 'License generation failed'); }
    finally { setBusy(false); }
  };
  const revoke = async (item: License) => { if (!window.confirm(`Revoke license for ${item.customer}?`)) return; await api.revokeLicense(item.id); setLicenses((items) => items.map((license) => license.id === item.id ? { ...license, status: 'revoked' } : license)); setNotice('License revoked'); };
  const close = () => { setOpen(false); setGenerated(undefined); };
  return <>
    <div className="actions-row"><label className="search"><Search size={16} /><input aria-label="Search licenses" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search key, customer, or product…" /></label><Button onClick={() => setOpen(true)}><Plus size={17} /> Generate license</Button></div>
    <section className="panel table-panel"><div className="table-wrap"><table><thead><tr><th>License</th><th>Product</th><th>Customer</th><th>Status</th><th>Devices</th><th>Expires</th><th /></tr></thead><tbody>{visible.map((item) => <tr key={item.id}><td><code>{item.key}</code><small>{item.entitlements.join(' · ')}</small></td><td>{item.productName}</td><td>{item.customer}</td><td><StatusBadge value={item.status} /></td><td>{item.devices} / {item.maxDevices}</td><td>{formatDate(item.expiresAt)}</td><td><Button variant="ghost" disabled={item.status === 'revoked'} onClick={() => void revoke(item)}><ShieldOff size={15} /> Revoke</Button></td></tr>)}</tbody></table></div></section>
    {open && <Modal title={generated ? 'License generated' : 'Generate a license'} description={generated ? 'Copy this key now and deliver it securely.' : 'Issue access under a product policy.'} onClose={close}>{generated ? <div className="generated"><KeyRound /><code>{generated.key}</code><p>{generated.customer} · {generated.productName}</p><Button onClick={() => void navigator.clipboard?.writeText(generated.key)}>Copy key</Button></div> : <form onSubmit={(event) => void generate(event)}><div className="form-grid"><label className="wide">Product<select required name="productId" defaultValue=""><option value="" disabled>Select product</option>{products.map((product) => <option key={product.id} value={product.id}>{product.name} · {product.maxDevices} device · {product.offlineHours}h</option>)}</select></label><label className="wide">Customer / note<input required name="customer" placeholder="Customer name" /></label><label>Duration<select name="duration" defaultValue="365"><option value="30">30 days</option><option value="365">1 year</option><option value="never">Lifetime</option></select></label><label>Entitlements<input name="entitlements" defaultValue="pro, export" placeholder="pro, export" /></label></div><div className="modal-actions"><Button type="button" variant="ghost" onClick={close}>Cancel</Button><Button type="submit" busy={busy}>Generate license</Button></div></form>}</Modal>}
    {notice && <Toast>{notice}</Toast>}
  </>;
}
