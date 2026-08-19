using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Text.RegularExpressions;

return Runner.Run(args);

internal static partial class Runner
{
    private static readonly HashSet<string> Stacks = new(StringComparer.OrdinalIgnoreCase) { "dotnet", "electron", "python", "cpp" };

    public static int Run(string[] args)
    {
        try
        {
            if (args.Length == 0 || args[0] is "-h" or "--help") { Usage(); return 0; }
            if (!string.Equals(args[0], "init", StringComparison.OrdinalIgnoreCase)) throw new ArgumentException("expected command 'init'");
            var parsed = Parse(args[1..]);
            var stack = Required(parsed, "stack").ToLowerInvariant();
            if (!Stacks.Contains(stack)) throw new ArgumentException("--stack must be dotnet, electron, python or cpp");
            var product = Required(parsed, "product");
            if (!NamePattern().IsMatch(product)) throw new ArgumentException("--product contains invalid characters");
            var endpoint = ValidateEndpoint(Required(parsed, "server"));
            var verificationKeys = ParseVerificationKeys(parsed.GetValueOrDefault("public-key") ?? []);
            var output = Path.GetFullPath(Required(parsed, "out"));
            WriteKit(stack, product, endpoint, verificationKeys, output);
            Console.WriteLine($"READY {output}");
            return 0;
        }
        catch (Exception error) { Console.Error.WriteLine($"ERROR {error.Message}"); return 2; }
    }

    private static Dictionary<string, List<string>> Parse(string[] args)
    {
        var result = new Dictionary<string, List<string>>(StringComparer.OrdinalIgnoreCase);
        for (var index = 0; index < args.Length; index++)
        {
            var token = args[index];
            if (!token.StartsWith("--", StringComparison.Ordinal) || token.Length == 2) throw new ArgumentException($"unexpected argument '{token}'");
            if (++index >= args.Length || args[index].StartsWith("--", StringComparison.Ordinal)) throw new ArgumentException($"{token} requires a value");
            var name = token[2..];
            if (name is not ("stack" or "product" or "server" or "public-key" or "out")) throw new ArgumentException($"unknown option '{token}'");
            if (!result.TryGetValue(name, out var values)) result[name] = values = [];
            if (name != "public-key" && values.Count != 0) throw new ArgumentException($"{token} may only be provided once");
            values.Add(args[index]);
        }
        return result;
    }

    private static string Required(Dictionary<string, List<string>> values, string name)
    {
        if (!values.TryGetValue(name, out var found) || found.Count == 0 || string.IsNullOrWhiteSpace(found[0])) throw new ArgumentException($"--{name} is required");
        return found[0];
    }

    private static string ValidateEndpoint(string raw)
    {
        if (!Uri.TryCreate(raw, UriKind.Absolute, out var uri) || string.IsNullOrWhiteSpace(uri.Host)) throw new ArgumentException("--server must be an absolute URL");
        if (uri.Scheme != Uri.UriSchemeHttps && !(uri.Scheme == Uri.UriSchemeHttp && uri.IsLoopback)) throw new ArgumentException("--server must use HTTPS; HTTP is limited to localhost");
        return uri.GetLeftPart(UriPartial.Authority).TrimEnd('/');
    }

    private static SortedDictionary<string, string> ParseVerificationKeys(List<string> inputs)
    {
        if (inputs.Count == 0) throw new ArgumentException("--public-key is required");
        var result = new SortedDictionary<string, string>(StringComparer.Ordinal);
        foreach (var input in inputs)
        {
            var split = input.IndexOf('=');
            var kid = split > 0 ? input[..split] : "primary";
            var value = split > 0 ? input[(split + 1)..] : input;
            if (!NamePattern().IsMatch(kid)) throw new ArgumentException($"invalid verification key id '{kid}'");
            byte[] decoded;
            try { decoded = Convert.FromBase64String(value); }
            catch (FormatException) { throw new ArgumentException($"verification key '{kid}' is not base64"); }
            if (decoded.Length != 32) throw new ArgumentException($"verification key '{kid}' must decode to 32 bytes");
            if (!result.TryAdd(kid, value)) throw new ArgumentException($"duplicate verification key id '{kid}'");
        }
        return result;
    }

    private static void Usage() => Console.WriteLine("licctl init --stack dotnet|electron|python|cpp --product ID --server URL --public-key [KID=]BASE64 --out DIR");
    [GeneratedRegex("^[A-Za-z0-9._-]{1,128}$", RegexOptions.CultureInvariant)] private static partial Regex NamePattern();

    private static partial void WriteKit(string stack, string product, string endpoint, SortedDictionary<string, string> verificationKeys, string output);
}
