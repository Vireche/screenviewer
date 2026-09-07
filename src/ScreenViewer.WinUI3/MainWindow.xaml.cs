using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Dispatching;
using ScreenViewer.WinUI3.Models;
using ScreenViewer.WinUI3.Services;
using ScreenViewer.WinUI3.ViewModels;
using ScreenViewer.WinUI3.Views;
using Windows.ApplicationModel.DataTransfer;
using Windows.Storage;
using Windows.Storage.Pickers;
using Windows.UI.Core;
using WinRT.Interop;
using Microsoft.UI.Input;
using Microsoft.UI.Xaml.Media;
using Rectangle = System.Drawing.Rectangle;

namespace ScreenViewer.WinUI3;

public sealed partial class MainWindow : Window
{
    private const int MaxImages = 9;
    private const int UploadPort = 8765;

    private readonly MainViewModel viewModel = new();
    private readonly ImageFactory imageFactory = new();
    private readonly ImageLibraryService imageLibraryService = new();
    private readonly ScreenCaptureService screenCaptureService = new();
    private readonly ClipboardImageService clipboardImageService;
    private readonly LocalUploadServer uploadServer = new(UploadPort);
    private readonly List<string> browserFiles = new();
    private readonly DispatcherQueueTimer previewTimer;

    private FullscreenImageWindow? fullscreenWindow;
    private bool refreshingPreview;
    private bool browserPanelVisible;

    public MainWindow()
    {
        InitializeComponent();
        RootGrid.DataContext = viewModel;
        Closed += MainWindow_Closed;
        clipboardImageService = new ClipboardImageService(imageFactory);
        previewTimer = DispatcherQueue.CreateTimer();
        previewTimer.Interval = TimeSpan.FromMilliseconds(500);
        previewTimer.Tick += async (_, _) => await RefreshPreviewAsync();
        viewModel.ActiveImages.CollectionChanged += (_, _) => UpdatePresentation();
    }

    private async void RootGrid_Loaded(object sender, RoutedEventArgs e)
    {
        try
        {
            WindowingService.MoveAndResize(this, new Rectangle(100, 100, 1280, 760));
        }
        catch
        {
            // Some machines throw COM E_FAIL if the app window is not fully ready yet.
        }

        try
        {
            await uploadServer.StartAsync();
            uploadServer.ImageReceivedAsync = HandleIncomingUploadAsync;
        }
        catch (Exception exception)
        {
            viewModel.StatusText = $"Upload server failed to start: {exception.Message}";
            StatusTextBlock.Text = viewModel.StatusText;
        }

        RefreshDisplays();
        SetBrowserPanelVisible(false);
        previewTimer.Start();
        await RefreshPreviewAsync();
        UpdateStatus();
    }

    private void RefreshDisplays()
    {
        var displays = screenCaptureService.GetDisplays();
        viewModel.Displays.Clear();
        foreach (var display in displays)
        {
            viewModel.Displays.Add(display);
        }

        if (viewModel.Displays.Count == 0)
        {
            viewModel.StatusText = "No displays were detected.";
            return;
        }

        DisplayComboBox.SelectedIndex = viewModel.Displays.Count > 1 ? 1 : 0;
    }

    private async Task RefreshPreviewAsync()
    {
        if (refreshingPreview || viewModel.SelectedDisplay is null)
        {
            return;
        }

        refreshingPreview = true;
        try
        {
            var preview = await screenCaptureService.CaptureDisplayAsync(viewModel.SelectedDisplay);
            viewModel.PreviewImageSource = preview;
            PreviewImage.Source = preview;
            UpdateStatus();
        }
        finally
        {
            refreshingPreview = false;
        }
    }

    private void UpdateStatus()
    {
        var displayLabel = viewModel.SelectedDisplay?.Label ?? "No display selected";
        var imageCount = viewModel.ActiveImages.Count;
        viewModel.StatusText = $"{displayLabel} | {imageCount} active image{(imageCount == 1 ? string.Empty : "s")}";
        StatusTextBlock.Text = viewModel.StatusText;
    }

