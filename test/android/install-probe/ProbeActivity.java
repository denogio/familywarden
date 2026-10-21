package io.github.denogio.familywarden.fixture;

import android.app.Activity;
import android.os.Bundle;
import android.os.SystemClock;
import android.util.Log;
import android.widget.TextView;

/** Synthetic target only: successful execution is evidence of a launch window. */
public class ProbeActivity extends Activity {
    @Override public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        Log.i("FwInstallFixture", "CREATED uptime=" + SystemClock.uptimeMillis());
        TextView view = new TextView(this);
        view.setText("Synthetic install/usage fixture");
        setContentView(view);
    }
}
