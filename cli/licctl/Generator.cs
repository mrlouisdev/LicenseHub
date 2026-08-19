using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

internal static partial class Runner
{
    private static partial void WriteKit(string stack, string product, string endpoint, SortedDictionary<string, string> verificationKeys, string output)
    {
        Directory.CreateDirectory(output);
        var manifestPath = Path.Combine(output, "product.manifest.json");
        var snippetName = stack switch
        {
            "dotnet" => "LicenseIntegration.cs", "electron" => "license-integration.cjs",
            "python" => "license_integration.py", _ => "license_integration.hpp"
        };
        var snippetPath = Path.Combine(output, snippetName);
        var guidePath = Path.Combine(output, "INTEGRATION.md");
        foreach (var file in new[] { manifestPath, snippetPath, guidePath })
            if (File.Exists(file)) throw new IOException($"refusing to overwrite '{file}'");

        var manifest = new ProductManifest(1, product, endpoint, verificationKeys, 300, 15, endpoint.StartsWith("http://", StringComparison.OrdinalIgnoreCase));
        File.WriteAllText(manifestPath, JsonSerializer.Serialize(manifest, JsonOptions), new UTF8Encoding(false));
        File.WriteAllText(snippetPath, Snippet(stack, product, endpoint, verificationKeys), new UTF8Encoding(false));
        File.WriteAllText(guidePath, Guide(stack, snippetName), new UTF8Encoding(false));
    }

    private static string Snippet(string stack, string product, string endpoint, SortedDictionary<string, string> verificationKeys) => stack switch
    {
        "dotnet" => DotNetSnippet,
        "electron" => ElectronSnippet,
        "python" => PythonSnippet,
        _ => CppSnippet(product, endpoint, verificationKeys),
    };

    private static string CppSnippet(string product, string endpoint, SortedDictionary<string, string> verificationKeys)
    {
        var entries = string.Join(", ", verificationKeys.Select(pair => $"{{\"{EscapeCpp(pair.Key)}\", \"{EscapeCpp(pair.Value)}\"}}"));
        var keyMap = "{" + entries + "}";
        return $$"""
#pragma once
#include <licensehub/license_client.hpp>

inline licensehub::client make_license_client() {
    licensehub::config config{"{{EscapeCpp(product)}}", "{{EscapeCpp(endpoint)}}", "license-cache", {{keyMap}}, 300, 15, {{(endpoint.StartsWith("http://", StringComparison.OrdinalIgnoreCase) ? "true" : "false")}}};
    return licensehub::client(config);
}
""";
    }

    private static string EscapeCpp(string value) => value.Replace("\\", "\\\\", StringComparison.Ordinal).Replace("\"", "\\\"", StringComparison.Ordinal);
    private static string Guide(string stack, string snippet) => $"""
# LicenseHub integration kit

Stack: `{stack}`

1. Install the matching package from `bindings/{(stack == "electron" ? "node" : stack)}`.
2. Copy `product.manifest.json` beside the application.
3. Adapt `{snippet}` into the startup and activation flow.
4. Enforce entitlements again at protected feature boundaries.
5. Ship the matching `license_core` runtime with the application.

The manifest contains public verification material only.
""";

    private static readonly JsonSerializerOptions JsonOptions = new() { WriteIndented = true };
    private sealed record ProductManifest(
        [property: JsonPropertyName("schema_version")] int SchemaVersion,
        [property: JsonPropertyName("product_id")] string ProductId,
        [property: JsonPropertyName("server_url")] string ServerUrl,
        [property: JsonPropertyName("public_keys")] IReadOnlyDictionary<string, string> PublicKeys,
        [property: JsonPropertyName("clock_rollback_tolerance_seconds")] int ClockTolerance,
        [property: JsonPropertyName("request_timeout_seconds")] int RequestTimeout,
        [property: JsonPropertyName("allow_insecure_localhost")] bool AllowInsecureLocalhost);

    private const string DotNetSnippet = """
using System.Text.Json;
using LicenseHub.Licensing;

var manifest = JsonDocument.Parse(File.ReadAllText("product.manifest.json")).RootElement;
var product = manifest.GetProperty("product_id").GetString()!;
var client = LicenseClient.Initialize(new LicenseClientConfig {
    ProductId = product,
    ServerUrl = manifest.GetProperty("server_url").GetString()!,
    CacheDirectory = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "LicenseHub", product),
    PublicKeys = manifest.GetProperty("public_keys").EnumerateObject().ToDictionary(p => p.Name, p => p.Value.GetString()!)
});
// Connect client.Activate(userInput) to activation UI and gate protected features with RequireEntitlement.
""";

    private const string ElectronSnippet = """
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { LicenseClient } = require("@licensehub/licensing");
const manifest = JSON.parse(fs.readFileSync(path.join(__dirname, "product.manifest.json"), "utf8"));
const client = LicenseClient.initialize({ ...manifest, cache_dir: path.join(os.homedir(), ".licensehub", manifest.product_id) });
// Keep the client in Electron's main process and expose narrow IPC methods.
module.exports = client;
""";

    private const string PythonSnippet = """
import json
import os
from pathlib import Path
from licensehub_licensing import LicenseClient

manifest = json.loads(Path(__file__).with_name("product.manifest.json").read_text(encoding="utf-8"))
manifest["cache_dir"] = str(Path(os.getenv("LOCALAPPDATA", Path.home())) / "LicenseHub" / manifest["product_id"])
client = LicenseClient.initialize(manifest)
# Connect client.activate(user_input) to activation UI and call require_entitlement at protected features.
""";
}
