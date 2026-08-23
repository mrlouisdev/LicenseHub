using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Text.RegularExpressions;

return await Runner.RunAsync(args);

internal static partial class Runner
{
    internal static readonly HashSet<string> Stacks = new(StringComparer.OrdinalIgnoreCase) { "dotnet", "electron", "python", "cpp" };
    internal static readonly JsonSerializerOptions JsonOptions = new() { WriteIndented = true, DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull };

    public static async Task<int> RunAsync(string[] args)
    {
        try
        {
            if (args.Length == 0 || args[0] is "-h" or "--help") { Usage(); return 0; }
            return args[0].ToLowerInvariant() switch
            {
                "init" => Init(Options.Parse(args[1..], ["stack", "product", "server", "public-key", "profile", "out"])),
                "add" => IntegrationManager.Add(Options.Parse(args[1..], ["project", "kit"])),
                "doctor" => IntegrationManager.Doctor(Options.Parse(args[1..], ["project", "kit", "json"], ["json"])),
                "verify" => await LiveVerifier.VerifyAsync(Options.Parse(args[1..], ["project", "live", "activation-stdin", "entitlement", "json"], ["live", "activation-stdin", "json"])),
                "remove" => IntegrationManager.Remove(Options.Parse(args[1..], ["project"])),
                _ => throw new ArgumentException("expected command init, add, doctor, verify or remove")
            };
        }
        catch (Exception error) { Console.Error.WriteLine($"ERROR {error.Message}"); return 2; }
    }

    private static int Init(Options parsed)
    {
        var product = parsed.Required("product");
        if (!NamePattern().IsMatch(product)) throw new ArgumentException("--product contains invalid characters");
        string endpoint;
        SortedDictionary<string, string> verificationKeys;
        if (parsed.Has("profile"))
        {
            if (parsed.Has("server") || parsed.Has("public-key")) throw new ArgumentException("--profile cannot be combined with --server or --public-key");
            var profile = LoadProfile(parsed.Required("profile"));
            endpoint = profile.ServerUrl;
            verificationKeys = profile.PublicKeys;
        }
        else
        {
            endpoint = ValidateEndpoint(parsed.Required("server"));
            verificationKeys = ParseVerificationKeys(parsed.All("public-key"));
        }
        string? legacyStack = parsed.Optional("stack")?.ToLowerInvariant();
        if (legacyStack is not null && !Stacks.Contains(legacyStack)) throw new ArgumentException("--stack must be dotnet, electron, python or cpp");
        var output = Path.GetFullPath(parsed.Required("out"));
        WriteKit(legacyStack, product, endpoint, verificationKeys, output);
        Console.WriteLine($"READY {output}");
        return 0;
    }

    internal static ProductProfile LoadProfile(string rawPath)
    {
        var path = Path.GetFullPath(rawPath);
        if (!File.Exists(path)) throw new FileNotFoundException("product profile is missing", path);
        using var document = JsonDocument.Parse(File.ReadAllText(path), StrictJson);
        var root = document.RootElement;
        if (root.ValueKind != JsonValueKind.Object || RequiredInt(root, "schema_version") != 1) throw new InvalidDataException("unsupported product profile");
        if (root.TryGetProperty("stack", out _)) throw new InvalidDataException("product.profile.json must be stack-agnostic");
        var product = RequiredString(root, "product_id");
        if (!NamePattern().IsMatch(product)) throw new InvalidDataException("product_id is invalid");
        var endpoint = ValidateEndpoint(RequiredString(root, "server_url"));
        var keys = ParseKeyObject(root.GetProperty("public_keys"));
        var timeout = root.TryGetProperty("request_timeout_seconds", out _) ? RequiredInt(root, "request_timeout_seconds") : 15;
        var tolerance = root.TryGetProperty("clock_rollback_tolerance_seconds", out _) ? RequiredInt(root, "clock_rollback_tolerance_seconds") : 300;
        if (timeout is < 1 or > 120) throw new InvalidDataException("request_timeout_seconds must be 1-120");
        if (tolerance is < 0 or > 3600) throw new InvalidDataException("clock_rollback_tolerance_seconds must be 0-3600");
        var allowInsecure = root.TryGetProperty("allow_insecure_localhost", out var insecure) && insecure.ValueKind == JsonValueKind.True;
        if (endpoint.StartsWith("http://", StringComparison.OrdinalIgnoreCase) != allowInsecure) throw new InvalidDataException("allow_insecure_localhost must be true exactly for loopback HTTP");
        var fingerprints = new SortedDictionary<string, string>(StringComparer.Ordinal);
        if (root.TryGetProperty("key_fingerprints_sha256", out var fingerprintElement))
        {
            if (fingerprintElement.ValueKind != JsonValueKind.Object) throw new InvalidDataException("key_fingerprints_sha256 must be an object");
            foreach (var property in fingerprintElement.EnumerateObject()) fingerprints[property.Name] = property.Value.GetString() ?? "";
        }
        foreach (var pair in keys)
        {
            var actual = Sha256(Convert.FromBase64String(pair.Value));
            if (!fingerprints.TryGetValue(pair.Key, out var expected) || !FixedHexEquals(actual, expected)) throw new InvalidDataException($"missing or mismatched fingerprint for verification key '{pair.Key}'");
        }
        if (fingerprints.Count != keys.Count) throw new InvalidDataException("fingerprints contain an unpinned key id");
        return new ProductProfile(1, product, endpoint, keys, fingerprints, tolerance, timeout, allowInsecure);
    }

