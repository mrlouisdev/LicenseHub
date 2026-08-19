import { runtimeFetch } from './transport';

export interface ConsoleUser { id: string; email: string; name: string; isAdmin: boolean; role: string }
interface Envelope<T> { success: boolean; data?: T; error?: { code: string; message: string } }

export class ConnectionError extends Error {
  constructor(message: string, public readonly code: string, public readonly status: number) { super(message); this.name = 'ConnectionError'; }
}

export function validateServerUrl(value: string): string {
  let url: URL;
  try { url = new URL(value.trim()); } catch { throw new ConnectionError('Enter a valid server URL', 'INVALID_URL', 400); }
  const local = url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]';
  if (url.protocol !== 'https:' && !(local && url.protocol === 'http:')) throw new ConnectionError('Use HTTPS. HTTP is allowed only for localhost.', 'INSECURE_URL', 400);
  if (url.username || url.password || url.search || url.hash) throw new ConnectionError('Server URL cannot contain credentials, query, or fragment', 'INVALID_URL', 400);
  return url.origin;
}

export class AuthApi {
  readonly baseUrl: string;
  constructor(value: string) { this.baseUrl = validateServerUrl(value); }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers); headers.set('Accept', 'application/json');
    if (init.body) headers.set('Content-Type', 'application/json');
    let response: Response;
    try { response = await runtimeFetch(`${this.baseUrl}${path}`, { credentials: 'include', ...init, headers }); }
    catch { throw new ConnectionError('Unable to reach the LicenseHub server', 'NETWORK_ERROR', 0); }
    let envelope: Envelope<T>;
    try { envelope = await response.json() as Envelope<T>; }
    catch { throw new ConnectionError(`Server returned an invalid response (${response.status})`, 'INVALID_RESPONSE', response.status); }
    if (!response.ok || !envelope.success || envelope.data === undefined) throw new ConnectionError(envelope.error?.message ?? `Request failed (${response.status})`, envelope.error?.code ?? 'HTTP_ERROR', response.status);
    return envelope.data;
  }

  testConnection() { return this.request<{ version: string; project: string }>('/api/v1/version'); }
  sendOtp(email: string) { return this.request<{ status: string }>('/api/v1/auth/otp/send', { method: 'POST', body: JSON.stringify({ email }) }); }
  verifyOtp(email: string, code: string) { return this.request<{ status: string; email: string; name: string; is_admin: boolean; role: string }>('/api/v1/auth/otp/verify', { method: 'POST', body: JSON.stringify({ email, code }) }); }
  async currentUser(): Promise<ConsoleUser> {
    const user = await this.request<{ id: string; email: string; name: string; is_admin: boolean; role: string }>('/api/v1/portal/me');
    return { id: user.id, email: user.email, name: user.name, isAdmin: user.is_admin, role: user.role };
  }
  logout() { return this.request<{ status: string }>('/api/v1/auth/logout', { method: 'POST' }); }
}
