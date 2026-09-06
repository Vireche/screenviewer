using System.Runtime.InteropServices;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Windows.Graphics;
using WinRT.Interop;

namespace ScreenViewer.WinUI3.Services;

public static class WindowingService
{
    private static readonly IntPtr HwndTopmost = new(-1);
    private static readonly IntPtr HwndNotTopmost = new(-2);

    private const uint SwpNomove = 0x0002;
    private const uint SwpNosize = 0x0001;
    private const uint SwpNoactivate = 0x0010;

    public static void ApplyBorderless(Window window)
    {
        _ = GetAppWindow(window);
    }

    public static void MoveAndResize(Window window, System.Drawing.Rectangle bounds)
    {
        var appWindow = GetAppWindow(window);
        appWindow.MoveAndResize(new RectInt32(bounds.Left, bounds.Top, bounds.Width, bounds.Height));
    }

    public static void SetTopMost(Window window, bool isTopMost)
    {
        var hwnd = WindowNative.GetWindowHandle(window);
        SetWindowPos(hwnd, isTopMost ? HwndTopmost : HwndNotTopmost, 0, 0, 0, 0, SwpNomove | SwpNosize | SwpNoactivate);
    }

    public static AppWindow GetAppWindow(Window window)
    {
        var hwnd = WindowNative.GetWindowHandle(window);
        var windowId = Microsoft.UI.Win32Interop.GetWindowIdFromWindow(hwnd);
        return AppWindow.GetFromWindowId(windowId);
    }

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool SetWindowPos(IntPtr hwnd, IntPtr hwndInsertAfter, int x, int y, int cx, int cy, uint flags);
}
