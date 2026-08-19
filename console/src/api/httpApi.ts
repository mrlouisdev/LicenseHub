import { runtimeFetch } from './transport';
import type {
  AuditEvent, Backup, ConsoleApi, CreateProductInput, DashboardSnapshot, Device,
  GenerateLicenseInput, License, LicenseStatus, Product, SigningKey,
} from '../types';

interface Envelope<T> { success: boolean; data?: T; error?: { code: string; message: string; details?: unknown } }
interface ServerProduct { id: string; name: string; slug: string; type: string; created_at: string }
interface ServerPlan { id: string; product_id: string; name: string; slug: string; max_activations: number; active: boolean; entitlements?: Array<{ feature: string }> }
interface ServerActivation { id: string; license_id: string; identifier: string; label?: string; ip_address?: string; last_verified: string; created_at: string }
interface ServerLicense {
  id: string; product_id: string; plan_id: string; email: string; license_key: string; status: string;
  valid_until?: string; created_at: string; product?: ServerProduct; plan?: ServerPlan; activations?: ServerActivation[];
}
interface ServerSigningKey { id: string; product_id: string; active: boolean; created_at: string; rotated_at?: string }
interface ServerAudit { id: string; entity: string; entity_id: string; action: string; actor_id?: string; actor_type?: string; ip_address?: string; created_at: string }
interface ServerStats { total_products: number; active_licenses: number; total_activations: number; by_status: Record<string, number> }

export class ConsoleApiError extends Error {
  constructor(message: string, public readonly code: string, public readonly status: number) { super(message); this.name = 'ConsoleApiError'; }
}

export class UnsupportedConsoleOperationError extends ConsoleApiError {
  constructor(operation: string) { super(`${operation} is not exposed by the LicenseHub server API`, 'UNSUPPORTED_OPERATION', 501); this.name = 'UnsupportedConsoleOperationError'; }
}

const normalizeBaseUrl = (value: string) => value.replace(/\/+$/, '');
const statusOf = (value: string): LicenseStatus => {
  if (value === 'active' || value === 'suspended' || value === 'revoked' || value === 'expired') return value;
  if (value === 'trialing') return 'active';
  if (value === 'past_due') return 'suspended';
  return 'revoked';
};

export class HttpConsoleApi implements ConsoleApi {
  private readonly adminRoot: string;
  constructor(baseUrl: string) {
    if (!baseUrl.trim()) throw new Error('LicenseHub API URL is required in HTTP mode');
    this.adminRoot = `${normalizeBaseUrl(baseUrl)}/api/v1/admin`;
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set('Accept', 'application/json');
    if (init.body) headers.set('Content-Type', 'application/json');
    const response = await runtimeFetch(`${this.adminRoot}${path}`, { credentials: 'include', ...init, headers });
    if (response.status === 204) return undefined as T;
    let envelope: Envelope<T>;
    try { envelope = await response.json() as Envelope<T>; }
    catch { throw new ConsoleApiError(`Server returned invalid JSON (${response.status})`, 'INVALID_RESPONSE', response.status); }
    if (!response.ok || !envelope.success || envelope.data === undefined) {
      throw new ConsoleApiError(envelope.error?.message ?? `Request failed (${response.status})`, envelope.error?.code ?? 'HTTP_ERROR', response.status);
    }
    return envelope.data;
  }

  private mapProduct(item: ServerProduct, plans: ServerPlan[] = []): Product {
    const plan = plans.find((value) => value.product_id === item.id && value.active);
    return { id: item.id, name: item.name, code: item.slug.toUpperCase(), maxDevices: plan?.max_activations ?? 1, offlineHours: 72, activeLicenses: 0, createdAt: item.created_at };
  }

  private mapLicense(item: ServerLicense): License {
    const planEntitlements = item.plan?.entitlements?.map((value) => value.feature) ?? [];
    return { id: item.id, key: item.license_key, productId: item.product_id, productName: item.product?.name ?? item.product_id, customer: item.email, status: statusOf(item.status), expiresAt: item.valid_until ?? null, devices: item.activations?.length ?? 0, maxDevices: item.plan?.max_activations ?? 1, entitlements: planEntitlements };
  }

  async dashboard(): Promise<DashboardSnapshot> {
    const [stats, events] = await Promise.all([this.request<ServerStats>('/stats'), this.listAuditEvents()]);
    return { products: stats.total_products, activeLicenses: stats.active_licenses, activeDevices: stats.total_activations, expiringSoon: stats.by_status.expired ?? 0, activationSeries: [0, 0, 0, 0, 0, 0, stats.total_activations], recentEvents: events.slice(0, 4) };
  }

  async listProducts(): Promise<Product[]> {
    const [{ products }, { plans }] = await Promise.all([
      this.request<{ products: ServerProduct[] }>('/products'),
      this.request<{ plans: ServerPlan[] }>('/plans'),
    ]);
    return products.map((item) => this.mapProduct(item, plans));
  }

