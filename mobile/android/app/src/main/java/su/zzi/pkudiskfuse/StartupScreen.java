package su.zzi.pkudiskfuse;

import android.animation.ValueAnimator;
import android.app.Activity;
import android.content.res.ColorStateList;
import android.graphics.Typeface;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.TextView;

/** Covers the WebView until its local service and first usable frame are ready. */
@android.annotation.SuppressLint("ViewConstructor") // Activity-owned overlay, never inflated from XML.
final class StartupScreen extends FrameLayout {
    private final TextView message;
    private final ProgressBar progress;
    private final LinearLayout actions;

    StartupScreen(Activity activity, Runnable retry, Runnable quit) {
        super(activity);
        setId(R.id.startup_screen);
        setBackgroundColor(activity.getColor(R.color.startup_background));
        setClickable(true);
        setFocusableInTouchMode(true);

        LinearLayout content = new LinearLayout(activity);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setGravity(Gravity.CENTER_HORIZONTAL);
        content.setPadding(dp(32), dp(24), dp(32), dp(24));
        addView(content, new FrameLayout.LayoutParams(-1, -2, Gravity.CENTER));

        ImageView icon = new ImageView(activity);
        icon.setImageResource(R.drawable.icon);
        icon.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        content.addView(icon, new LinearLayout.LayoutParams(dp(80), dp(80)));

        TextView title = new TextView(activity);
        title.setText(R.string.app_name);
        title.setTextSize(22);
        title.setTypeface(null, Typeface.BOLD);
        title.setTextColor(activity.getColor(R.color.startup_text));
        title.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams titleLayout = new LinearLayout.LayoutParams(-2, -2);
        titleLayout.topMargin = dp(20);
        content.addView(title, titleLayout);

        progress = new ProgressBar(activity);
        progress.setIndeterminateTintList(ColorStateList.valueOf(activity.getColor(R.color.startup_accent)));
        LinearLayout.LayoutParams progressLayout = new LinearLayout.LayoutParams(dp(28), dp(28));
        progressLayout.topMargin = dp(24);
        content.addView(progress, progressLayout);

        message = new TextView(activity);
        message.setTextSize(14);
        message.setTextColor(activity.getColor(R.color.startup_text));
        message.setGravity(Gravity.CENTER);
        message.setMaxWidth(dp(360));
        message.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        LinearLayout.LayoutParams messageLayout = new LinearLayout.LayoutParams(-2, -2);
        messageLayout.topMargin = dp(16);
        content.addView(message, messageLayout);

        actions = new LinearLayout(activity);
        actions.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams actionsLayout = new LinearLayout.LayoutParams(-2, -2);
        actionsLayout.topMargin = dp(20);
        content.addView(actions, actionsLayout);
        Button retryButton = new Button(activity);
        retryButton.setText(R.string.startup_retry);
        retryButton.setOnClickListener(view -> retry.run());
        actions.addView(retryButton);
        Button quitButton = new Button(activity);
        quitButton.setText(R.string.stop_service);
        quitButton.setOnClickListener(view -> quit.run());
        LinearLayout.LayoutParams quitLayout = new LinearLayout.LayoutParams(-2, -2);
        quitLayout.setMarginStart(dp(12));
        actions.addView(quitButton, quitLayout);
        showLoading();
    }

    void showLoading() {
        show();
        message.setText(R.string.intro);
        progress.setVisibility(VISIBLE);
        actions.setVisibility(GONE);
    }

    void showError(String text) {
        show();
        message.setText(text);
        progress.setVisibility(GONE);
        actions.setVisibility(VISIBLE);
    }

    void dismiss() {
        animate().cancel();
        if (!ValueAnimator.areAnimatorsEnabled()) {
            setVisibility(GONE);
            return;
        }
        animate().alpha(0f).setDuration(200).withEndAction(() -> setVisibility(GONE)).start();
    }

    private void show() {
        animate().cancel();
        setAlpha(1f);
        setVisibility(VISIBLE);
        bringToFront();
        requestFocus();
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
