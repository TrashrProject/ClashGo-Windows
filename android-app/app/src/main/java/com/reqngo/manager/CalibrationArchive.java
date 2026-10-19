package com.reqngo.manager;

import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.BitmapFactory;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;
import java.util.zip.ZipOutputStream;

/**
 * Explicit user-created local backup of calibration and reference PNG images.
 *
 * Never exports login data, fleet IDs, clan tags, or active gesture states.
 * The ZIP is NOT encrypted and may contain visible in-game player names.
 * Import is bounded, allowlisted and validated before replacing live data.
 */
public final class CalibrationArchive {
    private static final String MANIFEST = "reqngo-calibration-v1.json";
    private static final int MAX_ENTRY = 12 * 1024 * 1024;
    private static final int MAX_TOTAL = 40 * 1024 * 1024;
    private static final int MAX_FILES = ScreenCatalog.IDS.length + 2;
    private static final String CURRENT = "latest_game_capture.png";

    private CalibrationArchive() { }

    private static SharedPreferences preferences(Context ctx) {
        return ctx.getSharedPreferences("local_preferences", Context.MODE_PRIVATE);
    }

    private static boolean allowedImage(String name) {
        if (CURRENT.equals(name)) return true;
        for (String id : ScreenCatalog.IDS)
            if (("reference_" + id + ".png").equals(name)) return true;
        return false;
    }

    public static int referenceCount(Context context) {
        int count = 0;
        for (String id : ScreenCatalog.IDS) {
            if (ScreenCatalog.referenceFile(context, id).isFile()) count++;
        }
        return count;
    }

    public static int flowReferenceCount(Context context) {
        int count = 0;
        for (int i = 0; i < GuidedRoute.SIZE + 1; i++)
            if (ScreenCatalog.referenceFile(context, ScreenCatalog.IDS[i]).isFile()) count++;
        return count;
    }

    public static void exportTo(Context context, OutputStream output) throws Exception {
        if (flowReferenceCount(context) == 0)
            throw new IllegalStateException("Aucune référence à sauvegarder");
        JSONObject settings = new JSONObject();
        settings.put("format", 1);
        SharedPreferences p = preferences(context);
        for (String id : ScreenCatalog.IDS) {
            String x = "roi_" + id + "_x", y = "roi_" + id + "_y";
            if (p.contains(x) && p.contains(y)) {
                settings.put(x, (double) p.getFloat(x, -1f));
                settings.put(y, (double) p.getFloat(y, -1f));
            }
        }
        for (int index = 0; index < GuidedRoute.SIZE; index++) {
            String key = "route_target_" + index;
            if (p.contains(key)) settings.put(key, p.getString(key, ""));
        }
        // Never include session credentials, account names, clan tags or task state.
        ZipOutputStream zip = new ZipOutputStream(output);
        ZipEntry manifest = new ZipEntry(MANIFEST);
        zip.putNextEntry(manifest);
        byte[] json = settings.toString().getBytes(java.nio.charset.StandardCharsets.UTF_8);
        zip.write(json);
        zip.closeEntry();
        File current = new File(context.getFilesDir(), CURRENT);
        if (current.isFile()) append(zip, current, CURRENT);
        for (String id : ScreenCatalog.IDS) {
            File reference = ScreenCatalog.referenceFile(context, id);
            if (reference.isFile()) append(zip, reference, reference.getName());
        }
        zip.finish();
        zip.flush();
    }

    private static void append(ZipOutputStream zip, File source, String filename) throws Exception {
        if (source.length() < 1 || source.length() > MAX_ENTRY)
            throw new IllegalStateException("Image trop volumineuse pour sauvegarde : " + filename);
        zip.putNextEntry(new ZipEntry(filename));
        try (FileInputStream in = new FileInputStream(source)) {
            byte[] data = new byte[8192];
            int n;
            while ((n = in.read(data)) != -1) zip.write(data, 0, n);
        }
        zip.closeEntry();
    }

