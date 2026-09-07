using System.Drawing;

namespace ScreenViewer.WinUI3.Models;

public sealed record DisplayOption(int Index, string Name, Rectangle Bounds)
{
    public string Label => $"Display {Index + 1} ({Bounds.Width}x{Bounds.Height})";
}
