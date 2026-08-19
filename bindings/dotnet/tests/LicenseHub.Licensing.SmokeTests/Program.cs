using LicenseHub.Licensing;

var config = new LicenseClientConfig
{
    ProductId = "wrapper_smoke",
    ServerUrl = "http://localhost:18080",
    CacheDirectory = Path.Combine(Path.GetTempPath(), "licensehub-dotnet-test", Guid.NewGuid().ToString("N")),
    PublicKeys = new Dictionary<string, string> { ["test"] = "11qYAYdk9J2EORuRTvM9P4BKrMvBf7d7n8U8rTjU5YI=" },
    AllowInsecureLocalhost = true
};
using var client = LicenseClient.Initialize(config);
if (client.Status().State != LicenseState.NotActivated) throw new Exception("new client must be not_activated");
if (!client.DeviceId.StartsWith("dev_", StringComparison.Ordinal)) throw new Exception("device id prefix mismatch");
try { client.RequireEntitlement("pro"); throw new Exception("missing activation must fail"); }
catch (LicenseCoreException error) when (error.ErrorCode == 41) { }
Console.WriteLine("PASS dotnet wrapper ABI/status/error smoke test");
