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
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.CookieManager;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.Toast;
import androidx.activity.ComponentActivity;
import androidx.activity.OnBackPressedCallback;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.core.splashscreen.SplashScreen;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;
import androidx.webkit.URLUtilCompat;

/** A WebView host; startup and resume share one covered, render-ready transition. */
public final class MainActivity extends ComponentActivity implements GuiService.Listener {
    private static final String ROUTE = "route";
    // Read-only readiness contract. No JavaScript interface or native action is
    // exposed to web content, and no credentials are read from the document.
    private static final String PAGE_STATE = "(()=>{if(document.readyState!=='complete')return 'connecting';"
            + "const p=document.documentElement.dataset.pkudiskPhase;"
            + "return p==='ready'&&!document.querySelector('main,[role=main]')?'connecting':p})()";
    private final Handler main = new Handler(Looper.getMainLooper());
    private GuiService service;
    private StartupScreen startup;
    private WebView webView;
    private String origin, route, pendingAddress;
    private boolean bound, forceOpen, started, resumed, covering = true, pageCommitted, allowStopped, clearHistory;
    private int generation, recoveryAttempts;
    private String lastPageState;
    private void trace(String message) { if (BuildConfig.DEBUG) android.util.Log.d("PKUDiskLifecycle", message); }
    private ValueCallback<Uri[]> fileResult;
    private final Runnable checkPage = this::checkPageReady;
    private final Runnable startupTimeout = () -> showFailure(R.string.startup_browser_error);

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
            if (!started) { service.setVisible(false); return; }
            service.setListener(MainActivity.this);
            service.setVisible(true);
            service.setPickerActive(fileResult != null);
            service.open(forceOpen ? null : origin);
            forceOpen = false;
        }
        @Override public void onServiceDisconnected(ComponentName name) {
            service = null;
            if (started) showFailure(R.string.startup_service_error);
        }
    };

    @Override public void onCreate(Bundle state) {
        SplashScreen.installSplashScreen(this);
        super.onCreate(state);
        String saved = state == null ? null : state.getString(ROUTE);
        String restored = saved == null ? getPreferences(MODE_PRIVATE).getString(ROUTE, "/") : saved;
        route = safeRoute(Uri.parse("http://localhost" + (restored.startsWith("/") ? restored : "/")));
        FrameLayout layout = new FrameLayout(this);
        ViewCompat.setOnApplyWindowInsetsListener(layout, (view, insets) -> {
            var bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() | WindowInsetsCompat.Type.ime());
            view.setPadding(bars.left, bars.top, bars.right, bars.bottom);
            return insets;
        });
        webView = new WebView(this);
        webView.setBackgroundColor(getColor(R.color.startup_background));
        layout.addView(webView, new FrameLayout.LayoutParams(-1, -1));
        startup = new StartupScreen(this, () -> { recoveryAttempts = 0; startAndOpen(true); }, () -> {
            if (service != null && service.running()) service.stop();
            else finishAndRemoveTask();
        });
        layout.addView(startup, new FrameLayout.LayoutParams(-1, -1));
        setContentView(layout);
        configureWebView();
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override public void handleOnBackPressed() {
                if (!covering && service != null && service.running() && webView != null && webView.canGoBack()) webView.goBack();
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
                showFailure(R.string.startup_browser_error);
                return true;
            }
            @Override public void doUpdateVisitedHistory(WebView view, String address, boolean reload) {
                rememberRoute(address);
            }
            @Override public void onPageCommitVisible(WebView view, String address) {
                if (trusted(Uri.parse(address))) { pageCommitted = true; checkPageReady(); }
            }
            @Override public void onPageFinished(WebView view, String address) {
                if ("about:blank".equals(address) && pendingAddress != null) {
                    String next = pendingAddress;
                    pendingAddress = null;
                    view.loadUrl(next);
                    return;
                }
                if (!trusted(Uri.parse(address))) return;
                if (clearHistory) { view.clearHistory(); clearHistory = false; }
                checkPageReady();
            }
            @Override public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request.isForMainFrame() && trusted(request.getUrl()) && request.getUrl().toString().equals(view.getUrl()))
                    showFailure(R.string.startup_browser_error);
            }
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return !trusted(request.getUrl());
            }
        });
        webView.setDownloadListener((url, userAgent, disposition, mimeType, length) -> {
            Uri uri = Uri.parse(url);
            if (!trusted(uri) || !uri.getPath().startsWith("/download/")) return;
            try {
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
                if (view.getUrl() == null || !trusted(Uri.parse(view.getUrl()))) return false;
                if (fileResult != null) fileResult.onReceiveValue(null);
                fileResult = callback;
                if (service != null) service.setPickerActive(true);
                try { files.launch(params.createIntent()); }
                catch (RuntimeException error) {
                    fileResult.onReceiveValue(null);
                    fileResult = null;
                    if (service != null) service.setPickerActive(false);
                    Toast.makeText(MainActivity.this, R.string.startup_picker_error, Toast.LENGTH_LONG).show();
                }
                return true;
            }
        });
    }

    private boolean trusted(Uri uri) {
        return origin != null && "http".equals(uri.getScheme()) && uri.getUserInfo() == null
                && origin.equals(uri.getScheme() + "://" + uri.getAuthority());
    }
    private String safeRoute(Uri uri) {
        String path = uri.getPath();
        if (path == null) return "/";
        switch (path) {
            case "/": case "/accounts": case "/transfers": case "/recycle": case "/settings": break;
            default: return "/";
        }
        for (String key : uri.getQueryParameterNames())
            if ("ticket".equals(key) || "token".equals(key) || "access_token".equals(key)) return "/";
        return uri.getEncodedPath() + (uri.getEncodedQuery() == null ? "" : "?" + uri.getEncodedQuery());
    }
    private void rememberRoute(String address) {
        if (address == null) return;
        Uri uri = Uri.parse(address);
        if (!trusted(uri) || uri.getFragment() != null) return;
        route = safeRoute(uri);
        getPreferences(MODE_PRIVATE).edit().putString(ROUTE, route).apply();
    }
    private void cover() {
        generation++;
        covering = true;
        main.removeCallbacks(checkPage);
        main.removeCallbacks(startupTimeout);
        startup.showLoading();
        if (webView != null) webView.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS);
        if (resumed) main.postDelayed(startupTimeout, 30000);
    }
    private void showFailure(int message) {
        if (!started) return;
        cover();
        main.removeCallbacks(startupTimeout);
        startup.showError(getString(message));
    }
    private void checkPageReady() {
        main.removeCallbacks(checkPage);
        if (!resumed || !covering || !pageCommitted || webView == null || webView.getUrl() == null || !trusted(Uri.parse(webView.getUrl()))) return;
        final int current = generation;
        webView.evaluateJavascript(PAGE_STATE, value -> {
            if (!java.util.Objects.equals(value, lastPageState)) { lastPageState = value; trace("page=" + value + " terminal=" + allowStopped); }
            if (current != generation || !resumed || webView == null || !covering) return;
            boolean ready = "\"ready\"".equals(value);
            boolean stopped = "\"stopped\"".equals(value) || "\"expired\"".equals(value);
            if ((!allowStopped && ready) || (allowStopped && stopped)) {
                webView.postVisualStateCallback(current, new WebView.VisualStateCallback() {
                    @Override public void onComplete(long requestId) {
                        if (current != generation || !resumed || webView == null || !covering) return;
                        covering = false;
                        main.removeCallbacks(startupTimeout);
                        webView.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_AUTO);
                        startup.dismiss();
                        webView.requestFocus();
                    }
                });
            } else if (!allowStopped && stopped) {
                if (service != null && service.running() && recoveryAttempts++ == 0) service.open(null);
                else showFailure(R.string.startup_browser_error);
            } else main.postDelayed(checkPage, 80);
        });
    }
    private void startAndOpen(boolean force) {
        if (webView == null) { recreate(); return; }
        cover();
        try {
            startService(new Intent(this, GuiService.class));
            if (service != null) { service.setVisible(true); service.open(force ? null : origin); }
            else { forceOpen |= force; bind(); }
        } catch (RuntimeException error) { showFailure(R.string.startup_service_error); }
    }
    private void bind() {
        if (!bound) bound = bindService(new Intent(this, GuiService.class), connection, 0);
    }
    @Override protected void onStart() {
        super.onStart();
        started = true;
        startAndOpen(false);
    }
    @Override protected void onResume() {
        super.onResume();
        resumed = true;
        trace("resume service=" + (service != null));
        recoveryAttempts = 0;
        cover();
        if (webView != null) webView.onResume();
        // Very fast switches can pause/resume without stopping the Activity.
        // Recheck those returns too, while retaining an active page and uploads.
        if (service != null) service.open(origin);
    }
    @Override protected void onPause() {
        rememberRoute(webView == null ? null : webView.getUrl());
        resumed = false;
        cover();
        if (webView != null) webView.onPause();
        super.onPause();
    }
    @Override protected void onStop() {
        rememberRoute(webView == null ? null : webView.getUrl());
        started = false;
        cover();
        if (service != null) { service.setListener(null); service.setVisible(false); }
        if (bound) unbindService(connection);
        bound = false;
        service = null;
        super.onStop();
    }
    @Override protected void onSaveInstanceState(Bundle state) {
        rememberRoute(webView == null ? null : webView.getUrl());
        state.putString(ROUTE, route);
        super.onSaveInstanceState(state);
    }
    @Override public void onConnecting() { allowStopped = false; cover(); }
    @Override public void onOpen(String address) {
        trace("activate document");
        if (!started || webView == null) return;
        cover();
        allowStopped = false;
        pageCommitted = false;
        clearHistory = true;
        Uri activation = Uri.parse(address);
        origin = activation.getScheme() + "://" + activation.getAuthority();
        // Android Uri treats a colon inside a relative query (e.g. gns://)
        // as a scheme separator. Parse against the trusted absolute origin.
        Uri destination = Uri.parse(origin + route);
        // A fresh ticket on the same URL would otherwise be only a fragment
        // navigation, leaving the old Nuxt session plugin alive. Re-activation
        // always starts a new document behind the startup cover.
        String next = activation.buildUpon().encodedPath(destination.getEncodedPath())
                .encodedQuery(destination.getEncodedQuery()).build().toString();
        if (webView.getUrl() == null || "about:blank".equals(webView.getUrl())) webView.loadUrl(next);
        else {
            pendingAddress = next;
            // Wait for the blank document to commit before navigating again;
            // immediately issuing both loadUrl calls can coalesce into a hash change.
            webView.loadUrl("about:blank");
        }
    }
    @Override public void onReady() { trace("reuse document committed=" + pageCommitted); allowStopped = false; checkPageReady(); }
    @Override public void onStopped() {
        trace("service stopped");
        if (origin == null || webView == null) { showFailure(R.string.startup_service_error); return; }
        allowStopped = true;
        cover();
        checkPageReady();
    }
    @Override public void onFailure() { showFailure(R.string.startup_service_error); }
    @Override protected void onDestroy() {
        generation++;
        main.removeCallbacksAndMessages(null);
        if (fileResult != null) fileResult.onReceiveValue(null);
        if (webView != null) webView.destroy();
        super.onDestroy();
    }
}
