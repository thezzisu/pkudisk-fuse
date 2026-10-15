package su.zzi.pkudiskfuse;

import android.app.ActivityManager;
import android.app.Service;
import android.content.Intent;
import android.net.Uri;
import android.os.Binder;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import java.io.File;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONObject;
import su.zzi.pkudiskfuse.mobile.mobilegui.Bridge;
import su.zzi.pkudiskfuse.mobile.mobilegui.Mobilegui;

/** Android lifecycle and notification policy around the shared mobile core. */
public final class GuiService extends Service {
    public interface Listener {
        void onConnecting();
        void onOpen(String address);
        void onReady();
        void onStopped();
        void onFailure();
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
    private String origin, requestedOrigin;
    private int lastStartId;
    private final Runnable monitor = this::refresh;
    private final Runnable idle = () -> {
        if (destroyed || starting || visible || pickerActive || suspending) return;
        suspending = true;
        final int idleStartId = lastStartId;
        COMMANDS.execute(() -> {
            boolean released = bridge.suspend();
            main.post(() -> {
                suspending = false;
                if (destroyed) return;
                if (released) {
                    origin = null;
                    notifications.clear();
                    if (visible || pickerActive) { opening = true; requestOpen(); }
                    else {
                        main.removeCallbacks(monitor);
                        stopSelf(idleStartId);
                    }
                } else {
                    if (opening) open(requestedOrigin);
                    refresh();
                }
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
        lastStartId = startId;
        // Idle visible use needs no persistent notification. Work promotes the
        // service to foreground operation through the task snapshot below.
        return START_NOT_STICKY;
    }
    void setListener(Listener next) { listener = next; }
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
        if (BuildConfig.DEBUG) android.util.Log.d("PKUDiskLifecycle", "open loaded=" + (loadedOrigin != null) + " starting=" + starting + " suspending=" + suspending + " running=" + bridge.running());
        requestedOrigin = loadedOrigin;
        opening = true;
        if (listener != null) listener.onConnecting();
        // Neither a cached activation nor the old document is usable while an
        // idle release is in flight. Its completion resumes this request.
        if (suspending || starting) return;
        if (loadedOrigin != null && loadedOrigin.equals(origin) && bridge.running()) {
            opening = false;
            if (listener != null) listener.onReady();
            return;
        }
        if (loadedOrigin != null && !bridge.running() && !bridge.suspended()) {
            opening = false;
            if (listener != null) listener.onStopped();
            return;
        }
        requestOpen();
    }
    private void requestOpen() {
        starting = true;
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
                        failed();
                        return;
                    }
                    origin = uri.getScheme() + "://" + uri.getAuthority();
                    if (BuildConfig.DEBUG) android.util.Log.d("PKUDiskLifecycle", "activation ready opening=" + opening + " visible=" + visible + " listener=" + (listener != null));
                    if (opening && listener != null && visible) listener.onOpen(address);
                    // An activation returned while hidden is deliberately not
                    // cached. A later open obtains a fresh single-use ticket.
                    opening = false;
                    refresh();
                });
            } catch (Exception error) { main.post(this::failed); }
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
            } catch (Exception error) { main.post(() -> { checking = false; failed(); }); }
        });
    }
    void stop() {
        COMMANDS.execute(() -> {
            try { bridge.stop(); main.post(this::ended); }
            catch (Exception error) { main.post(() -> { if (listener != null) listener.onFailure(); }); }
        });
    }
    private void failed() {
        if (destroyed) return;
        ended();
        if (listener != null) listener.onFailure();
    }
    private void ended() {
        if (destroyed) return;
        starting = false;
        opening = false;
        main.removeCallbacks(monitor);
        main.removeCallbacks(idle);
        notifications.clear();
        stopSelf();
        if (bridge.quitRequested()) {
            for (ActivityManager.AppTask task : getSystemService(ActivityManager.class).getAppTasks())
                task.finishAndRemoveTask();
        } else if (listener != null) listener.onStopped();
    }
    @Override public void onTimeout(int startId, int fgsType) {
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
