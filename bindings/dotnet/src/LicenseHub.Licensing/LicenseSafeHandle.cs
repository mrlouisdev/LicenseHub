using Microsoft.Win32.SafeHandles;

namespace LicenseHub.Licensing;

internal sealed class LicenseSafeHandle : SafeHandleZeroOrMinusOneIsInvalid
{
    internal LicenseSafeHandle(ulong value) : base(true) => SetHandle((nint)value);
    protected override bool ReleaseHandle() => NativeMethods.license_shutdown((ulong)handle.ToInt64()) >= 0;
}
