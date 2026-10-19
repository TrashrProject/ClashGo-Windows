package com.reqngo.manager;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Locale;

/**
 * Logical profile scheduling only. No account tokens, Android session cloning,
 * root file copying, or login operations. One local lease at a time.
 */
public final class ProfileQueue {
    public enum Status { WAITING, ACTIVE, NEEDS_OPERATOR_LOGIN, VISUAL_DONE, BLOCKED }
    public static final int MAX_PROFILES = 5000;
    public static final long LEASE_MS = 120000L;

    public static final class Item {
        public final String id;
        public Status status;
        public long leaseUntilMs;
        Item(String id, Status status, long until) {
            this.id = id; this.status = status; this.leaseUntilMs = until;
        }
    }

    private final List<Item> entries = new ArrayList<>();
    public List<Item> items() { return Collections.unmodifiableList(entries); }

    public void add(String value) {
        String id = normalize(value);
        if (entries.size() >= MAX_PROFILES) throw new IllegalStateException("Capacité atteinte");
        for (Item item : entries) if (item.id.equals(id))
            throw new IllegalArgumentException("Profil déjà présent");
        entries.add(new Item(id, Status.WAITING, 0));
    }

    public static String normalize(String input) {
        if (input == null) throw new IllegalArgumentException("Profil manquant");
        String id = input.trim().toUpperCase(Locale.ROOT);
        if (!id.matches("[A-Z0-9_-]{1,24}"))
            throw new IllegalArgumentException("Identifiant logique : lettres, chiffres, _ ou -");
        return id;
    }

    public Item acquire(long now) {
        if (now <= 0) throw new IllegalArgumentException("Horodatage invalide");
        for (Item item : entries) {
            if (item.status == Status.ACTIVE || item.status == Status.NEEDS_OPERATOR_LOGIN) {
                if (item.leaseUntilMs > now) return null;
                item.status = Status.BLOCKED; // expired lease is never silently reused
                item.leaseUntilMs = 0;
            }
        }
        for (Item item : entries) {
            if (item.status == Status.WAITING) {
                item.status = Status.NEEDS_OPERATOR_LOGIN;
                item.leaseUntilMs = now + LEASE_MS;
                return item;
            }
        }
        return null;
    }

    public void operatorConfirmedLogin(String id, long now) {
        Item item = get(id);
        if (item.status != Status.NEEDS_OPERATOR_LOGIN || item.leaseUntilMs <= now)
            throw new IllegalStateException("Identification expirée ou non autorisée");
        for (Item other : entries) if (other != item
                && (other.status == Status.ACTIVE || other.status == Status.NEEDS_OPERATOR_LOGIN))
            throw new IllegalStateException("Autre session déjà réservée");
        item.status = Status.ACTIVE;
        item.leaseUntilMs = now + LEASE_MS;
    }

    public void finish(String id, boolean visuallyConfirmed) {
        Item item = get(id);
        if (item.status != Status.ACTIVE) throw new IllegalStateException("Bail inactif");
        item.status = visuallyConfirmed ? Status.VISUAL_DONE : Status.BLOCKED;
        item.leaseUntilMs = 0;
    }

    public void block(String id) {
        Item item = get(id);
        item.status = Status.BLOCKED; item.leaseUntilMs = 0;
    }

    public Item get(String id) {
        String normalized = normalize(id);
        for (Item x : entries) if (x.id.equals(normalized)) return x;
        throw new IllegalArgumentException("Profil inconnu");
    }

    public void reconcileAfterRestart() {
        for (Item item : entries) {
            if (item.status == Status.ACTIVE || item.status == Status.NEEDS_OPERATOR_LOGIN) {
                item.status = Status.BLOCKED; item.leaseUntilMs = 0;
            }
        }
    }

    /** Compact text serialization, no user credentials. */
    public String encode() {
        StringBuilder out = new StringBuilder("v1\n");
        for (Item item : entries) {
            out.append(item.id).append('|').append(item.status.name())
                    .append('|').append(item.leaseUntilMs).append('\n');
        }
        return out.toString();
    }

    public static ProfileQueue decode(String raw) {
        ProfileQueue queue = new ProfileQueue();
        if (raw == null || raw.equals("")) return queue;
        String[] lines = raw.split("\n", -1);
        if (lines.length < 1 || !lines[0].equals("v1") || lines.length > MAX_PROFILES + 2)
            throw new IllegalArgumentException("Fichier profils invalide");
        for (int i = 1; i < lines.length; i++) {
            if (lines[i].isEmpty()) continue;
            String[] parts = lines[i].split("\\|", -1);
            if (parts.length != 3) throw new IllegalArgumentException("Profil endommagé");
            queue.add(parts[0]);
            Item item = queue.entries.get(queue.entries.size() - 1);
            try {
                item.status = Status.valueOf(parts[1]);
                item.leaseUntilMs = Long.parseLong(parts[2]);
            } catch (RuntimeException e) { throw new IllegalArgumentException("Profil endommagé", e); }
            if (item.leaseUntilMs < 0) throw new IllegalArgumentException("Bail invalide");
        }
        return queue;
    }
}
