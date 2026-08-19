import { describe, expect, it } from 'vitest';
import { validateServerUrl } from './authApi';

describe('connection URL validation', () => {
  it('requires HTTPS except for localhost development', () => {
    expect(validateServerUrl('https://licenses.example.test/path')).toBe('https://licenses.example.test');
    expect(validateServerUrl('http://localhost:8080')).toBe('http://localhost:8080');
    expect(() => validateServerUrl('http://licenses.example.test')).toThrow('HTTPS');
  });
});
