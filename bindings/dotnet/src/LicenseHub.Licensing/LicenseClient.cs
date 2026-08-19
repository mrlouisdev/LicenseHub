using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace LicenseHub.Licensing;

public sealed class LicenseClient : IDisposable
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        Converters = { new JsonStringEnumConverter<LicenseState>(JsonNamingPolicy.SnakeCaseLower) }
    };
    private readonly LicenseSafeHandle _handle;
    private LicenseClient(LicenseSafeHandle handle) => _handle = handle;

    public static LicenseClient Initialize(LicenseClientConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        NativeMethods.EnsureLoaded();
        var abi = NativeMethods.license_abi_version();
        if (abi != NativeMethods.ExpectedAbiVersion)
            throw new NotSupportedException($"License core ABI {abi} is not supported; expected {NativeMethods.ExpectedAbiVersion}.");
        ThrowIfError(NativeMethods.license_initialize(JsonSerializer.Serialize(config, JsonOptions), out var handle));
        return new LicenseClient(new LicenseSafeHandle(handle));
    }

    public LicenseStatus Status()
    {
        EnsureUsable();
        var json = ReadString((buffer, length) => NativeMethods.license_status(_handle, buffer, length));
        return JsonSerializer.Deserialize<LicenseStatus>(json, JsonOptions)
            ?? throw new InvalidDataException("License core returned an empty status payload.");
    }

    public string DeviceId
    {
        get { EnsureUsable(); return ReadString((buffer, length) => NativeMethods.license_device_id(_handle, buffer, length)); }
    }

    public LicenseStatus Activate(string value)
    {
        EnsureUsable();
        ArgumentException.ThrowIfNullOrWhiteSpace(value);
        ThrowIfError(NativeMethods.license_activate(_handle, value));
        return Status();
    }

    public LicenseStatus Refresh()
    {
        EnsureUsable();
        ThrowIfError(NativeMethods.license_refresh(_handle));
        return Status();
    }

    public void RequireEntitlement(string entitlement)
    {
        EnsureUsable();
        ArgumentException.ThrowIfNullOrWhiteSpace(entitlement);
        ThrowIfError(NativeMethods.license_require_entitlement(_handle, entitlement));
    }

    public void Deactivate() { EnsureUsable(); ThrowIfError(NativeMethods.license_deactivate(_handle)); }

    public void Dispose()
    {
        _handle.Dispose();
        GC.SuppressFinalize(this);
    }

    private static string ReadString(Func<byte[]?, nuint, nint> call)
    {
        var required = call(null, 0).ToInt64();
        if (required < 0) ThrowIfError(checked((int)required));
        if (required <= 1) return string.Empty;
        var buffer = new byte[checked((int)required)];
        var written = call(buffer, (nuint)buffer.Length).ToInt64();
        if (written < 0) ThrowIfError(checked((int)written));
        var nul = Array.IndexOf(buffer, (byte)0);
        return Encoding.UTF8.GetString(buffer, 0, nul >= 0 ? nul : buffer.Length);
    }

    private static void ThrowIfError(int result)
    {
        if (result >= 0) return;
        throw new LicenseCoreException(-result, ReadLastError() is { Length: > 0 } message ? message : "unknown native error");
    }

    private static string ReadLastError()
    {
        var required = NativeMethods.license_last_error(null, 0).ToInt64();
        if (required <= 1 || required > int.MaxValue) return string.Empty;
        var buffer = new byte[(int)required];
        if (NativeMethods.license_last_error(buffer, (nuint)buffer.Length).ToInt64() < 0) return string.Empty;
        var nul = Array.IndexOf(buffer, (byte)0);
        return Encoding.UTF8.GetString(buffer, 0, nul >= 0 ? nul : buffer.Length);
    }

    private void EnsureUsable() => ObjectDisposedException.ThrowIf(_handle.IsClosed || _handle.IsInvalid, this);
}
