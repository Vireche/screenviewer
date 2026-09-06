using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
using Microsoft.UI.Xaml.Media.Imaging;
using ScreenViewer.WinUI3.Models;

namespace ScreenViewer.WinUI3.Services;

public sealed class ScreenCaptureService
{
    public IReadOnlyList<DisplayOption> GetDisplays()
    {
        var displays = new List<DisplayOption>();

        EnumDisplayMonitors(IntPtr.Zero, IntPtr.Zero, (monitorHandle, _, _, _) =>
        {
            var monitorInfo = new MonitorInfoEx
            {
                cbSize = Marshal.SizeOf<MonitorInfoEx>()
            };

            if (GetMonitorInfo(monitorHandle, ref monitorInfo))
            {
                var bounds = Rectangle.FromLTRB(monitorInfo.rcMonitor.Left, monitorInfo.rcMonitor.Top, monitorInfo.rcMonitor.Right, monitorInfo.rcMonitor.Bottom);
                displays.Add(new DisplayOption(displays.Count, monitorInfo.szDevice.TrimEnd('\0'), bounds));
            }

            return true;
        }, IntPtr.Zero);

        return displays;
    }

    public async Task<BitmapImage?> CaptureDisplayAsync(DisplayOption display)
    {
        try
        {
            var bytes = await Task.Run(() => CaptureBytes(display));
            return await ImageFactory.CreateBitmapImageAsync(bytes);
        }
        catch
        {
            return null;
        }
    }

    private static byte[] CaptureBytes(DisplayOption display)
    {
        using var bitmap = new Bitmap(display.Bounds.Width, display.Bounds.Height, PixelFormat.Format32bppArgb);
        using (var graphics = Graphics.FromImage(bitmap))
        {
            graphics.CopyFromScreen(display.Bounds.Left, display.Bounds.Top, 0, 0, display.Bounds.Size, CopyPixelOperation.SourceCopy);
        }

        using var stream = new MemoryStream();
        bitmap.Save(stream, ImageFormat.Png);
        return stream.ToArray();
    }

    private delegate bool MonitorEnumProc(IntPtr monitor, IntPtr hdc, IntPtr clipRect, IntPtr data);

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct MonitorInfoEx
    {
        public int cbSize;
        public Rect rcMonitor;
        public Rect rcWork;
        public uint dwFlags;

        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)]
        public string szDevice;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct Rect
    {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }

    [DllImport("user32.dll")]
    private static extern bool EnumDisplayMonitors(IntPtr hdc, IntPtr clipRect, MonitorEnumProc callback, IntPtr data);

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern bool GetMonitorInfo(IntPtr hMonitor, ref MonitorInfoEx monitorInfo);
}
