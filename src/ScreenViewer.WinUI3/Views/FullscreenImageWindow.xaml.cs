using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Input;
using ScreenViewer.WinUI3.Models;
using ScreenViewer.WinUI3.Services;
using Microsoft.UI.Xaml.Controls;
using System.Drawing;

namespace ScreenViewer.WinUI3.Views;

public sealed partial class FullscreenImageWindow : Window
{
    public FullscreenImageWindow()
    {
        InitializeComponent();
        WindowingService.ApplyBorderless(this);
        WindowingService.SetTopMost(this, true);
    }

    public void ShowImages(IReadOnlyList<ImageEntry> images, Rectangle bounds, bool allowMultiple)
    {
        if (images.Count == 0)
        {
            Close();
            return;
        }

        WindowingService.MoveAndResize(this, bounds);
        WindowingService.SetTopMost(this, true);

        if (allowMultiple && images.Count > 1)
        {
            SingleImageHost.Visibility = Visibility.Collapsed;
            MultiImageHost.Visibility = Visibility.Visible;
            MultiImageStack.Children.Clear();
            foreach (var imageEntry in images)
            {
                MultiImageStack.Children.Add(CreateImageCard(imageEntry));
            }
        }
        else
        {
            MultiImageHost.Visibility = Visibility.Collapsed;
            SingleImageHost.Visibility = Visibility.Visible;
            SingleImage.Source = images[^1].Source;
        }

        Activate();
        RootGrid.Focus(FocusState.Programmatic);
    }

    public void HideWindow()
    {
        Close();
    }

    private static UIElement CreateImageCard(ImageEntry imageEntry)
    {
        var border = new Border
        {
            Margin = new Thickness(12),
            Background = new Microsoft.UI.Xaml.Media.SolidColorBrush(Microsoft.UI.Colors.Black),
            CornerRadius = new CornerRadius(8),
            Padding = new Thickness(12),
            Child = new Microsoft.UI.Xaml.Controls.Image
            {
                Source = imageEntry.Source,
                Stretch = Microsoft.UI.Xaml.Media.Stretch.Uniform,
                HorizontalAlignment = HorizontalAlignment.Center,
                VerticalAlignment = VerticalAlignment.Center,
            }
        };

        return border;
    }

    private void RootGrid_Loaded(object sender, RoutedEventArgs e)
    {
        RootGrid.Focus(FocusState.Programmatic);
    }

    private void RootGrid_PointerPressed(object sender, PointerRoutedEventArgs e)
    {
        HideWindow();
    }

    private void RootGrid_KeyDown(object sender, Microsoft.UI.Xaml.Input.KeyRoutedEventArgs e)
    {
        HideWindow();
    }
}
