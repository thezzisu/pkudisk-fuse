package su.zzi.pkudiskfuse;

import android.Manifest;
import android.app.DownloadManager;
import android.content.ComponentName;
import android.content.Intent;
import android.content.ServiceConnection;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.os.IBinder;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.CookieManager;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.widget.Toast;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;
import androidx.activity.ComponentActivity;
import androidx.activity.OnBackPressedCallback;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.webkit.URLUtilCompat;

/** A WebView host for the in-process gomobile library. */
public final class MainActivity extends ComponentActivity implements GuiService.Listener {
    private GuiService service;
    private TextView status;
    private LinearLayout controls;
    private WebView webView;
    private String origin;
    private boolean bound;
    private boolean forceOpen;
    private ValueCallback<Uri[]> fileResult;

    private final ActivityResultLauncher<Intent> files = registerForActivityResult(
            new ActivityResultContracts.StartActivityForResult(), result -> {
                ValueCallback<Uri[]> callback = fileResult;
                fileResult = null;
                if (callback != null) callback.onReceiveValue(
                        WebChromeClient.FileChooserParams.parseResult(result.getResultCode(), result.getData()));
                if (service != null) service.setPickerActive(false);
            });
    private final ActivityResultLauncher<String> notifications = registerForActivityResult(
            new ActivityResultContracts.RequestPermission(), granted -> {});
    private final ServiceConnection connection = new ServiceConnection() {
        @Override public void onServiceConnected(ComponentName name, IBinder binder) {
            service = ((GuiService.LocalBinder) binder).service();
            service.setListener(MainActivity.this);
            service.setVisible(true);
            service.setPickerActive(fileResult != null);
            service.open(forceOpen ? null : origin);
            forceOpen = false;
        }
        @Override public void onServiceDisconnected(ComponentName name) {
            service = null;
            onStatus("Local service disconnected. Tap Open to restart.");
        }
    };

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        ViewCompat.setOnApplyWindowInsetsListener(layout, (view, insets) -> {
            var bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() | WindowInsetsCompat.Type.ime());
            view.setPadding(bars.left, bars.top, bars.right, bars.bottom);
            return insets;
        });
        controls = new LinearLayout(this);
        controls.setOrientation(LinearLayout.VERTICAL);
        int padding = (int) (16 * getResources().getDisplayMetrics().density);
        controls.setPadding(padding, padding, padding, padding);
        status = new TextView(this);
        status.setText(R.string.intro);
        controls.addView(status);
        Button open = new Button(this);
        open.setText(R.string.open_browser);
        open.setOnClickListener(view -> startAndOpen(true));
        controls.addView(open);
        Button stop = new Button(this);
        stop.setText(R.string.stop_service);
        stop.setOnClickListener(view -> { if (service != null) service.stop(); });
        controls.addView(stop);
        layout.addView(controls);
        webView = new WebView(this);
        layout.addView(webView, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(layout);
        configureWebView();
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override public void handleOnBackPressed() {
                if (service != null && service.running() && webView != null && webView.canGoBack()) webView.goBack();
                else if (service == null || !service.running()) finishAndRemoveTask();
                else finish();
            }
        });
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED)
            notifications.launch(Manifest.permission.POST_NOTIFICATIONS);
    }

    @android.annotation.SuppressLint("SetJavaScriptEnabled")
    private void configureWebView() {
        WebView.setWebContentsDebuggingEnabled(BuildConfig.DEBUG);
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        CookieManager.getInstance().setAcceptThirdPartyCookies(webView, false);
        webView.setWebViewClient(new WebViewClient() {
            @Override public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
                ((ViewGroup) view.getParent()).removeView(view);
                view.destroy();
                webView = null;
                onStatus("WebView stopped. Tap Open to reload.");
                return true;
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                // Keep the embedded interface on its exact local origin.
                return !trusted(request.getUrl());
            }
        });
        webView.setDownloadListener((url, userAgent, disposition, mimeType, length) -> {
            Uri uri = Uri.parse(url);
            if (!trusted(uri) || !uri.getPath().startsWith("/download/")) return;
            try {
                // RFC 6266/5987 filename* preserves Chinese attachment names on
                // every supported Android version, including Android 10.
                String name = URLUtilCompat.guessFileName(url, disposition, mimeType);
                DownloadManager.Request request = new DownloadManager.Request(uri)
                        .setMimeType(mimeType).setTitle(name)
                        .addRequestHeader("User-Agent", userAgent)
                        .setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, name)
                        .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
                String cookies = CookieManager.getInstance().getCookie(url);
                if (cookies != null) request.addRequestHeader("Cookie", cookies);
                getSystemService(DownloadManager.class).enqueue(request);
            } catch (RuntimeException error) {
                Toast.makeText(this, "Could not start the download.", Toast.LENGTH_LONG).show();
            }
        });
        webView.setWebChromeClient(new WebChromeClient() {
            @Override public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                if (!trusted(Uri.parse(view.getUrl()))) return false;
                if (fileResult != null) fileResult.onReceiveValue(null);
                fileResult = callback;
                if (service != null) service.setPickerActive(true);
                try { files.launch(params.createIntent()); }
                catch (RuntimeException error) {
                    fileResult.onReceiveValue(null);
                    fileResult = null;
                    if (service != null) service.setPickerActive(false);
                    onStatus("No system file picker is available.");
                }
                return true;
            }
        });
    }

    private boolean trusted(Uri uri) {
        return origin != null && "http".equals(uri.getScheme()) && uri.getUserInfo() == null
                && origin.equals(uri.getScheme() + "://" + uri.getAuthority());
    }

    private void startAndOpen(boolean force) {
        if (webView == null) { recreate(); return; }
        try {
            startService(new Intent(this, GuiService.class));
            if (service != null) { service.setVisible(true); service.open(force ? null : origin); }
            else { forceOpen |= force; bind(); }
        } catch (RuntimeException error) { onStatus("Could not start service: " + error.getMessage()); }
    }
    private void bind() {
        if (!bound) bound = bindService(new Intent(this, GuiService.class), connection, 0);
    }
    @Override protected void onStart() { super.onStart(); startAndOpen(false); }
    @Override protected void onStop() {
        if (service != null) { service.setListener(null); service.setVisible(false); }
        if (bound) unbindService(connection);
        bound = false;
        service = null;
        super.onStop();
    }
    @Override public void onStatus(String message) {
        status.setText(message);
        // Once loaded, the web UI owns service-loss feedback. Native recovery
        // controls are only for startup failures or a crashed WebView renderer.
        controls.setVisibility(origin == null || webView == null ? View.VISIBLE : View.GONE);
    }
    @Override public void onOpen(String address) {
        Uri uri = Uri.parse(address);
        origin = uri.getScheme() + "://" + uri.getAuthority();
        controls.setVisibility(View.GONE);
        webView.loadUrl(address);
    }
    @Override protected void onDestroy() {
        if (fileResult != null) fileResult.onReceiveValue(null);
        if (webView != null) webView.destroy();
        super.onDestroy();
    }
}