    private async Task<ImageEntry?> CreateImageFromPathAsync(string path, string? sourceLabel)
    {
        if (!ImageLibraryService.IsSupportedImage(path))
        {
            return null;
        }

        try
        {
            return await imageFactory.CreateFromFileAsync(path, sourceLabel);
        }
        catch
        {
            return null;
        }
    }

    private async Task ShowImageEntryAsync(ImageEntry? imageEntry)
    {
        if (imageEntry is null)
        {
            return;
        }

        if (!viewModel.AllowMultipleImages)
        {
            viewModel.ActiveImages.Clear();
        }
        else
        {
            var existing = viewModel.ActiveImages.FirstOrDefault(entry => string.Equals(entry.FilePath, imageEntry.FilePath, StringComparison.OrdinalIgnoreCase));
            if (existing is not null)
            {
                viewModel.ActiveImages.Remove(existing);
            }
        }

        while (viewModel.ActiveImages.Count >= MaxImages)
        {
            viewModel.ActiveImages.RemoveAt(0);
        }

        viewModel.ActiveImages.Add(imageEntry);
        TrimActiveImagesToLimit();
        await EnsureFullscreenWindowAsync();
        UpdatePresentation();
    }

    private async Task ShowImageFromPathAsync(string path, string? sourceLabel)
    {
        var imageEntry = await CreateImageFromPathAsync(path, sourceLabel);
        await ShowImageEntryAsync(imageEntry);
    }

    private async Task ShowImagesFromPathsAsync(IEnumerable<string> paths, string? sourceLabel)
    {
        foreach (var path in paths)
        {
            await ShowImageFromPathAsync(path, sourceLabel);
        }
    }

    private async Task ShowClipboardImageAsync()
    {
        var imageEntry = await clipboardImageService.TryGetImageAsync();
        await ShowImageEntryAsync(imageEntry);
    }

    private async Task HandleIncomingUploadAsync(byte[] bytes, string? fileName, string? sourceLabel)
    {
        try
        {
            var imageEntry = await imageFactory.CreateFromBytesAsync(bytes, fileName ?? $"upload-{DateTimeOffset.Now:yyyyMMdd-HHmmss}.png", sourceLabel ?? "Chrome extension");
            await DispatcherQueue.EnqueueAsync(() => ShowImageEntryAsync(imageEntry));
        }
        catch
        {
        }
    }

    private async Task EnsureFullscreenWindowAsync()
    {
        if (viewModel.ActiveImages.Count == 0)
        {
            fullscreenWindow?.HideWindow();
            fullscreenWindow = null;
            return;
        }

        if (fullscreenWindow is null)
        {
            fullscreenWindow = new FullscreenImageWindow();
        }

        var display = viewModel.SelectedDisplay ?? viewModel.Displays.FirstOrDefault();
        if (display is null)
        {
            return;
        }

        fullscreenWindow.ShowImages(viewModel.ActiveImages.ToList(), display.Bounds, viewModel.AllowMultipleImages);
        await Task.CompletedTask;
    }

    private void TrimActiveImagesToLimit()
    {
        while (viewModel.ActiveImages.Count > MaxImages)
        {
            viewModel.ActiveImages.RemoveAt(0);
        }
    }

    private void UpdatePresentation()
    {
        _ = DispatcherQueue.EnqueueAsync(async () =>
        {
            RenderMainActiveImages();
            await EnsureFullscreenWindowAsync();
            UpdateStatus();
        });
    }

