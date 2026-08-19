using System.Text.Json;
using System.Text.Json.Serialization;
using LicenseHub.Licensing;

if (args.Length < 1) { Console.Error.WriteLine("usage: dotnet run -- <product.manifest.json> [activation-value]"); return 2; }
var manifest = JsonDocument.Parse(File.ReadAllText(args[0])).RootElement;
var product = manifest.GetProperty("product_id").GetString()!;
var keys = manifest.GetProperty("public_keys").EnumerateObject().ToDictionary(p => p.Name, p => p.Value.GetString()!);
using var client = LicenseClient.Initialize(new LicenseClientConfig
{
    ProductId = product,
    ServerUrl = manifest.GetProperty("server_url").GetString()!,
    CacheDirectory = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "LicenseHub", product),
    PublicKeys = keys,
    AllowInsecureLocalhost = manifest.TryGetProperty("allow_insecure_localhost", out var local) && local.GetBoolean()
});
var status = args.Length > 1 ? client.Activate(args[1]) : client.Status();
var jsonOptions = new JsonSerializerOptions { WriteIndented = true };
jsonOptions.Converters.Add(new JsonStringEnumConverter<LicenseState>(JsonNamingPolicy.SnakeCaseLower));
Console.WriteLine(JsonSerializer.Serialize(status, jsonOptions));
return status.State is LicenseState.Active or LicenseState.NotActivated ? 0 : 1;
