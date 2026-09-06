namespace ScreenViewer.WinUI3.Services;

public sealed class ImageLibraryService
{
    private static readonly HashSet<string> SupportedExtensions = new(StringComparer.OrdinalIgnoreCase)
    {
        ".png",
        ".jpg",
        ".jpeg",
        ".gif",
        ".bmp",
        ".webp",
    };

    public IEnumerable<string> EnumerateImageFiles(string folderPath)
    {
        if (!Directory.Exists(folderPath))
        {
            return Enumerable.Empty<string>();
        }

        return Directory.EnumerateFiles(folderPath)
            .Where(IsSupportedImage);
    }

    public static bool IsSupportedImage(string filePath)
    {
        return SupportedExtensions.Contains(Path.GetExtension(filePath));
    }
}
