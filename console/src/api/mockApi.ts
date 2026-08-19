import type {
  AuditEvent,
  Backup,
  ConsoleApi,
  CreateProductInput,
  Device,
  GenerateLicenseInput,
  License,
  Product,
  SigningKey,
} from '../types';

const delay = <T,>(value: T, ms = 120) => new Promise<T>((resolve) => setTimeout(() => resolve(value), ms));
const now = () => new Date().toISOString();
const id = (prefix: string) => `${prefix}_${Math.random().toString(36).slice(2, 9)}`;

const products: Product[] = [
  { id: 'prd_atlas', name: 'Atlas Studio', code: 'ATLAS', maxDevices: 1, offlineHours: 72, activeLicenses: 126, createdAt: '2026-07-04T08:00:00Z' },
  { id: 'prd_nova', name: 'Nova Automator', code: 'NOVA', maxDevices: 1, offlineHours: 72, activeLicenses: 84, createdAt: '2026-07-21T08:00:00Z' },
  { id: 'prd_orbit', name: 'Orbit Toolkit', code: 'ORBIT', maxDevices: 2, offlineHours: 48, activeLicenses: 37, createdAt: '2026-08-03T08:00:00Z' },
];

const licenses: License[] = [
  { id: 'lic_1', key: 'ATLAS-9F2K-••••-8XDQ', productId: 'prd_atlas', productName: 'Atlas Studio', customer: 'Minh Hoàng', status: 'active', expiresAt: '2027-08-14T00:00:00Z', devices: 1, maxDevices: 1, entitlements: ['pro', 'export'] },
  { id: 'lic_2', key: 'NOVA-41QZ-••••-WM7P', productId: 'prd_nova', productName: 'Nova Automator', customer: 'Thanh Vũ', status: 'active', expiresAt: '2026-10-01T00:00:00Z', devices: 1, maxDevices: 1, entitlements: ['pro'] },
  { id: 'lic_3', key: 'ORBIT-K8J4-••••-P2ME', productId: 'prd_orbit', productName: 'Orbit Toolkit', customer: 'Studio K', status: 'suspended', expiresAt: null, devices: 1, maxDevices: 2, entitlements: ['basic'] },
  { id: 'lic_4', key: 'ATLAS-6LWA-••••-2YMN', productId: 'prd_atlas', productName: 'Atlas Studio', customer: 'Quốc Anh', status: 'expired', expiresAt: '2026-08-08T00:00:00Z', devices: 0, maxDevices: 1, entitlements: ['pro'] },
];

const devices: Device[] = [
  { id: 'dev_1', name: 'DESKTOP-Q7V2', fingerprint: '7d91…9af2', licenseKey: licenses[0].key, productName: 'Atlas Studio', activatedAt: '2026-08-14T03:20:00Z', lastSeenAt: '2026-08-19T06:42:00Z', state: 'online' },
  { id: 'dev_2', name: 'WORKSTATION-05', fingerprint: '3aa2…18bc', licenseKey: licenses[1].key, productName: 'Nova Automator', activatedAt: '2026-08-10T09:11:00Z', lastSeenAt: '2026-08-19T06:38:00Z', state: 'online' },
  { id: 'dev_3', name: 'MINH-PC', fingerprint: '10f3…c775', licenseKey: licenses[2].key, productName: 'Orbit Toolkit', activatedAt: '2026-07-28T02:14:00Z', lastSeenAt: '2026-08-17T11:04:00Z', state: 'offline' },
];

const signingKeys: SigningKey[] = [
  { kid: 'lh_2026_08', algorithm: 'Ed25519', state: 'active', createdAt: '2026-08-01T00:00:00Z' },
  { kid: 'lh_2026_02', algorithm: 'Ed25519', state: 'retiring', createdAt: '2026-02-01T00:00:00Z' },
];

const backups: Backup[] = [
  { id: 'bkp_0819', createdAt: '2026-08-19T02:00:00Z', size: '24.8 MB', status: 'verified' },
  { id: 'bkp_0818', createdAt: '2026-08-18T02:00:00Z', size: '24.5 MB', status: 'verified' },
  { id: 'bkp_0817', createdAt: '2026-08-17T02:00:00Z', size: '24.2 MB', status: 'verified' },
];

