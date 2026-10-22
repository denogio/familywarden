package io.github.denogio.familywarden.fixture;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.SystemClock;
import android.util.Log;

/** Distinguishes an activity launch barrier from a blanket code-execution barrier. */
public class ProbeProvider extends ContentProvider {
    @Override public boolean onCreate() {
        Log.i("FwProviderFixture", "PROVIDER_CREATED uptime=" + SystemClock.uptimeMillis());
        return true;
    }
    @Override public Cursor query(Uri uri, String[] projection, String selection, String[] args, String order) {
        MatrixCursor result = new MatrixCursor(new String[] {"value"});
        result.addRow(new Object[] {"synthetic"});
        return result;
    }
    @Override public String getType(Uri uri) { return "vnd.android.cursor.item/synthetic"; }
    @Override public Uri insert(Uri uri, ContentValues values) { throw new UnsupportedOperationException(); }
    @Override public int delete(Uri uri, String selection, String[] args) { throw new UnsupportedOperationException(); }
    @Override public int update(Uri uri, ContentValues values, String selection, String[] args) { throw new UnsupportedOperationException(); }
}