  async createProduct(input: CreateProductInput): Promise<Product> {
    const product = await this.request<ServerProduct>('/products', { method: 'POST', body: JSON.stringify({ name: input.name, slug: input.code.toLowerCase(), type: 'desktop' }) });
    try {
      const plan = await this.request<ServerPlan>('/plans', { method: 'POST', body: JSON.stringify({ product_id: product.id, name: 'Standard', slug: 'standard', license_type: 'perpetual', max_activations: input.maxDevices, license_model: 'standard' }) });
      return this.mapProduct(product, [plan]);
    } catch (error) {
      throw new ConsoleApiError(`Product was created, but its default plan failed: ${error instanceof Error ? error.message : 'unknown error'}`, 'PARTIAL_PRODUCT_CREATE', 500);
    }
  }

  async exportIntegrationKit(productId: string): Promise<{ filename: string }> {
    void productId;
    throw new UnsupportedConsoleOperationError('Integration-kit export');
  }

  async listLicenses(): Promise<License[]> {
    const { licenses } = await this.request<{ licenses: ServerLicense[]; total: number }>('/licenses?limit=200');
    return licenses.map((item) => this.mapLicense(item));
  }

  async generateLicense(input: GenerateLicenseInput): Promise<License> {
    const { plans } = await this.request<{ plans: ServerPlan[] }>(`/plans?product_id=${encodeURIComponent(input.productId)}`);
    const plan = plans.find((item) => item.active);
    if (!plan) throw new ConsoleApiError('Create an active plan for this product before generating a license', 'PLAN_REQUIRED', 409);
    const validUntil = input.durationDays ? new Date(Date.now() + input.durationDays * 86400000).toISOString() : '';
    const license = await this.request<ServerLicense>('/licenses', { method: 'POST', body: JSON.stringify({ product_id: input.productId, plan_id: plan.id, email: input.customer, notes: input.entitlements.length ? `Requested features: ${input.entitlements.join(', ')}` : '', valid_until: validUntil }) });
    return this.mapLicense({ ...license, plan });
  }

  async revokeLicense(id: string): Promise<void> { await this.request<{ status: string }>(`/licenses/${encodeURIComponent(id)}/revoke`, { method: 'POST' }); }

  async listDevices(): Promise<Device[]> {
    const { licenses } = await this.request<{ licenses: ServerLicense[]; total: number }>('/licenses?limit=200');
    const details = await Promise.all(licenses.map((item) => this.request<ServerLicense>(`/licenses/${encodeURIComponent(item.id)}`)));
    return details.flatMap((license) => (license.activations ?? []).map((item) => ({ id: item.id, name: item.label || item.identifier, fingerprint: item.identifier, licenseKey: license.license_key, productName: license.product?.name ?? license.product_id, activatedAt: item.created_at, lastSeenAt: item.last_verified, state: Date.now() - new Date(item.last_verified).getTime() < 5 * 60_000 ? 'online' as const : 'offline' as const })));
  }

  async resetDevice(id: string): Promise<void> { await this.request<void>(`/activations/${encodeURIComponent(id)}`, { method: 'DELETE' }); }

  async listSigningKeys(productId?: string): Promise<SigningKey[]> {
    if (!productId) throw new ConsoleApiError('Select a product to view signing keys', 'PRODUCT_REQUIRED', 400);
    const { keys } = await this.request<{ keys: ServerSigningKey[] }>(`/products/${encodeURIComponent(productId)}/signing-keys`);
    return keys.map((item) => ({ kid: item.id, algorithm: 'Ed25519', state: item.active ? 'active' : 'retiring', createdAt: item.created_at }));
  }

  async rotateSigningKey(productId?: string): Promise<SigningKey> {
    if (!productId) throw new ConsoleApiError('Select a product before rotating its signing key', 'PRODUCT_REQUIRED', 400);
    const item = await this.request<ServerSigningKey>(`/products/${encodeURIComponent(productId)}/signing-key/rotate`, { method: 'POST', body: JSON.stringify({ note: 'Rotated from LicenseHub Console' }) });
    return { kid: item.id, algorithm: 'Ed25519', state: 'active', createdAt: item.created_at };
  }

  async listBackups(): Promise<Backup[]> { throw new UnsupportedConsoleOperationError('Backup listing'); }
  async createBackup(): Promise<Backup> { throw new UnsupportedConsoleOperationError('Server backup'); }

  async listAuditEvents(): Promise<AuditEvent[]> {
    const { audit_logs } = await this.request<{ audit_logs: ServerAudit[]; total: number }>('/audit-logs?limit=100');
    return audit_logs.map((item) => ({ id: item.id, action: `${item.entity} ${item.action}`, actor: item.actor_id || item.actor_type || 'system', target: item.entity_id, at: item.created_at, severity: item.action.includes('revok') || item.action.includes('delet') ? 'warning' : item.action.includes('creat') || item.action.includes('generat') ? 'success' : 'info' }));
  }
}
