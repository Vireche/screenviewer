using Microsoft.UI.Xaml.Media.Imaging;

namespace ScreenViewer.WinUI3.Models;

public sealed class ImageEntry
{
    public string Title { get; init; } = string.Empty;

    public string? FilePath { get; init; }

    public string? SourceLabel { get; init; }

    public byte[] Content { get; init; } = [];

    public BitmapImage? Source { get; init; }

    public override string ToString()
    {
        return Title;
    }
}
