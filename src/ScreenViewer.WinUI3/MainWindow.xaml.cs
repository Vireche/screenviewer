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
using WinRT.Interop;
using System.Drawing;

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
        clipboardImageService = new ClipboardImageService(imageFactory);
        previewTimer = DispatcherQueue.CreateTimer();
        previewTimer.Interval = TimeSpan.FromMilliseconds(500);
        previewTimer.Tick += async (_, _) => await RefreshPreviewAsync();
        viewModel.ActiveImages.CollectionChanged += (_, _) => UpdatePresentation();
    }

    private async void RootGrid_Loaded(object sender, RoutedEventArgs e)
    {
        WindowingService.MoveAndResize(this, new Rectangle(100, 100, 1280, 760));
        await uploadServer.StartAsync();
        uploadServer.ImageReceivedAsync = HandleIncomingUploadAsync;

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

        fullscreenWindow?.HideWindow();
        fullscreenWindow = new FullscreenImageWindow();
        var display = viewModel.SelectedDisplay ?? viewModel.Displays.FirstOrDefault();
        if (display is null)
        {
            return;
        }

        fullscreenWindow.ShowImages(viewModel.ActiveImages.ToList(), display.Bounds, viewModel.AllowMultipleImages);
        await Task.CompletedTask;
    }

    private void UpdatePresentation()
    {
        _ = DispatcherQueue.EnqueueAsync(async () =>
        {
            await EnsureFullscreenWindowAsync();
            UpdateStatus();
        });
    }

    private void SetBrowserPanelVisible(bool isVisible)
    {
        browserPanelVisible = isVisible;
        BrowserPanel.Visibility = isVisible ? Visibility.Visible : Visibility.Collapsed;
        BrowserColumn.Width = isVisible ? new GridLength(280) : new GridLength(0);
        BrowserToggle.IsChecked = isVisible;
        BrowserMenuItem.IsChecked = isVisible;
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

        browserFiles.Clear();
        browserFiles.AddRange(imageLibraryService.EnumerateImageFiles(folder.Path));
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

    private void Exit_Click(object sender, RoutedEventArgs e)
    {
        Close();
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

    private async void DisplayComboBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (DisplayComboBox.SelectedItem is DisplayOption display)
        {
            viewModel.SelectedDisplay = display;
            await RefreshPreviewAsync();
            UpdateStatus();
        }
    }

    private void AlwaysOnTopToggle_Checked(object sender, RoutedEventArgs e)
    {
        viewModel.StatusText = viewModel.StatusText;
        WindowingService.SetTopMost(this, true);
        AlwaysOnTopMenuItem.IsChecked = true;
    }

    private void AlwaysOnTopToggle_Unchecked(object sender, RoutedEventArgs e)
    {
        WindowingService.SetTopMost(this, false);
        AlwaysOnTopMenuItem.IsChecked = false;
    }

    private void AllowMultipleToggle_Checked(object sender, RoutedEventArgs e)
    {
        viewModel.AllowMultipleImages = true;
        AllowMultipleMenuItem.IsChecked = true;
        UpdatePresentation();
    }

    private void AllowMultipleToggle_Unchecked(object sender, RoutedEventArgs e)
    {
        viewModel.AllowMultipleImages = false;
        AllowMultipleMenuItem.IsChecked = false;
        while (viewModel.ActiveImages.Count > 1)
        {
            viewModel.ActiveImages.RemoveAt(0);
        }

        UpdatePresentation();
    }

    private void BrowserToggle_Checked(object sender, RoutedEventArgs e)
    {
        SetBrowserPanelVisible(true);
    }

    private void BrowserToggle_Unchecked(object sender, RoutedEventArgs e)
    {
        SetBrowserPanelVisible(false);
    }

    private void BrowserMenuItem_Click(object sender, RoutedEventArgs e)
    {
        SetBrowserPanelVisible(!browserPanelVisible);
    }

    private void AlwaysOnTopMenuItem_Click(object sender, RoutedEventArgs e)
    {
        var isChecked = AlwaysOnTopMenuItem.IsChecked;
        AlwaysOnTopToggle.IsChecked = isChecked;
        WindowingService.SetTopMost(this, isChecked);
    }

    private void AllowMultipleMenuItem_Click(object sender, RoutedEventArgs e)
    {
        var isChecked = AllowMultipleMenuItem.IsChecked;
        AllowMultipleToggle.IsChecked = isChecked;
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
        await EnsureFullscreenWindowAsync();
        UpdateStatus();
    }
}
