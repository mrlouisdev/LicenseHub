using System.Diagnostics;
using System.IO.Compression;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.Json.Serialization;
using System.Text.RegularExpressions;

internal static class IntegrationManager
{
    private const string InstallRelative = ".licensehub/install.json";
    private const string BeginMarker = "<!-- LICENSEHUB:BEGIN -->";
    private const string EndMarker = "<!-- LICENSEHUB:END -->";

    internal static int Add(Options options)
    {
        var project = ProjectRoot(options.Required("project"));
        using var kit = KitSource.Open(options.Required("kit"));
        var profileSource = Path.Combine(kit.Root, "product.profile.json");
        var profile = Runner.LoadProfile(profileSource);
        var stack = DetectStack(project);
        var existingPath = Path.Combine(project, InstallRelative.Replace('/', Path.DirectorySeparatorChar));
        if (File.Exists(existingPath))
        {
            var existing = ReadInstall(project);
            var expectedProfile = Runner.Sha256File(profileSource);
            if (existing.Stack == stack && existing.ProfileSha256 == expectedProfile && CheckInstalledHashes(project, existing))
            {
                Console.WriteLine($"NOOP stack={stack} project={project}");
                return 0;
            }
            throw new InvalidOperationException("an existing LicenseHub installation differs or was modified; run doctor/remove before add");
        }
        var stateDirectory = Path.Combine(project, ".licensehub");
        if (Directory.Exists(stateDirectory)) throw new IOException(".licensehub already exists without an install manifest; move it aside before add");

        var sourceRoot = FindPayloadRoot(kit.Root, project);
        var tracker = new MutationTracker(project);
        try
        {
            Directory.CreateDirectory(stateDirectory);
            tracker.CopyCreated(profileSource, ".licensehub/product.profile.json");
            switch (stack)
            {
                case "dotnet": InstallDotNet(project, sourceRoot, profile, tracker); break;
                case "electron": InstallElectron(project, sourceRoot, tracker); break;
                case "python": InstallPython(project, sourceRoot, tracker); break;
                case "cpp": InstallCpp(project, sourceRoot, profile, tracker); break;
            }

            var files = tracker.Files.ToList();
            foreach (var file in Directory.EnumerateFiles(stateDirectory, "*", SearchOption.AllDirectories))
            {
                var relative = Relative(project, file);
                if (relative == InstallRelative || relative.StartsWith(".licensehub/rollback/", StringComparison.Ordinal)) continue;
                if (files.All(item => item.Path != relative)) files.Add(new InstallFile(relative, "created", Runner.Sha256File(file), null, null));
            }
            var install = new InstallRecord(1, stack, profile.ProductId, Runner.Sha256File(profileSource), DateTimeOffset.UtcNow, files.OrderBy(x => x.Path, StringComparer.Ordinal).ToArray());
            Runner.AtomicWrite(existingPath, JsonSerializer.Serialize(install, Runner.JsonOptions));
            Console.WriteLine($"ADDED stack={stack} product={profile.ProductId} files={install.Files.Count} project={project}");
            return 0;
        }
        catch
        {
            tracker.Rollback();
            if (Directory.Exists(stateDirectory)) Directory.Delete(stateDirectory, true);
            throw;
        }
    }

    internal static int Remove(Options options)
    {
        var project = ProjectRoot(options.Required("project"));
        var install = ReadInstall(project);
        foreach (var file in install.Files.Where(file => !file.Path.StartsWith(".licensehub/", StringComparison.Ordinal)))
        {
            var path = ResolveInside(project, file.Path);
            if (!File.Exists(path)) throw new IOException($"installed file is missing: {file.Path}");
            if (!string.Equals(Runner.Sha256File(path), file.InstalledSha256, StringComparison.Ordinal))
                throw new IOException($"refusing to overwrite locally modified installed file: {file.Path}");
        }
        foreach (var file in install.Files.Where(file => file.Kind == "modified"))
        {
            var path = ResolveInside(project, file.Path);
            var backup = ResolveInside(project, file.Backup!);
            if (!File.Exists(backup)) throw new IOException($"rollback backup is missing: {file.Backup}");
            File.Copy(backup, path, true);
            if (file.OriginalSha256 is not null && Runner.Sha256File(path) != file.OriginalSha256) throw new IOException($"rollback hash mismatch: {file.Path}");
        }
        foreach (var file in install.Files.Where(file => file.Kind == "created" && !file.Path.StartsWith(".licensehub/", StringComparison.Ordinal)))
            File.Delete(ResolveInside(project, file.Path));
        Directory.Delete(Path.Combine(project, ".licensehub"), true);
        Console.WriteLine($"REMOVED stack={install.Stack} project={project}");
        return 0;
    }

