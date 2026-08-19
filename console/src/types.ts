export type LicenseStatus = 'active' | 'suspended' | 'revoked' | 'expired';

export interface Product {
  id: string;
  name: string;
  code: string;
  maxDevices: number;
  offlineHours: number;
  activeLicenses: number;
  createdAt: string;
}

export interface License {
  id: string;
  key: string;
  productId: string;
  productName: string;
  customer: string;
  status: LicenseStatus;
  expiresAt: string | null;
  devices: number;
  maxDevices: number;
  entitlements: string[];
}

export interface Device {
  id: string;
  name: string;
  fingerprint: string;
  licenseKey: string;
  productName: string;
  activatedAt: string;
  lastSeenAt: string;
  state: 'online' | 'offline';
}

export interface SigningKey {
  kid: string;
  algorithm: 'Ed25519';
  state: 'active' | 'retiring';
  createdAt: string;
}

export interface Backup {
  id: string;
  createdAt: string;
  size: string;
  status: 'verified' | 'running';
}

export interface AuditEvent {
  id: string;
  action: string;
  actor: string;
  target: string;
  at: string;
  severity: 'info' | 'warning' | 'success';
}

export interface DashboardSnapshot {
  products: number;
  activeLicenses: number;
  activeDevices: number;
  expiringSoon: number;
  activationSeries: number[];
  recentEvents: AuditEvent[];
}

export interface CreateProductInput {
  name: string;
  code: string;
  maxDevices: number;
  offlineHours: number;
}

export interface GenerateLicenseInput {
  productId: string;
  customer: string;
  durationDays: number | null;
  entitlements: string[];
}

export interface ConsoleApi {
  dashboard(): Promise<DashboardSnapshot>;
  listProducts(): Promise<Product[]>;
  createProduct(input: CreateProductInput): Promise<Product>;
  exportIntegrationKit(productId: string): Promise<{ filename: string }>;
  listLicenses(): Promise<License[]>;
  generateLicense(input: GenerateLicenseInput): Promise<License>;
  revokeLicense(id: string): Promise<void>;
  listDevices(): Promise<Device[]>;
  resetDevice(id: string): Promise<void>;
  listSigningKeys(productId?: string): Promise<SigningKey[]>;
  rotateSigningKey(productId?: string): Promise<SigningKey>;
  listBackups(): Promise<Backup[]>;
  createBackup(): Promise<Backup>;
  listAuditEvents(): Promise<AuditEvent[]>;
}