    private void RenderMainActiveImages()
    {
        ActiveImagesHost.Children.Clear();
        ActiveImagesHost.RowDefinitions.Clear();
        ActiveImagesHost.ColumnDefinitions.Clear();

        var count = viewModel.ActiveImages.Count;
        if (count == 0)
        {
            ActiveImagesHost.Visibility = Visibility.Collapsed;
            PreviewImage.Visibility = Visibility.Visible;
            return;
        }

        ActiveImagesHost.Visibility = Visibility.Visible;
        PreviewImage.Visibility = Visibility.Collapsed;

        var (rows, columns) = GetTileGridSize(count);
        for (var row = 0; row < rows; row++)
        {
            ActiveImagesHost.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        }

        for (var column = 0; column < columns; column++)
        {
            ActiveImagesHost.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        }

        for (var index = 0; index < count; index++)
        {
            var imageEntry = viewModel.ActiveImages[index];
            var imageGrid = new Grid();
            imageGrid.Children.Add(new Image
            {
                Source = imageEntry.Source,
                Stretch = Stretch.Uniform,
                HorizontalAlignment = HorizontalAlignment.Stretch,
                VerticalAlignment = VerticalAlignment.Stretch,
            });

            var closeButton = new Button
            {
                Width = 28,
                Height = 28,
                Margin = new Thickness(0, 0, 1, 0),
                Padding = new Thickness(0),
                HorizontalAlignment = HorizontalAlignment.Right,
                VerticalAlignment = VerticalAlignment.Top,
                HorizontalContentAlignment = HorizontalAlignment.Center,
                VerticalContentAlignment = VerticalAlignment.Center,
                Background = new SolidColorBrush(Microsoft.UI.ColorHelper.FromArgb(255, 214, 58, 58)),
                BorderBrush = new SolidColorBrush(Microsoft.UI.Colors.White),
                BorderThickness = new Thickness(1),
                Tag = imageEntry,
                Content = new TextBlock
                {
                    Text = "X",
                    Foreground = new SolidColorBrush(Microsoft.UI.Colors.White),
                    FontWeight = Microsoft.UI.Text.FontWeights.Bold,
                    FontSize = 14,
                    HorizontalAlignment = HorizontalAlignment.Center,
                    VerticalAlignment = VerticalAlignment.Center,
                }
            };

            closeButton.Click += RemoveActiveImage_Click;
            imageGrid.Children.Add(closeButton);

            var tile = new Border
            {
                Margin = new Thickness(6),
                Background = new SolidColorBrush(Microsoft.UI.ColorHelper.FromArgb(255, 15, 18, 22)),
                BorderBrush = new SolidColorBrush(Microsoft.UI.ColorHelper.FromArgb(255, 43, 49, 56)),
                BorderThickness = new Thickness(1),
                CornerRadius = new CornerRadius(10),
                Padding = new Thickness(10),
                Child = imageGrid,
            };

            Grid.SetRow(tile, index / columns);
            Grid.SetColumn(tile, index % columns);
            ActiveImagesHost.Children.Add(tile);
        }
    }

    private static (int Rows, int Columns) GetTileGridSize(int count)
    {
        return count switch
        {
            <= 1 => (1, 1),
            2 => (1, 2),
            <= 4 => (2, 2),
            _ => (3, 3),
        };
    }

    private void SetBrowserPanelVisible(bool isVisible)
    {
        browserPanelVisible = isVisible;
        BrowserPanel.Visibility = isVisible ? Visibility.Visible : Visibility.Collapsed;
        BrowserColumn.Width = isVisible ? new GridLength(280) : new GridLength(0);
    }

    private async Task RefreshBrowserImagesAsync()
    {
        viewModel.BrowserImages.Clear();

        foreach (var path in browserFiles)
        {
            var fileName = Path.GetFileName(path);
            var filter = FilterBox.Text?.Trim();
            if (!string.IsNullOrWhiteSpace(filter) && !fileName.Contains(filter, StringComparison.OrdinalIgnoreCase))
            {
                continue;
            }

            var imageEntry = await CreateImageFromPathAsync(path, "Folder browser");
            if (imageEntry is not null)
            {
                viewModel.BrowserImages.Add(imageEntry);
            }
        }
    }