    internal static int Doctor(Options options)
    {
        if (options.Has("kit"))
        {
            using var kit = KitSource.Open(options.Required("kit"));
            var kitProfile = Runner.LoadProfile(Path.Combine(kit.Root, "product.profile.json"));
            Console.WriteLine(options.Flag("json")
                ? JsonSerializer.Serialize(new { ok = true, kind = "kit", product = kitProfile.ProductId, keys = kitProfile.PublicKeys.Count }, Runner.JsonOptions)
                : $"DOCTOR_OK kit product={kitProfile.ProductId} keys={kitProfile.PublicKeys.Count}");
            return 0;
        }
        var project = ProjectRoot(options.Optional("project") ?? ".");
        var install = ReadInstall(project);
        var profile = Runner.LoadProfile(Path.Combine(project, ".licensehub", "product.profile.json"));
        var results = new List<DoctorCheck>
        {
            new("manifest", true, $"schema=1 stack={install.Stack}"),
            new("profile", profile.ProductId == install.ProductId, $"product={profile.ProductId}"),
            new("hashes", CheckInstalledHashes(project, install), $"files={install.Files.Count}")
        };
        results.Add(RunStackDoctor(project, install.Stack));
        var ok = results.All(result => result.Ok);
        if (options.Flag("json")) Console.WriteLine(JsonSerializer.Serialize(new { ok, stack = install.Stack, product = install.ProductId, checks = results }, Runner.JsonOptions));
        else Console.WriteLine((ok ? "DOCTOR_OK" : "DOCTOR_FAIL") + $" stack={install.Stack}; " + string.Join("; ", results.Select(x => $"{x.Name}={(x.Ok ? "ok" : "fail")} ({x.Detail})")));
        return ok ? 0 : 1;
    }

    internal static string ProjectRoot(string raw)
    {
        var path = Path.GetFullPath(raw);
        if (!Directory.Exists(path)) throw new DirectoryNotFoundException($"project directory does not exist: {path}");
        return path.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
    }
    internal static InstallRecord ReadInstall(string project)
    {
        var path = Path.Combine(project, InstallRelative.Replace('/', Path.DirectorySeparatorChar));
        if (!File.Exists(path)) throw new FileNotFoundException("LicenseHub is not installed in this project", path);
        var value = JsonSerializer.Deserialize<InstallRecord>(File.ReadAllText(path), Runner.JsonOptions) ?? throw new InvalidDataException("install.json is empty");
        if (value.SchemaVersion != 1 || !Runner.Stacks.Contains(value.Stack)) throw new InvalidDataException("install.json schema or stack is unsupported");
        return value;
    }
    internal static bool CheckInstalledHashes(string project, InstallRecord install)
    {
        foreach (var file in install.Files)
        {
            var path = ResolveInside(project, file.Path);
            if (!File.Exists(path) || Runner.Sha256File(path) != file.InstalledSha256) return false;
        }
        return true;
    }

    private static string DetectStack(string project)
    {
        var detected = new List<string>();
        if (Directory.EnumerateFiles(project, "*.csproj", SearchOption.TopDirectoryOnly).Any()) detected.Add("dotnet");
        if (File.Exists(Path.Combine(project, "package.json"))) detected.Add("electron");
        if (File.Exists(Path.Combine(project, "pyproject.toml")) || File.Exists(Path.Combine(project, "requirements.txt")) || Directory.EnumerateFiles(project, "*.py", SearchOption.TopDirectoryOnly).Any()) detected.Add("python");
        if (File.Exists(Path.Combine(project, "CMakeLists.txt"))) detected.Add("cpp");
        if (detected.Count == 0) throw new InvalidOperationException("could not detect dotnet, electron, python or cpp project at the project root");
        if (detected.Count > 1) throw new InvalidOperationException($"ambiguous project stack ({string.Join(", ", detected)}); use a stack-specific project root");
        return detected[0];
    }

    private static string FindPayloadRoot(string kitRoot, string project)
    {
        foreach (var candidate in new[] { kitRoot, AppContext.BaseDirectory, project, Directory.GetCurrentDirectory() }.SelectMany(Ancestors).Distinct(StringComparer.OrdinalIgnoreCase))
            if (Directory.Exists(Path.Combine(candidate, "bindings")) && File.Exists(Path.Combine(candidate, "core", "include", "license_core.h"))) return candidate;
        throw new DirectoryNotFoundException("LicenseHub bindings payload was not found in the kit, beside licctl, or in a repository ancestor");
    }

