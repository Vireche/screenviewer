using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using Microsoft.UI.Xaml.Media.Imaging;
using ScreenViewer.WinUI3.Models;

namespace ScreenViewer.WinUI3.ViewModels;

public sealed class MainViewModel : INotifyPropertyChanged
{
    private string statusText = "Ready";
    private BitmapImage? previewImageSource;
    private DisplayOption? selectedDisplay;
    private bool allowMultipleImages;

    public ObservableCollection<DisplayOption> Displays { get; } = new();

    public ObservableCollection<ImageEntry> BrowserImages { get; } = new();

    public ObservableCollection<ImageEntry> ActiveImages { get; } = new();

    public string StatusText
    {
        get => statusText;
        set
        {
            if (statusText == value)
            {
                return;
            }

            statusText = value;
            OnPropertyChanged();
        }
    }

    public BitmapImage? PreviewImageSource
    {
        get => previewImageSource;
        set
        {
            if (ReferenceEquals(previewImageSource, value))
            {
                return;
            }

            previewImageSource = value;
            OnPropertyChanged();
        }
    }

    public DisplayOption? SelectedDisplay
    {
        get => selectedDisplay;
        set
        {
            if (Equals(selectedDisplay, value))
            {
                return;
            }

            selectedDisplay = value;
            OnPropertyChanged();
        }
    }

    public bool AllowMultipleImages
    {
        get => allowMultipleImages;
        set
        {
            if (allowMultipleImages == value)
            {
                return;
            }

            allowMultipleImages = value;
            OnPropertyChanged();
        }
    }

    public event PropertyChangedEventHandler? PropertyChanged;

    private void OnPropertyChanged([CallerMemberName] string? propertyName = null)
    {
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(propertyName));
    }
}
