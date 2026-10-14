package su.zzi.pkudiskfuse;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.text.format.Formatter;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import org.json.JSONArray;
import org.json.JSONObject;

/** Native presentation of the shared core's task snapshot; called on the main thread. */
final class TaskNotifications {
    private static final String CHANNEL = "tasks";
    private static final String GROUP = "pkudisk-tasks";
    private static final int SUMMARY_ID = 1;
    private final Service service;
    private final NotificationManager manager;
    private final PendingIntent open;
    private final Map<String, Integer> childIDs = new HashMap<>();
    private int nextID = 2;
    private boolean foreground;

    TaskNotifications(Service service) {
        this.service = service;
        manager = service.getSystemService(NotificationManager.class);
        NotificationChannel channel = new NotificationChannel(CHANNEL,
                service.getString(R.string.notification_channel_tasks), NotificationManager.IMPORTANCE_LOW);
        channel.setDescription(service.getString(R.string.notification_channel_tasks_description));
        channel.setShowBadge(false);
        manager.createNotificationChannel(channel);
        open = PendingIntent.getActivity(service, 0,
                new Intent(service, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    void update(JSONArray tasks, boolean busy) {
        Map<String, JSONObject> active = new LinkedHashMap<>();
        if (tasks != null) {
            for (int i = 0; i < tasks.length(); i++) {
                JSONObject task = tasks.optJSONObject(i);
                if (task == null) continue;
                String id = task.optString("clientId");
                if (id.isEmpty()) id = task.optString("id");
                if (id.isEmpty()) continue;
                // One download can have two HTTP attempts with the same client ID.
                String key = task.optString("account") + "\n" + task.optString("kind") + "\n" + id;
                active.putIfAbsent(key, task);
            }
        }
        active.values().removeIf(task -> !task.optBoolean("active"));
        if (!busy && active.isEmpty()) { clear(); return; }

        Notification summary;
        Set<String> visibleChildren = new HashSet<>();
        if (active.size() == 1) {
            summary = taskNotification(active.values().iterator().next(), false);
        } else if (active.isEmpty()) {
            summary = builder().setContentTitle(service.getString(R.string.app_name))
                    .setContentText(service.getString(R.string.notification_processing))
                    .setProgress(0, 0, true).build();
        } else {
            String count = service.getResources().getQuantityString(
                    R.plurals.notification_active_tasks, active.size(), active.size());
            Notification.InboxStyle lines = new Notification.InboxStyle().setBigContentTitle(count);
            for (JSONObject task : active.values()) lines.addLine(title(task) + " · " + progressText(task));
            summary = builder().setContentTitle(service.getString(R.string.app_name))
                    .setContentText(count).setNumber(active.size()).setStyle(lines)
                    .setGroup(GROUP).setGroupSummary(true).build();
        }
        if (!foreground) {
            service.startForeground(SUMMARY_ID, summary, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
            foreground = true;
        } else {
            manager.notify(SUMMARY_ID, summary);
        }
        if (active.size() > 1) {
            for (Map.Entry<String, JSONObject> entry : active.entrySet()) {
                String key = entry.getKey();
                visibleChildren.add(key);
                int id = childIDs.computeIfAbsent(key, ignored -> nextID++);
                manager.notify(id, taskNotification(entry.getValue(), true));
            }
        }
        childIDs.entrySet().removeIf(entry -> {
            if (visibleChildren.contains(entry.getKey())) return false;
            manager.cancel(entry.getValue());
            return true;
        });
    }

    void clear() {
        for (int id : childIDs.values()) manager.cancel(id);
        childIDs.clear();
        if (foreground) {
            service.stopForeground(Service.STOP_FOREGROUND_REMOVE);
            foreground = false;
        }
        manager.cancel(SUMMARY_ID);
    }

    private Notification.Builder builder() {
        Notification.Builder result = new Notification.Builder(service, CHANNEL)
                .setSmallIcon(R.drawable.notification).setContentIntent(open)
                .setCategory(Notification.CATEGORY_PROGRESS).setVisibility(Notification.VISIBILITY_PRIVATE)
                .setOngoing(true).setOnlyAlertOnce(true).setShowWhen(false)
                .setGroupAlertBehavior(Notification.GROUP_ALERT_SUMMARY);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S)
            result.setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE);
        return result;
    }

    private Notification taskNotification(JSONObject task, boolean grouped) {
        long total = task.optLong("total");
        long done = Math.max(0, task.optLong("done"));
        int percent = Math.max(0, Math.min(100, task.optInt("percent")));
        if (total > 0) percent = (int) Math.min(100, 100.0 * done / total);
        Notification.Builder result = builder().setContentTitle(title(task))
                .setContentText(progressText(task)).setSubText(kind(task))
                .setProgress(100, percent, total <= 0 && percent == 0);
        if (grouped) result.setGroup(GROUP);
        return result.build();
    }

    private String title(JSONObject task) {
        String name = task.optString("name");
        return name.isEmpty() ? kind(task) : name;
    }

    private String kind(JSONObject task) {
        int text;
        switch (task.optString("kind")) {
            case "upload": text = R.string.notification_upload; break;
            case "download": text = R.string.notification_download; break;
            case "copy": text = R.string.notification_copy; break;
            case "extract": text = R.string.notification_extract; break;
            case "recover": text = R.string.notification_recover; break;
            default: text = R.string.notification_processing;
        }
        return service.getString(text);
    }

    private String progressText(JSONObject task) {
        if ("queued".equals(task.optString("state"))) return service.getString(R.string.notification_queued);
        int phase;
        switch (task.optString("phase")) {
            case "preparing": phase = R.string.notification_preparing; break;
            case "hashing": phase = R.string.notification_hashing; break;
            case "transferring": phase = R.string.notification_transferring; break;
            case "finalizing": phase = R.string.notification_finalizing; break;
            case "extracting": phase = R.string.notification_extract; break;
            default: phase = R.string.notification_processing;
        }
        long total = task.optLong("total");
        long done = Math.max(0, task.optLong("done"));
        String detail = "";
        if (total > 0) {
            String kind = task.optString("kind");
            detail = "upload".equals(kind) || "download".equals(kind)
                    ? service.getString(R.string.notification_progress_bytes,
                            Formatter.formatShortFileSize(service, done), Formatter.formatShortFileSize(service, total))
                    : service.getString(R.string.notification_progress_count, done, total);
        } else if (task.optInt("percent") > 0) {
            detail = service.getString(R.string.notification_progress_percent, Math.min(100, task.optInt("percent")));
        }
        String label = service.getString(phase);
        return detail.isEmpty() ? label : service.getString(R.string.notification_progress_phase, label, detail);
    }
}
