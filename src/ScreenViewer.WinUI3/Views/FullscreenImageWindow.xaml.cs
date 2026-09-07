using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Input;
using ScreenViewer.WinUI3.Models;
using ScreenViewer.WinUI3.Services;
using Microsoft.UI.Xaml.Controls;
using System.Collections.Generic;
using Microsoft.UI.Xaml.Media;
using Rectangle = System.Drawing.Rectangle;

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
            RenderImageTiles(images);
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

    private void RenderImageTiles(IReadOnlyList<ImageEntry> images)
    {
        MultiImageHost.Children.Clear();
        MultiImageHost.RowDefinitions.Clear();
        MultiImageHost.ColumnDefinitions.Clear();

        var count = images.Count;
        var (rows, columns) = GetTileGridSize(count);

        for (var row = 0; row < rows; row++)
        {
            MultiImageHost.RowDefinitions.Add(new RowDefinition { Height = new GridLength(1, GridUnitType.Star) });
        }

        for (var column = 0; column < columns; column++)
        {
            MultiImageHost.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
        }

        for (var index = 0; index < count; index++)
        {
            var imageEntry = images[index];
            var tile = new Border
            {
                Margin = new Thickness(6),
                Background = new SolidColorBrush(Microsoft.UI.ColorHelper.FromArgb(255, 15, 18, 22)),
                BorderBrush = new SolidColorBrush(Microsoft.UI.ColorHelper.FromArgb(255, 43, 49, 56)),
                BorderThickness = new Thickness(1),
                CornerRadius = new CornerRadius(10),
                Padding = new Thickness(10),
                Child = new Image
                {
                    Source = imageEntry.Source,
                    Stretch = Stretch.Uniform,
                    HorizontalAlignment = HorizontalAlignment.Stretch,
                    VerticalAlignment = VerticalAlignment.Stretch,
                }
            };

            Grid.SetRow(tile, index / columns);
            Grid.SetColumn(tile, index % columns);
            MultiImageHost.Children.Add(tile);
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

    private void RootGrid_Loaded(object sender, RoutedEventArgs e)
    {
        RootGrid.Focus(FocusState.Programmatic);
    }

    private void RootGrid_KeyDown(object sender, Microsoft.UI.Xaml.Input.KeyRoutedEventArgs e)
    {
        if (e.Key == Windows.System.VirtualKey.Escape)
        {
            HideWindow();
        }
    }
}
