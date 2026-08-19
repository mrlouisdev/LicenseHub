export type LicenseState = "active" | "expired" | "not_activated" | "clock_rollback";

export interface LicenseClientConfig {
  product_id: string;
  server_url: string;
  cache_dir: string;
  public_keys: Record<string, string>;
  clock_rollback_tolerance_seconds?: number;
  request_timeout_seconds?: number;
  allow_insecure_localhost?: boolean;
}

export interface LicenseStatus {
  state: LicenseState;
  product_id: string;
  device_id: string;
  license_id: string | null;
  entitlements: string[];
  issued_at: number | null;
  expires_at: number | null;
}

export class LicenseCoreError extends Error {
  readonly code: number;
}

export class LicenseClient {
  static initialize(config: LicenseClientConfig, options?: { nativePath?: string }): LicenseClient;
  readonly deviceId: string;
  status(): LicenseStatus;
  activate(value: string): LicenseStatus;
  refresh(): LicenseStatus;
  requireEntitlement(entitlement: string): void;
  deactivate(): void;
  close(): void;
}
