using System.Reflection;
using System.Runtime.InteropServices;

namespace LicenseHub.Licensing;

internal static class NativeMethods
{
    internal const string LibraryName = "license_core";
    internal const uint ExpectedAbiVersion = 1;

    static NativeMethods() => NativeLibrary.SetDllImportResolver(typeof(NativeMethods).Assembly, ResolveLibrary);
    internal static void EnsureLoaded() => _ = license_abi_version();

    private static nint ResolveLibrary(string libraryName, Assembly assembly, DllImportSearchPath? searchPath)
    {
        if (libraryName != LibraryName) return nint.Zero;
        var file = OperatingSystem.IsWindows() ? "license_core.dll" : OperatingSystem.IsMacOS() ? "liblicense_core.dylib" : "liblicense_core.so";
        var candidates = new[]
        {
            Path.Combine(AppContext.BaseDirectory, "runtimes", RuntimeInformation.RuntimeIdentifier, "native", file),
            Path.Combine(AppContext.BaseDirectory, "runtimes", "win-x64", "native", file),
            Path.Combine(AppContext.BaseDirectory, file)
        };
        foreach (var candidate in candidates)
            if (File.Exists(candidate)) return NativeLibrary.Load(Path.GetFullPath(candidate));
        return nint.Zero;
    }

    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern uint license_abi_version();
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern int license_initialize([MarshalAs(UnmanagedType.LPUTF8Str)] string configJson, out ulong handle);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern int license_shutdown(ulong handle);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern int license_activate(LicenseSafeHandle handle, [MarshalAs(UnmanagedType.LPUTF8Str)] string value);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern nint license_status(LicenseSafeHandle handle, [Out] byte[]? buffer, nuint bufferLength);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern int license_require_entitlement(LicenseSafeHandle handle, [MarshalAs(UnmanagedType.LPUTF8Str)] string entitlement);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern int license_refresh(LicenseSafeHandle handle);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern int license_deactivate(LicenseSafeHandle handle);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern nint license_device_id(LicenseSafeHandle handle, [Out] byte[]? buffer, nuint bufferLength);
    [DllImport(LibraryName, CallingConvention = CallingConvention.Cdecl)] internal static extern nint license_last_error([Out] byte[]? buffer, nuint bufferLength);
}