    // Make `licctl init` output portable: add must not depend on being run from
    // a LicenseHub source checkout. Only runtime files required by the four
    // supported adapters are copied; tests, build output and private material
    // are deliberately excluded.
    internal static void CopyDistributablePayload(string kitRoot)
    {
        var source = FindPayloadRoot(kitRoot, kitRoot);
        if (Path.GetFullPath(source).TrimEnd(Path.DirectorySeparatorChar).Equals(
            Path.GetFullPath(kitRoot).TrimEnd(Path.DirectorySeparatorChar), StringComparison.OrdinalIgnoreCase))
            return;

        CopyTree(Path.Combine(source, "bindings", "dotnet", "src", "LicenseHub.Licensing"),
            Path.Combine(kitRoot, "bindings", "dotnet", "src", "LicenseHub.Licensing"), ["bin", "obj"]);
        CopyTree(Path.Combine(source, "bindings", "python", "licensehub_licensing"),
            Path.Combine(kitRoot, "bindings", "python", "licensehub_licensing"), ["__pycache__"]);
        CopyTree(Path.Combine(source, "bindings", "cpp", "include"),
            Path.Combine(kitRoot, "bindings", "cpp", "include"), []);

        var nodeSource = Path.Combine(source, "bindings", "node");
        var nodeTarget = Path.Combine(kitRoot, "bindings", "node");
        CopyTree(Path.Combine(nodeSource, "src"), Path.Combine(nodeTarget, "src"), []);
        var nodeNative = Path.Combine(nodeTarget, "native", "win-x64");
        Directory.CreateDirectory(nodeNative);
        CopyNativeRuntime(LocateNative(source), Path.Combine(nodeNative, "license_core.dll"));
        CopyTree(Path.Combine(nodeSource, "node_modules", "koffi"), Path.Combine(nodeTarget, "node_modules", "koffi"), []);
        Directory.CreateDirectory(nodeTarget);
        File.Copy(Path.Combine(nodeSource, "package.json"), Path.Combine(nodeTarget, "package.json"));
        File.Copy(Path.Combine(nodeSource, "package-lock.json"), Path.Combine(nodeTarget, "package-lock.json"));

        var coreInclude = Path.Combine(kitRoot, "core", "include");
        Directory.CreateDirectory(coreInclude);
        File.Copy(Path.Combine(source, "core", "include", "license_core.h"), Path.Combine(coreInclude, "license_core.h"));
        var runtime = Path.Combine(kitRoot, "core", "target", "release");
        Directory.CreateDirectory(runtime);
        File.Copy(LocateNative(source), Path.Combine(runtime, "license_core.dll"));
        var importLibrary = LocateImportLibrary(source) ?? throw new FileNotFoundException("license_core import library is absent from the payload");
        File.Copy(importLibrary, Path.Combine(runtime, "license_core.dll.lib"));
    }
    private static IEnumerable<string> Ancestors(string raw)
    {
        var current = new DirectoryInfo(Path.GetFullPath(raw));
        while (current is not null) { yield return current.FullName; current = current.Parent; }
    }

    private static void InstallDotNet(string project, string source, ProductProfile profile, MutationTracker tracker)
    {
        var sourceBinding = Path.Combine(source, "bindings", "dotnet", "src", "LicenseHub.Licensing");
        var vendor = Path.Combine(project, ".licensehub", "vendor", "dotnet");
        CopyTree(sourceBinding, vendor, ["bin", "obj"]);
        var native = LocateNative(source);
        Directory.CreateDirectory(Path.Combine(vendor, "runtimes", "win-x64", "native"));
        File.Copy(native, Path.Combine(vendor, "runtimes", "win-x64", "native", "license_core.dll"), true);
        var vendorProject = Path.Combine(vendor, "LicenseHub.Licensing.csproj");
        Runner.AtomicWrite(vendorProject, """
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup><TargetFramework>net8.0</TargetFramework><Nullable>enable</Nullable><ImplicitUsings>enable</ImplicitUsings></PropertyGroup>
  <ItemGroup><None Include="runtimes/win-x64/native/license_core.dll" CopyToOutputDirectory="PreserveNewest" Link="runtimes/win-x64/native/license_core.dll" /></ItemGroup>
</Project>
""");
        tracker.WriteCreated("LicenseHubIntegration.cs", DotNetAdapter);
        var projectFile = Directory.EnumerateFiles(project, "*.csproj", SearchOption.TopDirectoryOnly).Single();
        var original = File.ReadAllText(projectFile);
        var patch = $"  {BeginMarker}\n  <ItemGroup>\n    <ProjectReference Include=\".licensehub\\vendor\\dotnet\\LicenseHub.Licensing.csproj\" />\n    <None Include=\".licensehub\\product.profile.json\" Link=\"product.profile.json\" CopyToOutputDirectory=\"PreserveNewest\" />\n  </ItemGroup>\n  {EndMarker}\n";
        if (!original.Contains("</Project>", StringComparison.Ordinal)) throw new InvalidDataException(".csproj has no closing Project element");
        tracker.WriteModified(Relative(project, projectFile), original.Replace("</Project>", patch + "</Project>", StringComparison.Ordinal));
    }

