import { invoke } from '@tauri-apps/api/core';

interface NativeResponse { status: number; body: string; contentType: string }
const isTauri = () => '__TAURI_INTERNALS__' in window;

export async function runtimeFetch(url: string, init: RequestInit = {}): Promise<Response> {
  if (!isTauri()) return fetch(url, init);
  const headers = Object.fromEntries(new Headers(init.headers).entries());
  const result = await invoke<NativeResponse>('http_request', {
    request: { url, method: init.method ?? 'GET', headers, body: typeof init.body === 'string' ? init.body : null },
  });
  return new Response(result.body, { status: result.status, headers: { 'Content-Type': result.contentType || 'application/json' } });
}
