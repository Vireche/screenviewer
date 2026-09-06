using System.Runtime.InteropServices.WindowsRuntime;
using ScreenViewer.WinUI3.Models;
using Windows.ApplicationModel.DataTransfer;
using Windows.Storage.Streams;

namespace ScreenViewer.WinUI3.Services;

public sealed class ClipboardImageService
{
    private readonly ImageFactory imageFactory;

    public ClipboardImageService(ImageFactory imageFactory)
    {
        this.imageFactory = imageFactory;
    }

    public async Task<ImageEntry?> TryGetImageAsync()
    {
        var dataPackage = Clipboard.GetContent();
        if (dataPackage is null || !dataPackage.Contains(StandardDataFormats.Bitmap))
        {
            return null;
        }

        var bitmapReference = await dataPackage.GetBitmapAsync();
        using var inputStream = await bitmapReference.OpenReadAsync();
        using var reader = new DataReader(inputStream);
        var bytes = new byte[inputStream.Size];
        await reader.LoadAsync((uint)inputStream.Size);
        reader.ReadBytes(bytes);

        return await imageFactory.CreateFromBytesAsync(bytes, $"clipboard-{DateTimeOffset.Now:yyyyMMdd-HHmmss}.png", "Clipboard");
    }
}
