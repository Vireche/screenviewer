using Microsoft.UI.Xaml.Media.Imaging;
using ScreenViewer.WinUI3.Models;
using Windows.Storage.Streams;
using System.Runtime.InteropServices.WindowsRuntime;

namespace ScreenViewer.WinUI3.Services;

public sealed class ImageFactory
{
    public async Task<ImageEntry> CreateFromBytesAsync(byte[] bytes, string fileName, string? sourceLabel = null)
    {
        var source = await CreateBitmapImageAsync(bytes);
        return new ImageEntry
        {
            Title = Path.GetFileName(fileName),
            FilePath = fileName,
            SourceLabel = sourceLabel,
            Content = bytes,
            Source = source,
        };
    }

    public async Task<ImageEntry> CreateFromFileAsync(string path, string? sourceLabel = null)
    {
        var bytes = await File.ReadAllBytesAsync(path);
        return await CreateFromBytesAsync(bytes, path, sourceLabel);
    }

    public static async Task<BitmapImage> CreateBitmapImageAsync(byte[] bytes)
    {
        using var stream = new InMemoryRandomAccessStream();
        await stream.WriteAsync(bytes.AsBuffer());

        stream.Seek(0);
        var image = new BitmapImage();
        await image.SetSourceAsync(stream);
        return image;
    }
}
