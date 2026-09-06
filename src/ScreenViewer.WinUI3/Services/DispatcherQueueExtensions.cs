using Microsoft.UI.Dispatching;

namespace ScreenViewer.WinUI3.Services;

public static class DispatcherQueueExtensions
{
    public static Task EnqueueAsync(this DispatcherQueue dispatcherQueue, Action action)
    {
        var completion = new TaskCompletionSource();
        if (!dispatcherQueue.TryEnqueue(() =>
        {
            try
            {
                action();
                completion.SetResult();
            }
            catch (Exception exception)
            {
                completion.SetException(exception);
            }
        }))
        {
            completion.SetException(new InvalidOperationException("Failed to enqueue UI work."));
        }

        return completion.Task;
    }

    public static Task EnqueueAsync(this DispatcherQueue dispatcherQueue, Func<Task> action)
    {
        var completion = new TaskCompletionSource();
        if (!dispatcherQueue.TryEnqueue(async () =>
        {
            try
            {
                await action();
                completion.SetResult();
            }
            catch (Exception exception)
            {
                completion.SetException(exception);
            }
        }))
        {
            completion.SetException(new InvalidOperationException("Failed to enqueue UI work."));
        }

        return completion.Task;
    }
}
