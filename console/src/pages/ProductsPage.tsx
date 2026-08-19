import { Boxes, Download, Plus, Shield, Timer } from 'lucide-react';
import { type FormEvent, useEffect, useState } from 'react';
import { api } from '../api';
import { Button, Modal, Toast } from '../components/UI';
import type { Product } from '../types';
import { formatDate } from '../utils';

export function ProductsPage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  useEffect(() => { void api.listProducts().then(setProducts); }, []);
  const create = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); setBusy(true);
    const form = new FormData(event.currentTarget);
    const product = await api.createProduct({ name: String(form.get('name')), code: String(form.get('code')).toUpperCase(), maxDevices: Number(form.get('maxDevices')), offlineHours: Number(form.get('offlineHours')) });
    setProducts((items) => [product, ...items]); setBusy(false); setOpen(false); setNotice(`${product.name} created`);
  };
  const exportKit = async (product: Product) => {
    try { const result = await api.exportIntegrationKit(product.id); setNotice(`${result.filename} is ready`); }
    catch (error) { setNotice(error instanceof Error ? error.message : 'Export failed'); }
  };
  return <>
    <div className="actions-row"><div className="summary-pill"><Boxes size={16} /><strong>{products.length}</strong> products configured</div><Button onClick={() => setOpen(true)}><Plus size={17} /> New product</Button></div>
    <section className="card-grid">
      {products.map((product) => <article className="product-card" key={product.id}>
        <div className="product-top"><div className="product-avatar">{product.code.slice(0, 2)}</div><span className="product-code">{product.code}</span></div>
        <h2>{product.name}</h2><p>Created {formatDate(product.createdAt)}</p>
        <div className="policy-grid"><span><Shield size={15} /><div><strong>{product.maxDevices}</strong><small>device{product.maxDevices > 1 ? 's' : ''} / license</small></div></span><span><Timer size={15} /><div><strong>{product.offlineHours}h</strong><small>offline lease</small></div></span></div>
        <div className="product-footer"><span><strong>{product.activeLicenses}</strong> active licenses</span><Button variant="secondary" onClick={() => void exportKit(product)}><Download size={15} /> Export kit</Button></div>
      </article>)}
    </section>
    {open && <Modal title="Create a product" description="Define the default device and offline policy." onClose={() => setOpen(false)}><form onSubmit={(event) => void create(event)}>
      <div className="form-grid"><label className="wide">Product name<input required name="name" placeholder="e.g. Atlas Studio" autoFocus /></label><label>Product code<input required name="code" placeholder="ATLAS" pattern="[A-Za-z0-9_-]{2,16}" /></label><label>Devices per license<select name="maxDevices" defaultValue="1"><option value="1">1 device</option><option value="2">2 devices</option><option value="3">3 devices</option></select></label><label className="wide">Offline lease<select name="offlineHours" defaultValue="72"><option value="24">24 hours</option><option value="48">48 hours</option><option value="72">72 hours (recommended)</option><option value="168">7 days</option></select><small>Revocation takes effect no later than the end of this window.</small></label></div>
      <div className="modal-actions"><Button type="button" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button><Button type="submit" busy={busy}>Create product</Button></div>
    </form></Modal>}
    {notice && <Toast>{notice}</Toast>}
  </>;
}
