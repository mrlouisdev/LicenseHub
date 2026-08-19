using System.Text.Json.Serialization;

namespace LicenseHub.Licensing;

public sealed record LicenseClientConfig
{
    [JsonPropertyName("product_id")]
    public required string ProductId { get; init; }
    [JsonPropertyName("server_url")]
    public required string ServerUrl { get; init; }
    [JsonPropertyName("cache_dir")]
    public required string CacheDirectory { get; init; }
    [JsonPropertyName("public_keys")]
    public required IReadOnlyDictionary<string, string> PublicKeys { get; init; }
    [JsonPropertyName("clock_rollback_tolerance_seconds")]
    public long ClockRollbackToleranceSeconds { get; init; } = 300;
    [JsonPropertyName("request_timeout_seconds")]
    public ulong RequestTimeoutSeconds { get; init; } = 15;
    [JsonPropertyName("allow_insecure_localhost")]
    public bool AllowInsecureLocalhost { get; init; }
}

public enum LicenseState { Active, Expired, NotActivated, ClockRollback }

public sealed record LicenseStatus
{
    [JsonPropertyName("state")]
    public required LicenseState State { get; init; }
    [JsonPropertyName("product_id")]
    public required string ProductId { get; init; }
    [JsonPropertyName("device_id")]
    public required string DeviceId { get; init; }
    [JsonPropertyName("license_id")]
    public string? LicenseId { get; init; }
    [JsonPropertyName("entitlements")]
    public IReadOnlyList<string> Entitlements { get; init; } = Array.Empty<string>();
    [JsonPropertyName("issued_at")]
    public long? IssuedAt { get; init; }
    [JsonPropertyName("expires_at")]
    public long? ExpiresAt { get; init; }
}

public sealed class LicenseCoreException : Exception
{
    public int ErrorCode { get; }
    internal LicenseCoreException(int errorCode, string message)
        : base($"License core error {errorCode}: {message}") => ErrorCode = errorCode;
}