    private static void InstallElectron(string project, string source, MutationTracker tracker)
    {
        var vendor = Path.Combine(project, ".licensehub", "vendor", "node");
        CopyTree(Path.Combine(source, "bindings", "node"), vendor, ["test", "dist"]);
        var native = Path.Combine(vendor, "native", "win-x64");
        Directory.CreateDirectory(native);
        CopyNativeRuntime(LocateNative(source), Path.Combine(native, "license_core.dll"));
        var packagePath = Path.Combine(project, "package.json");
        var package = JsonNode.Parse(File.ReadAllText(packagePath))?.AsObject() ?? throw new InvalidDataException("package.json must contain an object");
        var dependencies = package["dependencies"] as JsonObject ?? new JsonObject();
        package["dependencies"] = dependencies;
        if (dependencies.ContainsKey("@licensehub/licensing") && dependencies["@licensehub/licensing"]?.GetValue<string>() != "file:.licensehub/vendor/node")
            throw new InvalidOperationException("package.json already declares a different @licensehub/licensing dependency");
        dependencies["@licensehub/licensing"] = "file:.licensehub/vendor/node";
        tracker.WriteModified("package.json", package.ToJsonString(new JsonSerializerOptions { WriteIndented = true }) + Environment.NewLine);
        tracker.WriteCreated("licensehub-client.cjs", ElectronClient);
        tracker.WriteCreated("licensehub-main.cjs", ElectronMain);
        tracker.WriteCreated("licensehub-preload.cjs", ElectronPreload);
        tracker.WriteCreated("licensehub-renderer.js", ElectronRenderer);
    }

    private static void InstallPython(string project, string source, MutationTracker tracker)
    {
        var package = Path.Combine(project, ".licensehub", "vendor", "python", "licensehub_licensing");
        CopyTree(Path.Combine(source, "bindings", "python", "licensehub_licensing"), package, ["__pycache__"]);
        var native = Path.Combine(package, "_native", "win-x64");
        Directory.CreateDirectory(native);
        // Portable kits may already carry the native runtime inside the Python
        // package. Treat an identical copy as valid, but reject conflicting
        // binaries instead of silently selecting whichever file was copied last.
        CopyNativeRuntime(LocateNative(source), Path.Combine(native, "license_core.dll"));
        tracker.WriteCreated("licensehub_integration.py", PythonAdapter);
    }

    private static void InstallCpp(string project, string source, ProductProfile profile, MutationTracker tracker)
    {
        var vendor = Path.Combine(project, ".licensehub", "vendor", "cpp");
        Directory.CreateDirectory(Path.Combine(vendor, "include", "licensehub"));
        File.Copy(Path.Combine(source, "bindings", "cpp", "include", "licensehub", "license_client.hpp"), Path.Combine(vendor, "include", "licensehub", "license_client.hpp"));
        File.Copy(Path.Combine(source, "core", "include", "license_core.h"), Path.Combine(vendor, "include", "license_core.h"));
        Directory.CreateDirectory(Path.Combine(vendor, "runtime", "win-x64"));
        File.Copy(LocateNative(source), Path.Combine(vendor, "runtime", "win-x64", "license_core.dll"));
        var importLibrary = LocateImportLibrary(source) ?? throw new FileNotFoundException("license_core import library is absent from the payload");
        File.Copy(importLibrary, Path.Combine(vendor, "runtime", "win-x64", "license_core.dll.lib"));
        Runner.AtomicWrite(Path.Combine(vendor, "CMakeLists.txt"), CppCMake);
        tracker.WriteCreated("licensehub_integration.hpp", CppAdapter(profile));

        var cmake = Path.Combine(project, "CMakeLists.txt");
        var original = File.ReadAllText(cmake);
        var target = Regex.Match(original, @"add_executable\s*\(\s*([A-Za-z0-9_.+-]+)", RegexOptions.CultureInvariant).Groups[1].Value;
        if (target.Length == 0) throw new InvalidDataException("CMakeLists.txt needs a literal add_executable target for automatic integration");
        var patch = $"\n# LICENSEHUB:BEGIN\nadd_subdirectory(\"${{CMAKE_CURRENT_LIST_DIR}}/.licensehub/vendor/cpp\" \"${{CMAKE_CURRENT_BINARY_DIR}}/licensehub\")\ntarget_link_libraries({target} PRIVATE LicenseHub::Licensing)\nlicensehub_copy_runtime({target})\n# LICENSEHUB:END\n";
        tracker.WriteModified("CMakeLists.txt", original.TrimEnd() + patch);
    }