    public static int importFrom(Context context, InputStream input) throws Exception {
        Map<String, byte[]> items = new LinkedHashMap<>();
        int sum = 0;
        int files = 0;
        ZipInputStream zip = new ZipInputStream(input);
        ZipEntry entry;
        while ((entry = zip.getNextEntry()) != null) {
            if (entry.isDirectory() || (!entry.getName().equals(MANIFEST)
                    && !allowedImage(entry.getName()))) {
                throw new IllegalArgumentException("Fichier inconnu ou répertoire interdit dans l'archive");
            }
            if (++files > MAX_FILES || items.containsKey(entry.getName()))
                throw new IllegalArgumentException("Archive trop grande ou fichier en double");
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            byte[] buffer = new byte[8192];
            int n;
            while ((n = zip.read(buffer)) != -1) {
                sum += n;
                if (bytes.size() + n > MAX_ENTRY || sum > MAX_TOTAL)
                    throw new IllegalArgumentException("Archive dépassant les limites autorisées");
                bytes.write(buffer, 0, n);
            }
            items.put(entry.getName(), bytes.toByteArray());
            zip.closeEntry();
        }

        byte[] manifest = items.remove(MANIFEST);
        if (manifest == null || manifest.length > 12000)
            throw new IllegalArgumentException("Métadonnées de calibration absentes");
        JSONObject metadata = new JSONObject(new String(manifest,
                java.nio.charset.StandardCharsets.UTF_8));
        if (metadata.optInt("format", -1) != 1)
            throw new IllegalArgumentException("Version d'archive incompatible");

        int count = 0;
        for (Map.Entry<String, byte[]> item : items.entrySet()) {
            BitmapFactory.Options opt = new BitmapFactory.Options();
            opt.inJustDecodeBounds = true;
            BitmapFactory.decodeByteArray(item.getValue(), 0, item.getValue().length, opt);
            if (opt.outWidth < 20 || opt.outHeight < 20
                    || opt.outWidth > 4096 || opt.outHeight > 4096
                    || !"image/png".equals(opt.outMimeType))
                throw new IllegalArgumentException("Image de référence invalide : " + item.getKey());
            if (!CURRENT.equals(item.getKey())) count++;
        }
        if (count == 0) throw new IllegalArgumentException("Aucune référence à importer");

        // Stage every image before updating any live reference.
        File directory = context.getFilesDir();
        for (Map.Entry<String, byte[]> item : items.entrySet()) {
            File stage = new File(directory, item.getKey() + ".importing");
            try (FileOutputStream out = new FileOutputStream(stage, false)) {
                out.write(item.getValue());
                out.getFD().sync();
            }
        }
        // The storage directory is app-private and filenames are allowlisted.
        for (String key : items.keySet()) {
            File stage = new File(directory, key + ".importing");
            File finalImage = new File(directory, key);
            if (finalImage.exists() && !finalImage.delete())
                throw new IllegalStateException("Ancienne référence verrouillée : " + key);
            if (!stage.renameTo(finalImage))
                throw new IllegalStateException("Impossible de restaurer : " + key);
        }

        SharedPreferences.Editor editor = preferences(context).edit();
        for (String id : ScreenCatalog.IDS) {
            editor.remove("roi_" + id + "_x").remove("roi_" + id + "_y");
            String x = "roi_" + id + "_x", y = "roi_" + id + "_y";
            if (metadata.has(x) && metadata.has(y)) {
                double vx = metadata.optDouble(x, -1), vy = metadata.optDouble(y, -1);
                if (Double.isFinite(vx) && Double.isFinite(vy)
                        && vx > .10 && vx < .90 && vy > .10 && vy < .90) {
                    editor.putFloat(x, (float) vx).putFloat(y, (float) vy);
                }
            }
        }
        for (int i = 0; i < GuidedRoute.SIZE; i++) {
            String key = "route_target_" + i;
            editor.remove(key);
            if (metadata.has(key)) {
                try {
                    RouteTarget point = RouteTarget.decode(metadata.optString(key));
                    if (point.step == i && point.encode().length() < 160) {
                        editor.putString(key, point.encode());
                    }
                } catch (RuntimeException ignored) { /* invalid calibration is discarded */ }
            }
        }
        // NEVER restore stale armed gestures, automated runs, account states or leases.
        editor.putString("journal", "BLOCKED")
                .putInt("route_index", 0).putString("route_phase", "BLOCKED")
                .putString("auto_phase", "BLOCKED").putInt("auto_index", 0)
                .remove("auto_profile_id").remove("route_started_ms")
                .remove("comparison");
        if (items.containsKey(CURRENT)) {
            File image = new File(directory, CURRENT);
            BitmapFactory.Options options = new BitmapFactory.Options();
            options.inJustDecodeBounds = true;
            BitmapFactory.decodeFile(image.getAbsolutePath(), options);
            editor.putInt("capture_width", options.outWidth)
                    .putInt("capture_height", options.outHeight)
                    .putLong("capture_time", 0L);
        } else {
            editor.remove("capture_time").remove("capture_width")
                    .remove("capture_height");
        }
        if (!editor.commit())
            throw new IllegalStateException("Images présentes, mais préférences non sauvegardées");
        return count;
    }
}
