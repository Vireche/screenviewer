using System.Net;
using System.Text;
using System.Text.RegularExpressions;

namespace ScreenViewer.WinUI3.Services;

public sealed class LocalUploadServer : IAsyncDisposable
{
    private readonly int port;
    private readonly CancellationTokenSource shutdown = new();
    private HttpListener? listener;
    private Task? loopTask;

    public LocalUploadServer(int port)
    {
        this.port = port;
    }

    public Func<byte[], string?, string?, Task>? ImageReceivedAsync { get; set; }

    public Task StartAsync()
    {
        if (listener is not null)
        {
            return Task.CompletedTask;
        }

        listener = new HttpListener();
        listener.Prefixes.Add($"http://localhost:{port}/");
        listener.Start();
        loopTask = Task.Run(ListenLoopAsync);
        return Task.CompletedTask;
    }

    public async ValueTask DisposeAsync()
    {
        shutdown.Cancel();
        if (listener is not null)
        {
            listener.Stop();
            listener.Close();
            listener = null;
        }

        if (loopTask is not null)
        {
            try
            {
                await loopTask;
            }
            catch
            {
            }
        }

        shutdown.Dispose();
    }

    private async Task ListenLoopAsync()
    {
        if (listener is null)
        {
            return;
        }

        while (!shutdown.IsCancellationRequested && listener.IsListening)
        {
            HttpListenerContext? context = null;
            try
            {
                context = await listener.GetContextAsync();
            }
            catch when (shutdown.IsCancellationRequested)
            {
                break;
            }
            catch
            {
                continue;
            }

            if (context is not null)
            {
                _ = Task.Run(() => HandleRequestAsync(context));
            }
        }
    }

    private async Task HandleRequestAsync(HttpListenerContext context)
    {
        try
        {
            var path = context.Request.Url?.AbsolutePath ?? string.Empty;
            if (string.Equals(path, "/ping", StringComparison.OrdinalIgnoreCase))
            {
                await WriteTextAsync(context.Response, 200, "pong");
                return;
            }

            if (string.Equals(path, "/upload", StringComparison.OrdinalIgnoreCase) && string.Equals(context.Request.HttpMethod, "POST", StringComparison.OrdinalIgnoreCase))
            {
                var contentType = context.Request.ContentType ?? string.Empty;
                if (!contentType.Contains("multipart/form-data", StringComparison.OrdinalIgnoreCase))
                {
                    await WriteTextAsync(context.Response, 400, "Expected multipart/form-data");
                    return;
                }

                var boundary = GetBoundary(contentType);
                if (boundary is null)
                {
                    await WriteTextAsync(context.Response, 400, "Missing multipart boundary");
                    return;
                }

                using var memory = new MemoryStream();
                await context.Request.InputStream.CopyToAsync(memory);
                var body = memory.ToArray();

                if (!TryExtractImagePart(body, boundary, out var imageBytes, out var fileName, out var source))
                {
                    await WriteTextAsync(context.Response, 400, "No image part found");
                    return;
                }

                if (ImageReceivedAsync is not null)
                {
                    await ImageReceivedAsync(imageBytes, fileName, source);
                }

                await WriteTextAsync(context.Response, 200, "ok");
                return;
            }

            await WriteTextAsync(context.Response, 404, "Not found");
        }
        catch (Exception ex)
        {
            await WriteTextAsync(context.Response, 500, ex.Message);
        }
    }

    private static async Task WriteTextAsync(HttpListenerResponse response, int statusCode, string text)
    {
        response.StatusCode = statusCode;
        response.ContentType = "text/plain; charset=utf-8";
        var data = Encoding.UTF8.GetBytes(text);
        response.ContentLength64 = data.Length;
        await response.OutputStream.WriteAsync(data);
        response.OutputStream.Close();
    }

    private static string? GetBoundary(string contentType)
    {
        var match = Regex.Match(contentType, "boundary=(?<value>[^;]+)", RegexOptions.IgnoreCase);
        if (!match.Success)
        {
            return null;
        }

        return match.Groups["value"].Value.Trim('"');
    }

    private static bool TryExtractImagePart(byte[] body, string boundary, out byte[] imageBytes, out string? fileName, out string? source)
    {
        imageBytes = [];
        fileName = null;
        source = null;

        var boundaryBytes = Encoding.ASCII.GetBytes("--" + boundary);
        var marker = Encoding.ASCII.GetBytes("\r\n\r\n");
        var offset = IndexOf(body, boundaryBytes, 0);

        while (offset >= 0)
        {
            var headerStart = offset + boundaryBytes.Length + 2;
            if (headerStart >= body.Length)
            {
                break;
            }

            var headerEnd = IndexOf(body, marker, headerStart);
            if (headerEnd < 0)
            {
                break;
            }

            var headers = Encoding.UTF8.GetString(body, headerStart, headerEnd - headerStart);
            var contentStart = headerEnd + marker.Length;
            var nextBoundary = IndexOf(body, boundaryBytes, contentStart);
            if (nextBoundary < 0)
            {
                break;
            }

            var contentEnd = Math.Max(contentStart, nextBoundary - 2);
            var partBytes = body.AsSpan(contentStart, contentEnd - contentStart).ToArray();
            var name = GetDispositionValue(headers, "name");
            var partFileName = GetDispositionValue(headers, "filename");

            if (string.Equals(name, "image", StringComparison.OrdinalIgnoreCase))
            {
                imageBytes = partBytes;
                fileName = partFileName;
                return true;
            }

            if (string.Equals(name, "source", StringComparison.OrdinalIgnoreCase))
            {
                source = Encoding.UTF8.GetString(partBytes).Trim();
            }

            offset = nextBoundary;
        }

        return false;
    }

    private static int IndexOf(byte[] buffer, byte[] pattern, int startIndex)
    {
        for (var index = startIndex; index <= buffer.Length - pattern.Length; index++)
        {
            var matched = true;
            for (var patternIndex = 0; patternIndex < pattern.Length; patternIndex++)
            {
                if (buffer[index + patternIndex] != pattern[patternIndex])
                {
                    matched = false;
                    break;
                }
            }

            if (matched)
            {
                return index;
            }
        }

        return -1;
    }

    private static string? GetDispositionValue(string headers, string key)
    {
        var match = Regex.Match(headers, $"{key}=(?:\"(?<value>[^\"]+)\"|(?<value>[^;\r\n]+))", RegexOptions.IgnoreCase);
        return match.Success ? match.Groups["value"].Value : null;
    }
}
