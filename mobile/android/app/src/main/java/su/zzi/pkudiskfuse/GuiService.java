package su.zzi.pkudiskfuse;

import android.app.ActivityManager;
import android.app.Service;
import android.content.Intent;
import android.net.Uri;
import android.os.Binder;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.SystemClock;
import java.io.File;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONObject;
import su.zzi.pkudiskfuse.mobile.mobilegui.Bridge;
import su.zzi.pkudiskfuse.mobile.mobilegui.Mobilegui;

/** Android lifecycle and notification policy around the shared mobile core. */
public final class GuiService extends Service {
    public interface Listener {
        void onStatus(String message);
        void onOpen(String address);
    }
    public final class LocalBinder extends Binder {
        GuiService service() { return GuiService.this; }
    }
    // Serializes shutdown/restart even if Android recreates the Service object.
    private static final ExecutorService COMMANDS = Executors.newSingleThreadExecutor();
    private static Bridge bridge;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final LocalBinder binder = new LocalBinder();
    private Listener listener;
    private TaskNotifications notifications;
    private boolean starting, destroyed, opening, checking, suspending, visible, pickerActive;
    private String pendingURL, origin;
    private long pendingAt;
    private String status = "Local service stopped.";
    private final Runnable monitor = this::refresh;
    private final Runnable idle = () -> {
        if (destroyed || starting || visible || pickerActive || suspending) return;
        suspending = true;
        COMMANDS.execute(() -> {
            boolean released = bridge.suspend();
            main.post(() -> {
                suspending = false;
                if (destroyed) return;
                if (released) {
                    notifications.clear();
                    if (visible || pickerActive) { opening = true; requestOpen(); }
                    else {
                        main.removeCallbacks(monitor);
                        stopSelf();
                    }
                } else refresh();
            });
        });
    };

    @Override public void onCreate() {
        super.onCreate();
        go.Seq.setContext(getApplicationContext());
        if (bridge == null) bridge = Mobilegui.newBridge(
                new File(getFilesDir(), "service").getAbsolutePath(),
                new File(getCacheDir(), "gui").getAbsolutePath(), BuildConfig.VERSION_NAME);
        notifications = new TaskNotifications(this);
    }
    @Override public IBinder onBind(Intent intent) { return binder; }
    @Override public int onStartCommand(Intent intent, int flags, int startId) {
        // Started while the Activity is visible. Foreground promotion is only
        // necessary when work is active; idle use needs no persistent notification.
        return START_NOT_STICKY;
    }
    void setListener(Listener next) {
        listener = next;
        if (next != null) next.onStatus(status);
    }
    void setVisible(boolean value) {
        visible = value;
        if (value) main.removeCallbacks(idle);
        refresh();
    }
    void setPickerActive(boolean value) {
        pickerActive = value;
        if (value) main.removeCallbacks(idle);
        refresh();
    }
    boolean running() { return bridge.running(); }
    void open(String loadedOrigin) {
        if (loadedOrigin != null && loadedOrigin.equals(origin) && bridge.running()) return;
        // Only an intentional idle release auto-resumes an existing window.
        // A timeout or failure must retain the web UI's stopped overlay.
        if (loadedOrigin != null && !bridge.running() && !bridge.suspended() && !starting) return;
        opening = true;
        if (pendingURL != null && SystemClock.uptimeMillis() - pendingAt < 45000) deliverURL();
        else if (!starting && !suspending) requestOpen();
    }
    private void requestOpen() {
        starting = true;
        update("Starting local service…");
        COMMANDS.execute(() -> {
            try {
                String address = bridge.open();
                main.post(() -> {
                    starting = false;
                    if (destroyed) return;
                    Uri uri = Uri.parse(address);
                    if (!"http".equals(uri.getScheme()) || !"127.0.0.1".equals(uri.getHost()) || uri.getPort() < 1
                            || uri.getUserInfo() != null || uri.getQuery() != null || !"/".equals(uri.getPath())
                            || uri.getFragment() == null || !uri.getFragment().startsWith("ticket=")) {
                        ended("The local service returned an invalid address.");
                        return;
                    }
                    origin = uri.getScheme() + "://" + uri.getAuthority();
                    pendingURL = address;
                    pendingAt = SystemClock.uptimeMillis();
                    update("Local service running.");
                    refresh();
                    deliverURL();
                });
            } catch (Exception error) {
                main.post(() -> ended(error.getMessage()));
            }
        });
    }
    private void refresh() {
        main.removeCallbacks(monitor);
        if (destroyed || checking || suspending) return;
        if (starting) { main.postDelayed(monitor, 500); return; }
        checking = true;
        COMMANDS.execute(() -> {
            try {
                JSONObject snapshot = new JSONObject(bridge.status());
                main.post(() -> {
                    checking = false;
                    if (destroyed || suspending) return;
                    if (starting) { main.postDelayed(monitor, 500); return; }
                    if (!snapshot.optBoolean("running")) {
                        if (!bridge.suspended()) ended();
                        return;
                    }
                    boolean busy = snapshot.optBoolean("busy");
                    notifications.update(snapshot.optJSONArray("tasks"), busy);
                    if (busy || visible || pickerActive) main.removeCallbacks(idle);
                    else if (!main.hasCallbacks(idle)) main.postDelayed(idle, 1000);
                    main.postDelayed(monitor, 500);
                });
            } catch (Exception error) {
                main.post(() -> { checking = false; ended(error.getMessage()); });
            }
        });
    }
    void stop() {
        COMMANDS.execute(() -> {
            try { bridge.stop(); main.post(this::ended); }
            catch (Exception error) { main.post(() -> update(error.getMessage())); }
        });
    }
    private void deliverURL() {
        if (opening && listener != null && pendingURL != null) {
            String address = pendingURL;
            pendingURL = null;
            opening = false;
            listener.onOpen(address);
        }
    }
    private void update(String message) {
        if (destroyed) return;
        status = message;
        if (listener != null) listener.onStatus(message);
    }
    private void ended() { ended("Local service stopped."); }
    private void ended(String message) {
        if (destroyed) return;
        starting = false;
        pendingURL = null;
        opening = false;
        main.removeCallbacks(monitor);
        main.removeCallbacks(idle);
        update(message);
        notifications.clear();
        stopSelf();
        if (bridge.quitRequested()) {
            for (ActivityManager.AppTask task : getSystemService(ActivityManager.class).getAppTasks())
                task.finishAndRemoveTask();
        }
    }
    @Override public void onTimeout(int startId, int fgsType) {
        // Android's dataSync budget is a forced stop, never an idle suspension.
        COMMANDS.execute(bridge::close);
        ended();
    }
    @Override public void onDestroy() {
        destroyed = true;
        listener = null;
        main.removeCallbacks(monitor);
        main.removeCallbacks(idle);
        notifications.clear();
        COMMANDS.execute(bridge::close);
        super.onDestroy();
    }
}
