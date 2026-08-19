import type { ConsoleApi } from '../types';
import { ConsoleApiError, HttpConsoleApi } from './httpApi';
import { mockApi } from './mockApi';

const unavailable = (): never => { throw new ConsoleApiError('Connect to a LicenseHub server or explicitly enter Demo mode', 'NOT_CONNECTED', 503); };

const unavailableApi: ConsoleApi = {
  dashboard: async () => unavailable(), listProducts: async () => unavailable(), createProduct: async () => unavailable(),
  exportIntegrationKit: async () => unavailable(), listLicenses: async () => unavailable(), generateLicense: async () => unavailable(),
  revokeLicense: async () => unavailable(), listDevices: async () => unavailable(), resetDevice: async () => unavailable(),
  listSigningKeys: async () => unavailable(), rotateSigningKey: async () => unavailable(), listBackups: async () => unavailable(),
  createBackup: async () => unavailable(), listAuditEvents: async () => unavailable(),
};

export let api: ConsoleApi = unavailableApi;

export function activateHttpApi(baseUrl: string) { api = new HttpConsoleApi(baseUrl); }
export function activateDemoApi() { api = mockApi; }
export function deactivateConsoleApi() { api = unavailableApi; }

export { ConsoleApiError, HttpConsoleApi, UnsupportedConsoleOperationError } from './httpApi';
