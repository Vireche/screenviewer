using Microsoft.UI.Xaml.Media.Imaging;
using ScreenViewer.WinUI3.Models;
using Windows.Storage.Streams;

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
        using (var writer = new DataWriter(stream))
        {
            writer.WriteBytes(bytes);
            await writer.StoreAsync();
            await writer.FlushAsync();
        }

        stream.Seek(0);
        var image = new BitmapImage();
        await image.SetSourceAsync(stream);
        return image;
    }
}