    internal static SortedDictionary<string, string> ParseVerificationKeys(IEnumerable<string> inputs)
    {
        var result = new SortedDictionary<string, string>(StringComparer.Ordinal);
        foreach (var input in inputs)
        {
            var split = input.IndexOf('=');
            var kid = split > 0 ? input[..split] : "primary";
            var value = split > 0 ? input[(split + 1)..] : input;
            if (!NamePattern().IsMatch(kid)) throw new ArgumentException($"invalid verification key id '{kid}'");
            byte[] decoded;
            try { decoded = Convert.FromBase64String(value); } catch (FormatException) { throw new ArgumentException($"verification key '{kid}' is not base64"); }
            if (decoded.Length != 32) throw new ArgumentException($"verification key '{kid}' must decode to 32 bytes");
            if (!result.TryAdd(kid, value)) throw new ArgumentException($"duplicate verification key id '{kid}'");
        }
        if (result.Count == 0) throw new ArgumentException("at least one pinned public key is required");
        return result;
    }

    internal static SortedDictionary<string, string> ParseKeyObject(JsonElement element)
    {
        if (element.ValueKind != JsonValueKind.Object) throw new InvalidDataException("public_keys must be an object");
        return ParseVerificationKeys(element.EnumerateObject().Select(p => $"{p.Name}={p.Value.GetString() ?? ""}"));
    }
    internal static string ValidateEndpoint(string raw)
    {
        if (!Uri.TryCreate(raw, UriKind.Absolute, out var uri) || string.IsNullOrWhiteSpace(uri.Host) || !string.IsNullOrEmpty(uri.Query) || !string.IsNullOrEmpty(uri.Fragment)) throw new ArgumentException("server_url must be an absolute origin URL");
        if (uri.Scheme != Uri.UriSchemeHttps && !(uri.Scheme == Uri.UriSchemeHttp && uri.IsLoopback)) throw new ArgumentException("server_url must use HTTPS; HTTP is limited to localhost");
        return uri.GetLeftPart(UriPartial.Authority).TrimEnd('/');
    }
    internal static string RequiredString(JsonElement root, string name)
    {
        if (!root.TryGetProperty(name, out var value) || value.ValueKind != JsonValueKind.String || string.IsNullOrWhiteSpace(value.GetString())) throw new InvalidDataException($"{name} is required");
        return value.GetString()!;
    }
    internal static int RequiredInt(JsonElement root, string name)
    {
        if (!root.TryGetProperty(name, out var value) || !value.TryGetInt32(out var result)) throw new InvalidDataException($"{name} must be an integer");
        return result;
    }
    internal static string Sha256(byte[] value) => Convert.ToHexString(SHA256.HashData(value)).ToLowerInvariant();
    internal static string Sha256File(string path) { using var stream = File.OpenRead(path); return Convert.ToHexString(SHA256.HashData(stream)).ToLowerInvariant(); }
    internal static void AtomicWrite(string path, string content)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        var temporary = path + ".tmp-" + Guid.NewGuid().ToString("N");
        File.WriteAllText(temporary, content, new UTF8Encoding(false));
        File.Move(temporary, path, true);
    }
    private static bool FixedHexEquals(string actual, string expected)
    {
        if (actual.Length != expected.Length || !Regex.IsMatch(expected, "^[0-9a-fA-F]{64}$", RegexOptions.CultureInvariant)) return false;
        return CryptographicOperations.FixedTimeEquals(Encoding.ASCII.GetBytes(actual), Encoding.ASCII.GetBytes(expected.ToLowerInvariant()));
    }
    internal static bool KeysEqual(SortedDictionary<string, string> left, SortedDictionary<string, string> right) => left.Count == right.Count && left.All(pair => right.TryGetValue(pair.Key, out var value) && CryptographicOperations.FixedTimeEquals(Convert.FromBase64String(pair.Value), Convert.FromBase64String(value)));

    private static readonly JsonDocumentOptions StrictJson = new() { AllowTrailingCommas = false, CommentHandling = JsonCommentHandling.Disallow, MaxDepth = 32 };
    private static void Usage() => Console.WriteLine("""
licctl init --product ID (--profile FILE | --server URL --public-key [KID=]BASE64) --out KIT_DIR [--stack legacy-stack]
licctl add --project DIR --kit ZIP_OR_DIR
licctl doctor --project DIR [--json]
licctl verify --project DIR [--live] [--activation-stdin] [--entitlement NAME] [--json]
licctl remove --project DIR
""");
    [GeneratedRegex("^[A-Za-z0-9._-]{1,128}$", RegexOptions.CultureInvariant)] internal static partial Regex NamePattern();
    private static partial void WriteKit(string? legacyStack, string product, string endpoint, SortedDictionary<string, string> verificationKeys, string output);
}

