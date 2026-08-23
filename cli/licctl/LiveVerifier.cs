using System.Net.Http.Json;
using System.Text.Json;

internal static class LiveVerifier
{
    internal static async Task<int> VerifyAsync(Options options)
    {
        var project = IntegrationManager.ProjectRoot(options.Optional("project") ?? ".");
        var install = IntegrationManager.ReadInstall(project);
        var profile = Runner.LoadProfile(Path.Combine(project, ".licensehub", "product.profile.json"));
        if (!IntegrationManager.CheckInstalledHashes(project, install)) throw new IOException("an installed file is missing or modified");
        if (!options.Flag("live")) { Console.WriteLine("VERIFY_OK local configuration and installed hashes are valid"); return 0; }

        using var http = new HttpClient(new HttpClientHandler { AllowAutoRedirect = false })
        {
            BaseAddress = new Uri(profile.ServerUrl),
            Timeout = TimeSpan.FromSeconds(profile.RequestTimeoutSeconds)
        };
        using var keyResponse = await http.GetAsync("/v1/client/public-keys");
        if (!keyResponse.IsSuccessStatusCode) throw new InvalidOperationException($"public-key endpoint returned {(int)keyResponse.StatusCode}");
        using var keyDocument = JsonDocument.Parse(await keyResponse.Content.ReadAsStringAsync());
        var liveKeys = Runner.ParseKeyObject(keyDocument.RootElement.GetProperty("keys"));
        if (!Runner.KeysEqual(profile.PublicKeys, liveKeys)) throw new InvalidOperationException("live signing keys do not exactly match the locally pinned profile; refusing trust-on-first-use");

        var result = new Dictionary<string, object?> { ["keys_pinned"] = true, ["server"] = profile.ServerUrl };
        if (options.Flag("activation-stdin"))
        {
            var activation = (await Console.In.ReadToEndAsync()).Trim();
            if (activation.Length == 0) throw new ArgumentException("--activation-stdin requires a non-empty value on standard input");
            var devicePath = Path.Combine(project, ".licensehub", "verify-device-id");
            var deviceId = File.Exists(devicePath) ? File.ReadAllText(devicePath).Trim() : $"licctl-{Guid.NewGuid():N}";
            if (!File.Exists(devicePath)) Runner.AtomicWrite(devicePath, deviceId + Environment.NewLine);
            using var activate = await http.PostAsJsonAsync("/v1/client/activate", new { product_id = profile.ProductId, license_key = activation, device_id = deviceId, label = "licctl verify" });
            var activateText = await activate.Content.ReadAsStringAsync();
            if (!activate.IsSuccessStatusCode) throw new InvalidOperationException($"activation lifecycle failed with HTTP {(int)activate.StatusCode}");
            using var activated = JsonDocument.Parse(activateText);
            var lease = Runner.RequiredString(activated.RootElement, "lease");
            using var refresh = await http.PostAsJsonAsync("/v1/client/refresh", new { product_id = profile.ProductId, device_id = deviceId, lease });
            var refreshText = await refresh.Content.ReadAsStringAsync();
            if (!refresh.IsSuccessStatusCode) throw new InvalidOperationException($"refresh lifecycle failed with HTTP {(int)refresh.StatusCode}");
            using var refreshed = JsonDocument.Parse(refreshText);
            var refreshedLease = Runner.RequiredString(refreshed.RootElement, "lease");
            if (options.Optional("entitlement") is { Length: > 0 } entitlement)
            {
                var entitlements = refreshed.RootElement.GetProperty("entitlements").EnumerateArray().Select(x => x.GetString()).ToHashSet(StringComparer.Ordinal);
                if (!entitlements.Contains(entitlement)) throw new InvalidOperationException($"required entitlement '{entitlement}' is absent");
            }
            using var deactivate = await http.PostAsJsonAsync("/v1/client/deactivate", new { product_id = profile.ProductId, device_id = deviceId, lease = refreshedLease });
            if (!deactivate.IsSuccessStatusCode) throw new InvalidOperationException($"deactivation lifecycle failed with HTTP {(int)deactivate.StatusCode}");
            result["lifecycle"] = "activate_refresh_deactivate";
        }
        if (options.Flag("json")) Console.WriteLine(JsonSerializer.Serialize(result, Runner.JsonOptions));
        else Console.WriteLine("VERIFY_OK live signing identity pinned" + (result.ContainsKey("lifecycle") ? "; activate/refresh/deactivate passed" : ""));
        return 0;
    }
}