    private async void BrowseFolder_Click(object sender, RoutedEventArgs e)
    {
        var picker = new FolderPicker();
        picker.FileTypeFilter.Add("*");
        picker.SuggestedStartLocation = PickerLocationId.PicturesLibrary;
        InitializeWithWindow.Initialize(picker, WindowNative.GetWindowHandle(this));

        var folder = await picker.PickSingleFolderAsync();
        if (folder is null)
        {
            return;
        }

        BrowserPathBox.Text = folder.Path;
        browserFiles.Clear();
        browserFiles.AddRange(imageLibraryService.EnumerateImageFiles(folder.Path));
        SetBrowserPanelVisible(true);
        await RefreshBrowserImagesAsync();
        viewModel.StatusText = $"Loaded {viewModel.BrowserImages.Count} images from {folder.Path}";
        StatusTextBlock.Text = viewModel.StatusText;
    }

    private async void FilterBox_TextChanged(object sender, TextChangedEventArgs e)
    {
        if (browserFiles.Count == 0)
        {
            return;
        }

        await RefreshBrowserImagesAsync();
    }

    private async void BrowserListView_ItemClick(object sender, ItemClickEventArgs e)
    {
        if (e.ClickedItem is ImageEntry imageEntry && !string.IsNullOrWhiteSpace(imageEntry.FilePath))
        {
            await ShowImageFromPathAsync(imageEntry.FilePath, imageEntry.SourceLabel);
        }
    }

    private async void Paste_Click(object sender, RoutedEventArgs e)
    {
        await ShowClipboardImageAsync();
    }

    private async void RootGrid_Drop(object sender, DragEventArgs e)
    {
        if (!e.DataView.Contains(StandardDataFormats.StorageItems))
        {
            return;
        }

        var storageItems = await e.DataView.GetStorageItemsAsync();
        var filePaths = storageItems.OfType<StorageFile>()
            .Select(item => item.Path)
            .Where(ImageLibraryService.IsSupportedImage)
            .ToList();

        await ShowImagesFromPathsAsync(filePaths, "Drag and drop");
    }

    private void RootGrid_DragOver(object sender, DragEventArgs e)
    {
        e.AcceptedOperation = DataPackageOperation.Copy;
    }

    private async void RootGrid_KeyDown(object sender, KeyRoutedEventArgs e)
    {
        if (e.Key != Windows.System.VirtualKey.V)
        {
            return;
        }

        var focused = FocusManager.GetFocusedElement(this.Content.XamlRoot);
        if (focused is TextBox)
        {
            return;
        }

        var controlState = InputKeyboardSource.GetKeyStateForCurrentThread(Windows.System.VirtualKey.Control);
        if ((controlState & CoreVirtualKeyStates.Down) == 0)
        {
            return;
        }

        e.Handled = true;
        await ShowClipboardImageAsync();
    }

    private async void DisplayComboBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (DisplayComboBox.SelectedItem is DisplayOption display)
        {
            viewModel.SelectedDisplay = display;
            await RefreshPreviewAsync();
            UpdateStatus();
        }
    }

    private void CloseBrowserPanel_Click(object sender, RoutedEventArgs e)
    {
        SetBrowserPanelVisible(false);
    }

    private void AllowMultipleMenuItem_Click(object sender, RoutedEventArgs e)
    {
        var isChecked = AllowMultipleMenuItem.IsChecked == true;
        viewModel.AllowMultipleImages = isChecked;
        if (!isChecked)
        {
            while (viewModel.ActiveImages.Count > 1)
            {
                viewModel.ActiveImages.RemoveAt(0);
            }
        }

        UpdatePresentation();
    }

    private async void RemoveActiveImage_Click(object sender, RoutedEventArgs e)
    {
        if (sender is not Button button || button.Tag is not ImageEntry imageEntry)
        {
            return;
        }

        viewModel.ActiveImages.Remove(imageEntry);
        TrimActiveImagesToLimit();
        await EnsureFullscreenWindowAsync();
        UpdateStatus();
    }

    private void MainWindow_Closed(object sender, WindowEventArgs args)
    {
        try
        {
            fullscreenWindow?.HideWindow();
        }
        catch
        {
        }

        fullscreenWindow = null;
    }
}