internal sealed class Options
{
    private readonly Dictionary<string, List<string>> _values;
    private readonly HashSet<string> _flags;
    private Options(Dictionary<string, List<string>> values, HashSet<string> flags) { _values = values; _flags = flags; }
    internal static Options Parse(string[] args, IEnumerable<string> allowed, IEnumerable<string>? flags = null)
    {
        var allowedSet = allowed.ToHashSet(StringComparer.OrdinalIgnoreCase);
        var flagSet = (flags ?? []).ToHashSet(StringComparer.OrdinalIgnoreCase);
        var values = new Dictionary<string, List<string>>(StringComparer.OrdinalIgnoreCase);
        var foundFlags = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        for (var index = 0; index < args.Length; index++)
        {
            var token = args[index];
            if (!token.StartsWith("--", StringComparison.Ordinal) || token.Length == 2) throw new ArgumentException($"unexpected argument '{token}'");
            var name = token[2..];
            if (!allowedSet.Contains(name)) throw new ArgumentException($"unknown option '{token}'");
            if (flagSet.Contains(name)) { if (!foundFlags.Add(name)) throw new ArgumentException($"{token} may only be provided once"); continue; }
            if (++index >= args.Length || args[index].StartsWith("--", StringComparison.Ordinal)) throw new ArgumentException($"{token} requires a value");
            if (!values.TryGetValue(name, out var list)) values[name] = list = [];
            if (name != "public-key" && list.Count != 0) throw new ArgumentException($"{token} may only be provided once");
            list.Add(args[index]);
        }
        return new Options(values, foundFlags);
    }
    internal bool Has(string name) => _values.ContainsKey(name) || _flags.Contains(name);
    internal bool Flag(string name) => _flags.Contains(name);
    internal string Required(string name) => Optional(name) ?? throw new ArgumentException($"--{name} is required");
    internal string? Optional(string name) => _values.TryGetValue(name, out var values) && values.Count > 0 && !string.IsNullOrWhiteSpace(values[0]) ? values[0] : null;
    internal IEnumerable<string> All(string name) => _values.TryGetValue(name, out var values) ? values : [];
}

internal sealed record ProductProfile(
    [property: JsonPropertyName("schema_version")] int SchemaVersion,
    [property: JsonPropertyName("product_id")] string ProductId,
    [property: JsonPropertyName("server_url")] string ServerUrl,
    [property: JsonPropertyName("public_keys")] SortedDictionary<string, string> PublicKeys,
    [property: JsonPropertyName("key_fingerprints_sha256")] SortedDictionary<string, string> KeyFingerprints,
    [property: JsonPropertyName("clock_rollback_tolerance_seconds")] int ClockRollbackToleranceSeconds,
    [property: JsonPropertyName("request_timeout_seconds")] int RequestTimeoutSeconds,
    [property: JsonPropertyName("allow_insecure_localhost")] bool AllowInsecureLocalhost);
