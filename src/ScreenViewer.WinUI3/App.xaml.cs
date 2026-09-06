using Microsoft.UI.Xaml;

namespace ScreenViewer.WinUI3;

public partial class App : Application
{
    public App()
    {
        InitializeComponent();
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        MainWindow = new MainWindow();
        MainWindow.Activate();
    }

    public static MainWindow? MainWindow { get; private set; }
}
