import { afterEach, describe, expect, it, vi } from 'vitest';
import { HttpConsoleApi, UnsupportedConsoleOperationError } from './httpApi';

const jsonResponse = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } });
const baseUrl = 'http://localhost:8080';
const product = { id: 'prd_1', name: 'Atlas', slug: 'atlas', type: 'desktop', created_at: '2026-08-19T00:00:00Z' };
const plan = { id: 'plan_1', product_id: 'prd_1', name: 'Standard', slug: 'standard', max_activations: 1, active: true, entitlements: [{ feature: 'pro' }] };
const license = { id: 'lic_1', product_id: 'prd_1', plan_id: 'plan_1', email: 'owner@example.test', license_key: 'ATLAS-KEY', status: 'active', created_at: '2026-08-19T00:00:00Z', product, plan };

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe('HttpConsoleApi', () => {
  it('lists products and projects plan policy into the console model', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ products: [product] })).mockResolvedValueOnce(jsonResponse({ plans: [plan] }));
    vi.stubGlobal('fetch', fetchMock);
    const api = new HttpConsoleApi(baseUrl);
    await expect(api.listProducts()).resolves.toMatchObject([{ id: 'prd_1', maxDevices: 1, offlineHours: 72 }]);
    expect(fetchMock).toHaveBeenCalledWith(`${baseUrl}/api/v1/admin/products`, expect.objectContaining({ credentials: 'include', headers: expect.any(Headers) }));
    expect((fetchMock.mock.calls[0][1].headers as Headers).has('Authorization')).toBe(false);
  });

  it('creates a product and its default one-device plan', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(product, 201)).mockResolvedValueOnce(jsonResponse(plan, 201));
    vi.stubGlobal('fetch', fetchMock);
    const api = new HttpConsoleApi(baseUrl);
    await expect(api.createProduct({ name: 'Atlas', code: 'ATLAS', maxDevices: 1, offlineHours: 72 })).resolves.toMatchObject({ id: 'prd_1', maxDevices: 1 });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({ name: 'Atlas', slug: 'atlas', type: 'desktop' });
    expect(JSON.parse(fetchMock.mock.calls[1][1].body as string)).toMatchObject({ product_id: 'prd_1', max_activations: 1, license_type: 'perpetual' });
  });

  it('selects an active plan, generates a license, and revokes it', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ plans: [plan] })).mockResolvedValueOnce(jsonResponse(license, 201)).mockResolvedValueOnce(jsonResponse({ status: 'revoked' }));
    vi.stubGlobal('fetch', fetchMock);
    const api = new HttpConsoleApi(baseUrl);
    await expect(api.generateLicense({ productId: 'prd_1', customer: 'owner@example.test', durationDays: 365, entitlements: ['pro'] })).resolves.toMatchObject({ id: 'lic_1', customer: 'owner@example.test' });
    expect(JSON.parse(fetchMock.mock.calls[1][1].body as string)).toMatchObject({ product_id: 'prd_1', plan_id: 'plan_1', email: 'owner@example.test' });
    await expect(api.revokeLicense('lic_1')).resolves.toBeUndefined();
    expect(fetchMock.mock.calls[2][0]).toBe(`${baseUrl}/api/v1/admin/licenses/lic_1/revoke`);
  });

  it('resets an activation through the server device route', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);
    const api = new HttpConsoleApi(baseUrl);
    await expect(api.resetDevice('act_1')).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith(`${baseUrl}/api/v1/admin/activations/act_1`, expect.objectContaining({ method: 'DELETE' }));
  });

  it('lists and rotates signing keys for a selected product', async () => {
    const key = { id: 'key_1', product_id: 'prd_1', active: true, created_at: '2026-08-19T00:00:00Z' };
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ keys: [key] })).mockResolvedValueOnce(jsonResponse({ ...key, id: 'key_2' }, 201));
    vi.stubGlobal('fetch', fetchMock);
    const api = new HttpConsoleApi(baseUrl);
    await expect(api.listSigningKeys('prd_1')).resolves.toMatchObject([{ kid: 'key_1', state: 'active' }]);
    await expect(api.rotateSigningKey('prd_1')).resolves.toMatchObject({ kid: 'key_2', state: 'active' });
    expect(fetchMock.mock.calls[1][0]).toBe(`${baseUrl}/api/v1/admin/products/prd_1/signing-key/rotate`);
  });

  it('fails clearly for server operations that do not exist', async () => {
    const api = new HttpConsoleApi(baseUrl);
    await expect(api.createBackup()).rejects.toBeInstanceOf(UnsupportedConsoleOperationError);
    await expect(api.exportIntegrationKit('prd_1')).rejects.toMatchObject({ code: 'UNSUPPORTED_OPERATION' });
  });

  it('surfaces typed server errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ success: false, error: { code: 'UNAUTHORIZED', message: 'session required' } }), { status: 401, headers: { 'Content-Type': 'application/json' } })));
    const api = new HttpConsoleApi(baseUrl);
    await expect(api.listLicenses()).rejects.toMatchObject({ name: 'ConsoleApiError', code: 'UNAUTHORIZED', status: 401, message: 'session required' });
  });
});
