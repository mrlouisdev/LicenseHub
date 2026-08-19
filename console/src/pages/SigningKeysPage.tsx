import { KeyRound, RefreshCw, ShieldCheck, TriangleAlert } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api } from '../api';
import { Button, StatusBadge, Toast } from '../components/UI';
import type { Product, SigningKey } from '../types';
import { formatDate } from '../utils';

export function SigningKeysPage() {
  const [keys, setKeys] = useState<SigningKey[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  const [productId, setProductId] = useState('');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');

  useEffect(() => {
    void api.listProducts().then((items) => {
      setProducts(items);
      setProductId(items[0]?.id ?? '');
    });
  }, []);

  useEffect(() => {
    if (!productId) return;
    void api.listSigningKeys(productId).then(setKeys).catch((error: unknown) => {
      setKeys([]);
      setNotice(error instanceof Error ? error.message : 'Unable to load keys');
    });
  }, [productId]);

  const rotate = async () => {
    if (!window.confirm('Rotate the signing key? Existing public keys stay trusted during retirement.')) return;
    setBusy(true);
    try {
      const key = await api.rotateSigningKey(productId);
      setKeys((items) => [key, ...items.map((item) => ({ ...item, state: 'retiring' as const }))]);
      setNotice(`New signing key ${key.kid} is active`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Rotation failed');
    } finally {
      setBusy(false);
    }
  };

  return <div className="two-column">
    <section className="panel">
      <div className="panel-head">
        <div><h2>Key ring</h2><p>Per-product Ed25519 release-signing keys</p></div>
        <div className="key-actions">
          <select aria-label="Signing-key product" value={productId} onChange={(event) => setProductId(event.target.value)}>
            {products.map((product) => <option key={product.id} value={product.id}>{product.name}</option>)}
          </select>
          <Button disabled={!productId} onClick={() => void rotate()} busy={busy}><RefreshCw size={16} /> Rotate key</Button>
        </div>
      </div>
      <div className="key-list">{keys.map((key) => <article key={key.kid}>
        <span className={`key-icon ${key.state}`}><KeyRound /></span>
        <div><code>{key.kid}</code><p>{key.algorithm} · Created {formatDate(key.createdAt)}</p></div>
        <StatusBadge value={key.state} />
      </article>)}</div>
    </section>
    <aside className="panel info-panel">
      <ShieldCheck size={30} /><h2>Trust chain protected</h2>
      <p>Private signing material remains on the VPS. Applications receive only public material.</p>
      <div className="info-note"><TriangleAlert size={17} /><span>Server signing keys are scoped per product. Keep retiring keys through the compatibility window.</span></div>
      <dl><div><dt>Algorithm</dt><dd>Ed25519</dd></div><div><dt>Selected product</dt><dd>{products.find((item) => item.id === productId)?.name ?? '—'}</dd></div><div><dt>Active key</dt><dd>{keys.find((key) => key.state === 'active')?.kid ?? '—'}</dd></div></dl>
    </aside>
    {notice && <Toast>{notice}</Toast>}
  </div>;
}
