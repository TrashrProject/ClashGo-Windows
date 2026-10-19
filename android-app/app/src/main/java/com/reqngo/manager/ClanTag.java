package com.reqngo.manager;

import java.util.Locale;

/** Pure validation; never open a URL using untrusted free-form input. */
public final class ClanTag {
    private ClanTag() { }

    public static String normalize(String value) {
        if (value == null) throw new IllegalArgumentException("Identifiant manquant");
        String tag = value.trim().toUpperCase(Locale.ROOT);
        if (tag.startsWith("#")) tag = tag.substring(1);
        // Clash tags use restricted characters; preserve only well-known safe values.
        if (!tag.matches("[0289PYLQGRJCUV]{3,15}")) {
            throw new IllegalArgumentException("Tag invalide : utilise le code du clan, ex. #P0LY...");
        }
        return "#" + tag;
    }

    /** Deep link opens a clan PROFILE, never directly joins the clan. */
    public static String profileUrl(String value) {
        return "https://link.clashofclans.com/en?action=OpenClanProfile&tag=%23"
                + normalize(value).substring(1);
    }
}
