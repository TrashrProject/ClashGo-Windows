export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders() });
    }

    if (url.pathname === "/health") {
      return json({ ok: true, service: "clashgo-telemetry", time: new Date().toISOString() });
    }

    if (url.pathname === "/v1/logs" && request.method === "POST") {
      return ingest(request, env);
    }

    if (url.pathname === "/admin/summary" && request.method === "GET") {
      if (!adminAuthorized(request, env)) return unauthorized();
      return summary(env);
    }

    if (url.pathname === "/admin/recent" && request.method === "GET") {
      if (!adminAuthorized(request, env)) return unauthorized();
      return recent(url, env);
    }

    return new Response("not found", { status: 404 });
  },
};

function corsHeaders() {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Headers": "content-type, authorization, x-clashgo-ingest",
    "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
  };
}

function json(value, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", ...corsHeaders() },
  });
}

function unauthorized() {
  return json({ error: "unauthorized" }, 401);
}

function constantTimeLike(a, b) {
  // Workers JS has no portable constant-time string helper. Admin and ingest
  // secrets are compared server-side over TLS and never returned.
  return typeof a === "string" && typeof b === "string" && a.length === b.length && a === b;
}

function adminAuthorized(request, env) {
  const got = (request.headers.get("authorization") || "").replace(/^Bearer\s+/i, "");
  return !!env.ADMIN_TOKEN && constantTimeLike(got, env.ADMIN_TOKEN);
}

function ingestAuthorized(request, env) {
  if (!env.INGEST_TOKEN) return true;
  const got = request.headers.get("x-clashgo-ingest") || "";
  return constantTimeLike(got, env.INGEST_TOKEN);
}

function cleanString(value, max = 4096) {
  if (typeof value !== "string") return "";
  return value.replace(/[\u0000-\u001f\u007f]/g, " ").slice(0, max);
}

function normalizeLevel(v) {
  const s = cleanString(v, 16).toLowerCase();
  return ["debug", "info", "warn", "error", "fatal", "panic"].includes(s) ? s : "info";
}

function fingerprint(message) {
  return cleanString(message, 512)
    .toLowerCase()
    .replace(/0x[0-9a-f]+/g, "<hex>")
    .replace(/\b\d{2,}\b/g, "<n>")
    .replace(/[a-f0-9]{24,}/g, "<id>")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 320);
}

async function ingest(request, env) {
  if (!ingestAuthorized(request, env)) return unauthorized();

  const len = Number(request.headers.get("content-length") || "0");
  if (len > 2_000_000) return json({ error: "payload too large" }, 413);

  let body;
  try {
    body = await request.json();
  } catch {
    return json({ error: "invalid json" }, 400);
  }

  if (!body || !Array.isArray(body.events) || body.events.length < 1 || body.events.length > 256) {
    return json({ error: "invalid batch" }, 400);
  }

  const statements = [];
  const now = new Date().toISOString();

  for (const raw of body.events) {
    const installId = cleanString(raw?.install_id, 80);
    const version = cleanString(raw?.version, 80);
    const os = cleanString(raw?.os, 32);
    const arch = cleanString(raw?.arch, 32);
    const sentAt = cleanString(raw?.sent_at, 64) || now;
    const log = raw?.log && typeof raw.log === "object" ? raw.log : {};
    const level = normalizeLevel(log.level);
    const message = cleanString(log.message, 4096);
    if (!installId || !message) continue;

    const fp = fingerprint(message);
    const safeLog = JSON.stringify(log).slice(0, 16000);

    statements.push(
      env.DB.prepare(`
        INSERT INTO events
          (install_id, version, os, arch, sent_at, received_at, level, message, fingerprint, payload_json)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `).bind(installId, version, os, arch, sentAt, now, level, message, fp, safeLog)
    );
  }

  if (!statements.length) return json({ accepted: 0 });
  await env.DB.batch(statements);
  return json({ accepted: statements.length }, 202);
}

async function summary(env) {
  const [installs, versions, errors] = await Promise.all([
    env.DB.prepare("SELECT COUNT(DISTINCT install_id) AS n FROM events WHERE received_at >= datetime('now','-30 day')").first(),
    env.DB.prepare(`
      SELECT version, COUNT(DISTINCT install_id) AS installations, COUNT(*) AS events
      FROM events
      WHERE received_at >= datetime('now','-30 day')
      GROUP BY version
      ORDER BY installations DESC, events DESC
      LIMIT 30
    `).all(),
    env.DB.prepare(`
      SELECT fingerprint, MAX(message) AS example, level,
             COUNT(*) AS occurrences,
             COUNT(DISTINCT install_id) AS installations,
             MAX(received_at) AS last_seen
      FROM events
      WHERE level IN ('warn','error','fatal','panic')
        AND received_at >= datetime('now','-30 day')
      GROUP BY fingerprint, level
      ORDER BY occurrences DESC
      LIMIT 100
    `).all(),
  ]);

  return json({
    generated_at: new Date().toISOString(),
    installations_30d: Number(installs?.n || 0),
    versions: versions.results || [],
    top_errors: errors.results || [],
  });
}

async function recent(url, env) {
  const limit = Math.min(Math.max(Number(url.searchParams.get("limit") || "100"), 1), 500);
  const rows = await env.DB.prepare(`
    SELECT id, install_id, version, os, arch, sent_at, received_at, level, message, fingerprint
    FROM events
    ORDER BY id DESC
    LIMIT ?
  `).bind(limit).all();
  return json({ events: rows.results || [] });
}