    private static DoctorCheck RunStackDoctor(string project, string stack)
    {
        try
        {
            return stack switch
            {
                "dotnet" => RunDoctorProcess("compile", "dotnet", ["build", Directory.EnumerateFiles(project, "*.csproj", SearchOption.TopDirectoryOnly).Single(), "--nologo", "--verbosity", "minimal"], project),
                "electron" => DoctorElectron(project),
                "python" => RunDoctorProcess("import", PythonCommand(), ["-B", "-c", "import licensehub_integration; assert callable(licensehub_integration.create_client)"], project),
                "cpp" => DoctorCpp(project),
                _ => new("stack", false, "unsupported stack")
            };
        }
        catch (Exception error) { return new("compile_import", false, error.Message); }
    }
    private static DoctorCheck DoctorElectron(string project)
    {
        foreach (var file in new[] { "licensehub-client.cjs", "licensehub-main.cjs", "licensehub-preload.cjs", "licensehub-renderer.js" })
        {
            var result = RunDoctorProcess("syntax", "node", ["--check", file], project);
            if (!result.Ok) return result;
        }
        return RunDoctorProcess("import", "node", ["-e", "const x=require('./licensehub-client.cjs'); if(typeof x.createLicenseClient!=='function')process.exit(3)"], project);
    }
    private static DoctorCheck DoctorCpp(string project)
    {
        if (FindExecutable("cmake") is { } cmake)
        {
            var build = Path.Combine(project, ".licensehub", "doctor-build");
            var configure = RunDoctorProcess("configure", cmake, ["-S", project, "-B", build], project);
            if (!configure.Ok) return configure;
            return RunDoctorProcess("compile", cmake, ["--build", build, "--config", "Release"], project);
        }
        if (FindExecutable("cl") is { } cl)
        {
            var source = Directory.EnumerateFiles(project, "*.cpp", SearchOption.TopDirectoryOnly).FirstOrDefault() ?? throw new InvalidDataException("no C++ source file found");
            return RunDoctorProcess("compile", cl, ["/nologo", "/std:c++17", "/EHsc", "/c", source, "/I", Path.Combine(project, ".licensehub", "vendor", "cpp", "include"), "/Fo" + Path.Combine(project, ".licensehub", "doctor.obj")], project);
        }
        return new("compile", false, "neither cmake nor a C++ compiler is installed");
    }
    private static DoctorCheck RunDoctorProcess(string name, string command, IReadOnlyList<string> arguments, string directory)
    {
        var start = new ProcessStartInfo(command) { WorkingDirectory = directory, RedirectStandardOutput = true, RedirectStandardError = true, UseShellExecute = false, CreateNoWindow = true };
        foreach (var argument in arguments) start.ArgumentList.Add(argument);
        using var process = Process.Start(start) ?? throw new InvalidOperationException($"could not start {command}");
        var output = process.StandardOutput.ReadToEndAsync();
        var error = process.StandardError.ReadToEndAsync();
        if (!process.WaitForExit(120_000)) { process.Kill(true); return new(name, false, $"{command} timed out"); }
        Task.WaitAll(output, error);
        var lines = (output.Result + Environment.NewLine + error.Result).Split(['\r', '\n'], StringSplitOptions.RemoveEmptyEntries);
        var detail = string.Join(" ", lines.TakeLast(process.ExitCode == 0 ? 2 : 12)).Trim();
        return new(name, process.ExitCode == 0, detail.Length == 0 ? $"exit={process.ExitCode}" : detail);
    }
    private static string PythonCommand() => FindExecutable("python") ?? FindExecutable("python3") ?? throw new FileNotFoundException("python was not found");
    private static string? FindExecutable(string name)
    {
        var extensions = OperatingSystem.IsWindows() ? new[] { ".exe", ".cmd", ".bat", "" } : new[] { "" };
        foreach (var directory in (Environment.GetEnvironmentVariable("PATH") ?? "").Split(Path.PathSeparator, StringSplitOptions.RemoveEmptyEntries))
            foreach (var extension in extensions)
            {
                var candidate = Path.Combine(directory.Trim('"'), name + extension);
                if (File.Exists(candidate)) return candidate;
            }
        return null;
    }

