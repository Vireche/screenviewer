using Microsoft.UI.Xaml;
using System.Runtime.InteropServices;
using System.Text;

namespace ScreenViewer.WinUI3;

public partial class App : Application
{
    public App()
    {
        InitializeComponent();
        UnhandledException += App_UnhandledException;
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        try
        {
            MainWindow = new MainWindow();
            MainWindow.Activate();
        }
        catch (Exception exception)
        {
            LogException("OnLaunched", exception);
            ShowErrorBox("ScreenViewer failed to launch.\n\n" + exception);
            throw;
        }
    }

    public static MainWindow? MainWindow { get; private set; }

    private void App_UnhandledException(object sender, Microsoft.UI.Xaml.UnhandledExceptionEventArgs e)
    {
        LogException("UnhandledException", e.Exception);
        ShowErrorBox("Unhandled startup error:\n\n" + e.Exception);
    }

    private static void LogException(string source, Exception exception)
    {
        try
        {
            var logDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "ScreenViewer");
            Directory.CreateDirectory(logDir);
            var logPath = Path.Combine(logDir, "startup-error.log");
            var text = new StringBuilder()
                .AppendLine($"[{DateTimeOffset.Now:O}] {source}")
                .AppendLine(exception.ToString())
                .AppendLine(new string('-', 80))
                .ToString();
            File.AppendAllText(logPath, text);
        }
        catch
        {
        }
    }

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern int MessageBoxW(IntPtr hWnd, string text, string caption, uint type);

    private static void ShowErrorBox(string message)
    {
        MessageBoxW(IntPtr.Zero, message, "ScreenViewer", 0);
    }
}
