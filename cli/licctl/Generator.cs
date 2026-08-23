using System.Text.Json;

internal static partial class Runner
{
    private static partial void WriteKit(string? legacyStack, string product, string endpoint, SortedDictionary<string, string> verificationKeys, string output)
    {
        Directory.CreateDirectory(output);
        var profilePath = Path.Combine(output, "product.profile.json");
        var guidePath = Path.Combine(output, "INTEGRATION.md");
        foreach (var file in new[] { profilePath, guidePath })
            if (File.Exists(file)) throw new IOException($"refusing to overwrite '{file}'");

        var fingerprints = new SortedDictionary<string, string>(
            verificationKeys.ToDictionary(pair => pair.Key, pair => Sha256(Convert.FromBase64String(pair.Value))),
            StringComparer.Ordinal);
        var profile = new ProductProfile(1, product, endpoint, verificationKeys, fingerprints, 300, 15,
            endpoint.StartsWith("http://", StringComparison.OrdinalIgnoreCase));
        AtomicWrite(profilePath, JsonSerializer.Serialize(profile, JsonOptions));
        AtomicWrite(guidePath, """
# LicenseHub product profile

This kit is intentionally stack-agnostic. Its Ed25519 public keys and SHA-256
fingerprints are local trust anchors; `licctl` never replaces them with keys
downloaded from the network.

```text
licctl add --project . --kit <this-directory-or-zip>
licctl doctor --project . --json
licctl verify --project . --live --activation-stdin
licctl remove --project .
```
""");
        IntegrationManager.CopyDistributablePayload(output);
        if (legacyStack is not null)
            Console.Error.WriteLine("NOTICE --stack is accepted for backward compatibility but product.profile.json remains stack-agnostic; add auto-detects the project");
    }
}