    private static string LocateNative(string source)
    {
        foreach (var candidate in new[] { Path.Combine(source, "core", "target", "release", "license_core.dll"), Path.Combine(source, "bindings", "node", "native", "win-x64", "license_core.dll") })
            if (File.Exists(candidate)) return candidate;
        throw new FileNotFoundException("license_core.dll is absent from the payload");
    }
    private static void CopyNativeRuntime(string source, string destination)
    {
        if (File.Exists(destination))
        {
            if (!string.Equals(Runner.Sha256File(source), Runner.Sha256File(destination), StringComparison.Ordinal))
                throw new InvalidDataException($"conflicting native runtime at {destination}");
            return;
        }
        File.Copy(source, destination);
    }
    private static string? LocateImportLibrary(string source) => new[] { Path.Combine(source, "core", "target", "release", "license_core.dll.lib"), Path.Combine(source, "core", "target", "release", "license_core.lib") }.FirstOrDefault(File.Exists);
    private static void CopyTree(string source, string destination, IReadOnlyCollection<string> excludedDirectories)
    {
        if (!Directory.Exists(source)) throw new DirectoryNotFoundException($"binding payload is missing: {source}");
        foreach (var directory in Directory.EnumerateDirectories(source, "*", SearchOption.AllDirectories))
        {
            var relative = Path.GetRelativePath(source, directory);
            if (relative.Split(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar).Any(excludedDirectories.Contains)) continue;
            Directory.CreateDirectory(Path.Combine(destination, relative));
        }
        foreach (var file in Directory.EnumerateFiles(source, "*", SearchOption.AllDirectories))
        {
            var relative = Path.GetRelativePath(source, file);
            if (relative.Split(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar).Any(excludedDirectories.Contains)) continue;
            var target = Path.Combine(destination, relative);
            Directory.CreateDirectory(Path.GetDirectoryName(target)!);
            File.Copy(file, target, true);
        }
    }
    private static string Relative(string root, string path) => Path.GetRelativePath(root, path).Replace('\\', '/');
    private static string ResolveInside(string root, string relative)
    {
        var path = Path.GetFullPath(Path.Combine(root, relative.Replace('/', Path.DirectorySeparatorChar)));
        if (!path.StartsWith(root + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new InvalidDataException($"path escapes project root: {relative}");
        return path;
    }
    private static string EscapeCpp(string value) => value.Replace("\\", "\\\\", StringComparison.Ordinal).Replace("\"", "\\\"", StringComparison.Ordinal);
    private static string CppAdapter(ProductProfile profile)
    {
        var keys = string.Join(", ", profile.PublicKeys.Select(pair => $"{{\"{EscapeCpp(pair.Key)}\", \"{EscapeCpp(pair.Value)}\"}}"));
        return $$"""
#pragma once
#include <licensehub/license_client.hpp>
#include <cstdlib>
#include <filesystem>

namespace licensehub_integration {
inline std::filesystem::path cache_directory() {
#ifdef _WIN32
    if (const char* value = std::getenv("LOCALAPPDATA")) return std::filesystem::path(value) / "LicenseHub" / "{{EscapeCpp(profile.ProductId)}}";
#endif
    if (const char* value = std::getenv("XDG_STATE_HOME")) return std::filesystem::path(value) / "licensehub" / "{{EscapeCpp(profile.ProductId)}}";
    if (const char* value = std::getenv("HOME")) return std::filesystem::path(value) / ".local" / "state" / "licensehub" / "{{EscapeCpp(profile.ProductId)}}";
    throw std::runtime_error("no per-user cache directory is available");
}
inline licensehub::client create_client() {
    licensehub::config config{"{{EscapeCpp(profile.ProductId)}}", "{{EscapeCpp(profile.ServerUrl)}}", cache_directory().string(), {{{keys}}}, {{profile.ClockRollbackToleranceSeconds}}, {{profile.RequestTimeoutSeconds}}, {{(profile.AllowInsecureLocalhost ? "true" : "false")}}};
    return licensehub::client(config);
}
// Lifecycle: status -> activate -> refresh -> require_entitlement -> deactivate.
}
""";
    }

    private const string DotNetAdapter = """
using System.Text.Json;
using LicenseHub.Licensing;

public static class LicenseHubIntegration
{
    public static LicenseClient CreateClient(string? profilePath = null)
    {
        profilePath ??= Path.Combine(AppContext.BaseDirectory, "product.profile.json");
        using var document = JsonDocument.Parse(File.ReadAllText(profilePath));
        var profile = document.RootElement;
        var product = profile.GetProperty("product_id").GetString()!;
        return LicenseClient.Initialize(new LicenseClientConfig
        {
            ProductId = product,
            ServerUrl = profile.GetProperty("server_url").GetString()!,
            CacheDirectory = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "LicenseHub", product),
            PublicKeys = profile.GetProperty("public_keys").EnumerateObject().ToDictionary(pair => pair.Name, pair => pair.Value.GetString()!),
            ClockRollbackToleranceSeconds = profile.GetProperty("clock_rollback_tolerance_seconds").GetInt64(),
            RequestTimeoutSeconds = profile.GetProperty("request_timeout_seconds").GetUInt64(),
            AllowInsecureLocalhost = profile.GetProperty("allow_insecure_localhost").GetBoolean()
        });
    }
    // Full lifecycle: Status -> Activate -> Refresh -> RequireEntitlement -> Deactivate.
}
""";
    private const string PythonAdapter = """
from __future__ import annotations
import json
import os
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parent
_VENDOR = _ROOT / ".licensehub" / "vendor" / "python"
if str(_VENDOR) not in sys.path:
    sys.path.insert(0, str(_VENDOR))
from licensehub_licensing import LicenseClient

def create_client(profile_path: str | os.PathLike[str] | None = None) -> LicenseClient:
    path = Path(profile_path) if profile_path else _ROOT / ".licensehub" / "product.profile.json"
    profile = json.loads(path.read_text(encoding="utf-8"))
    profile["cache_dir"] = str(Path(os.getenv("LOCALAPPDATA", Path.home())) / "LicenseHub" / profile["product_id"])
    profile.pop("key_fingerprints_sha256", None)
    profile.pop("schema_version", None)
    return LicenseClient.initialize(profile)

# Full lifecycle: status -> activate -> refresh -> require_entitlement -> deactivate.
""";
    private const string ElectronClient = """
"use strict";
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
// Resolve the immutable vendored package directly. The package.json file: dependency
// remains for normal npm tooling, while first launch does not require a registry.
const { LicenseClient } = require("./.licensehub/vendor/node");
function createLicenseClient(profilePath = path.join(__dirname, ".licensehub", "product.profile.json")) {
  const profile = JSON.parse(fs.readFileSync(profilePath, "utf8"));
  const { schema_version, key_fingerprints_sha256, ...config } = profile;
  return LicenseClient.initialize({ ...config, cache_dir: path.join(os.homedir(), ".licensehub", profile.product_id) });
}
module.exports = { createLicenseClient };
""";
    private const string ElectronMain = """
"use strict";
const { ipcMain } = require("electron");
const { createLicenseClient } = require("./licensehub-client.cjs");
function registerLicenseHubIpc(client = createLicenseClient()) {
  const methods = {
    "licensehub:status": () => client.status(),
    "licensehub:activate": value => client.activate(requiredText(value, "activation", 4096)),
    "licensehub:refresh": () => client.refresh(),
    "licensehub:require-entitlement": value => { client.requireEntitlement(requiredText(value, "entitlement", 128)); return true; },
    "licensehub:deactivate": () => { client.deactivate(); return true; }
  };
  for (const [channel, action] of Object.entries(methods)) ipcMain.handle(channel, (_event, value) => action(value));
  return () => { for (const channel of Object.keys(methods)) ipcMain.removeHandler(channel); client.close(); };
}
function requiredText(value, name, maximum) {
  if (typeof value !== "string" || !value.trim() || value.length > maximum) throw new TypeError(`${name} is invalid`);
  return value;
}
module.exports = { registerLicenseHubIpc };
""";
    private const string ElectronPreload = """
"use strict";
const { contextBridge, ipcRenderer } = require("electron");
contextBridge.exposeInMainWorld("licenseHub", Object.freeze({
  status: () => ipcRenderer.invoke("licensehub:status"),
  activate: value => ipcRenderer.invoke("licensehub:activate", String(value)),
  refresh: () => ipcRenderer.invoke("licensehub:refresh"),
  requireEntitlement: value => ipcRenderer.invoke("licensehub:require-entitlement", String(value)),
  deactivate: () => ipcRenderer.invoke("licensehub:deactivate")
}));
""";
    private const string ElectronRenderer = """
export const license = Object.freeze({
  status: () => window.licenseHub.status(),
  activate: value => window.licenseHub.activate(value),
  refresh: () => window.licenseHub.refresh(),
  requireEntitlement: name => window.licenseHub.requireEntitlement(name),
  deactivate: () => window.licenseHub.deactivate()
});
""";
    private const string CppCMake = """
cmake_minimum_required(VERSION 3.20)
add_library(licensehub_core SHARED IMPORTED GLOBAL)
if(WIN32)
  set_target_properties(licensehub_core PROPERTIES
    IMPORTED_LOCATION "${CMAKE_CURRENT_LIST_DIR}/runtime/win-x64/license_core.dll"
    IMPORTED_IMPLIB "${CMAKE_CURRENT_LIST_DIR}/runtime/win-x64/license_core.dll.lib")
else()
  message(FATAL_ERROR "This kit currently contains only the win-x64 native runtime")
endif()
add_library(licensehub_licensing INTERFACE)
add_library(LicenseHub::Licensing ALIAS licensehub_licensing)
target_compile_features(licensehub_licensing INTERFACE cxx_std_17)
target_include_directories(licensehub_licensing INTERFACE "${CMAKE_CURRENT_LIST_DIR}/include")
target_link_libraries(licensehub_licensing INTERFACE licensehub_core)
function(licensehub_copy_runtime target)
  add_custom_command(TARGET ${target} POST_BUILD COMMAND ${CMAKE_COMMAND} -E copy_if_different
    "${CMAKE_CURRENT_FUNCTION_LIST_DIR}/runtime/win-x64/license_core.dll" "$<TARGET_FILE_DIR:${target}>")
endfunction()
""";

    private sealed class MutationTracker
    {
        private readonly string _project;
        private readonly List<InstallFile> _files = [];
        internal IReadOnlyList<InstallFile> Files => _files;
        internal MutationTracker(string project) => _project = project;
        internal void CopyCreated(string source, string relative)
        {
            var target = ResolveInside(_project, relative);
            if (File.Exists(target)) throw new IOException($"refusing to overwrite {relative}");
            Directory.CreateDirectory(Path.GetDirectoryName(target)!);
            File.Copy(source, target);
            _files.Add(new(relative, "created", Runner.Sha256File(target), null, null));
        }
        internal void WriteCreated(string relative, string content)
        {
            var target = ResolveInside(_project, relative);
            if (File.Exists(target)) throw new IOException($"refusing to overwrite {relative}");
            Runner.AtomicWrite(target, content);
            _files.Add(new(relative, "created", Runner.Sha256File(target), null, null));
        }
        internal void WriteModified(string relative, string content)
        {
            var target = ResolveInside(_project, relative);
            var original = Runner.Sha256File(target);
            var backupRelative = $".licensehub/rollback/{_files.Count:D4}.bak";
            var backup = ResolveInside(_project, backupRelative);
            Directory.CreateDirectory(Path.GetDirectoryName(backup)!);
            File.Copy(target, backup);
            Runner.AtomicWrite(target, content);
            _files.Add(new(relative, "modified", Runner.Sha256File(target), original, backupRelative));
        }
        internal void Rollback()
        {
            foreach (var file in _files.AsEnumerable().Reverse())
            {
                var target = ResolveInside(_project, file.Path);
                if (file.Kind == "modified" && file.Backup is not null && File.Exists(ResolveInside(_project, file.Backup))) File.Copy(ResolveInside(_project, file.Backup), target, true);
                else if (file.Kind == "created" && File.Exists(target)) File.Delete(target);
            }
        }
    }

    private sealed class KitSource : IDisposable
    {
        internal string Root { get; }
        private readonly string? _temporary;
        private KitSource(string root, string? temporary) { Root = root; _temporary = temporary; }
        internal static KitSource Open(string raw)
        {
            var path = Path.GetFullPath(raw);
            if (Directory.Exists(path)) return new(path, null);
            if (!File.Exists(path) || !path.EndsWith(".zip", StringComparison.OrdinalIgnoreCase)) throw new FileNotFoundException("--kit must be a directory or zip file", path);
            var temporary = Path.Combine(Path.GetTempPath(), "licctl-kit-" + Guid.NewGuid().ToString("N"));
            Directory.CreateDirectory(temporary);
            using var archive = ZipFile.OpenRead(path);
            foreach (var entry in archive.Entries)
            {
                var destination = Path.GetFullPath(Path.Combine(temporary, entry.FullName));
                if (!destination.StartsWith(temporary + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new InvalidDataException("kit zip contains a path traversal entry");
                if (entry.FullName.EndsWith('/') || entry.FullName.EndsWith('\\')) Directory.CreateDirectory(destination);
                else { Directory.CreateDirectory(Path.GetDirectoryName(destination)!); entry.ExtractToFile(destination); }
            }
            var profile = Directory.EnumerateFiles(temporary, "product.profile.json", SearchOption.AllDirectories).ToArray();
            if (profile.Length != 1) { Directory.Delete(temporary, true); throw new InvalidDataException("kit zip must contain exactly one product.profile.json"); }
            return new(Path.GetDirectoryName(profile[0])!, temporary);
        }
        public void Dispose() { if (_temporary is not null && Directory.Exists(_temporary)) Directory.Delete(_temporary, true); }
    }
}

internal sealed record InstallRecord(
    [property: JsonPropertyName("schema_version")] int SchemaVersion,
    [property: JsonPropertyName("stack")] string Stack,
    [property: JsonPropertyName("product_id")] string ProductId,
    [property: JsonPropertyName("profile_sha256")] string ProfileSha256,
    [property: JsonPropertyName("installed_at")] DateTimeOffset InstalledAt,
    [property: JsonPropertyName("files")] IReadOnlyList<InstallFile> Files);
internal sealed record InstallFile(
    [property: JsonPropertyName("path")] string Path,
    [property: JsonPropertyName("kind")] string Kind,
    [property: JsonPropertyName("installed_sha256")] string InstalledSha256,
    [property: JsonPropertyName("original_sha256")] string? OriginalSha256,
    [property: JsonPropertyName("backup")] string? Backup);
internal sealed record DoctorCheck(
    [property: JsonPropertyName("name")] string Name,
    [property: JsonPropertyName("ok")] bool Ok,
    [property: JsonPropertyName("detail")] string Detail);