const audit: AuditEvent[] = [
  { id: 'evt_1', action: 'Device activated', actor: 'client', target: 'DESKTOP-Q7V2 · Atlas Studio', at: '2026-08-19T06:42:00Z', severity: 'success' },
  { id: 'evt_2', action: 'License generated', actor: 'admin@licensehub', target: 'Nova Automator · Thanh Vũ', at: '2026-08-19T05:18:00Z', severity: 'info' },
  { id: 'evt_3', action: 'Activation rejected', actor: 'client', target: 'ATLAS-9F2K · device slot full', at: '2026-08-19T04:51:00Z', severity: 'warning' },
  { id: 'evt_4', action: 'Backup verified', actor: 'system', target: 'bkp_0819', at: '2026-08-19T02:01:00Z', severity: 'success' },
  { id: 'evt_5', action: 'License suspended', actor: 'admin@licensehub', target: 'Orbit Toolkit · Studio K', at: '2026-08-18T11:24:00Z', severity: 'warning' },
];

const addAudit = (action: string, target: string, severity: AuditEvent['severity'] = 'info') => {
  audit.unshift({ id: id('evt'), action, actor: 'admin@licensehub', target, at: now(), severity });
};

export const mockApi: ConsoleApi = {
  async dashboard() {
    return delay({ products: products.length, activeLicenses: 247, activeDevices: devices.length, expiringSoon: 12, activationSeries: [18, 25, 21, 34, 29, 42, 38], recentEvents: audit.slice(0, 4) });
  },
  async listProducts() { return delay([...products]); },
  async createProduct(input: CreateProductInput) {
    const product: Product = { id: id('prd'), ...input, activeLicenses: 0, createdAt: now() };
    products.unshift(product);
    addAudit('Product created', product.name, 'success');
    return delay(product);
  },
  async exportIntegrationKit(productId: string) {
    const product = products.find((item) => item.id === productId);
    if (!product) throw new Error('Product not found');
    addAudit('Integration kit exported', product.name);
    return delay({ filename: `${product.code.toLowerCase()}-license-kit.zip` });
  },
  async listLicenses() { return delay([...licenses]); },
  async generateLicense(input: GenerateLicenseInput) {
    const product = products.find((item) => item.id === input.productId);
    if (!product) throw new Error('Product not found');
    const segments = Array.from({ length: 3 }, () => Math.random().toString(36).slice(2, 6).toUpperCase());
    const license: License = {
      id: id('lic'), key: `${product.code}-${segments.join('-')}`, productId: product.id, productName: product.name,
      customer: input.customer, status: 'active', expiresAt: input.durationDays ? new Date(Date.now() + input.durationDays * 86400000).toISOString() : null,
      devices: 0, maxDevices: product.maxDevices, entitlements: input.entitlements,
    };
    licenses.unshift(license);
    product.activeLicenses += 1;
    addAudit('License generated', `${product.name} · ${input.customer}`, 'success');
    return delay(license);
  },
  async revokeLicense(licenseId: string) {
    const license = licenses.find((item) => item.id === licenseId);
    if (!license) throw new Error('License not found');
    license.status = 'revoked';
    addAudit('License revoked', `${license.productName} · ${license.customer}`, 'warning');
    await delay(undefined);
  },
  async listDevices() { return delay([...devices]); },
  async resetDevice(deviceId: string) {
    const index = devices.findIndex((item) => item.id === deviceId);
    if (index < 0) throw new Error('Device not found');
    const [device] = devices.splice(index, 1);
    const license = licenses.find((item) => item.key === device.licenseKey);
    if (license) license.devices = Math.max(0, license.devices - 1);
    addAudit('Device reset', `${device.name} · ${device.productName}`, 'warning');
    await delay(undefined);
  },
  async listSigningKeys() { return delay([...signingKeys]); },
  async rotateSigningKey() {
    signingKeys.forEach((key) => { if (key.state === 'active') key.state = 'retiring'; });
    const key: SigningKey = { kid: `lh_${new Date().toISOString().slice(0, 10).replaceAll('-', '_')}`, algorithm: 'Ed25519', state: 'active', createdAt: now() };
    signingKeys.unshift(key);
    addAudit('Signing key rotated', key.kid, 'success');
    return delay(key);
  },
  async listBackups() { return delay([...backups]); },
  async createBackup() {
    const backup: Backup = { id: id('bkp'), createdAt: now(), size: '24.9 MB', status: 'verified' };
    backups.unshift(backup);
    addAudit('Backup verified', backup.id, 'success');
    return delay(backup, 400);
  },
  async listAuditEvents() { return delay([...audit]); },
};
