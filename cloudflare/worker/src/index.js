const JSON_HEADERS = {
  "content-type": "application/json; charset=utf-8",
  "cache-control": "no-store",
};

function json(data, status = 200, extraHeaders = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { ...JSON_HEADERS, ...extraHeaders },
  });
}

function normalizeRole(role) {
  const v = String(role || "").trim().toLowerCase();
  if (v === "developer") return "developer";
  if (v === "admin") return "admin";
  return "member";
}

function normalizePlan(plan) {
  const v = String(plan || "").trim().toLowerCase();
  if (v === "free_2d") return { plan: "free_2d", days: 2 };
  if (v === "week_1") return { plan: "week_1", days: 7 };
  if (v === "month_1") return { plan: "month_1", days: 30 };
  return { plan: "lifetime", days: null };
}

function expiryFromActivation(activatedAt, durationDays) {
  if (!activatedAt || !durationDays) return null;
  const start = new Date(activatedAt);
  return new Date(start.getTime() + Number(durationDays) * 24 * 60 * 60 * 1000).toISOString();
}

function isExpired(expiresAt) {
  if (!expiresAt) return false;
  const ts = Date.parse(expiresAt);
  return Number.isFinite(ts) && Date.now() >= ts;
}

function clean(value) {
  return String(value || "").trim();
}

function licenseHint(key) {
  const v = clean(key).toUpperCase();
  return v.length >= 4 ? "••••-" + v.slice(-4) : "";
}

function hex(bytes) {
  return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function sha256(value) {
  const data = new TextEncoder().encode(clean(value).toUpperCase());
  const digest = await crypto.subtle.digest("SHA-256", data);
  return hex(new Uint8Array(digest));
}

function randomHex(bytes = 8) {
  const buf = new Uint8Array(bytes);
  crypto.getRandomValues(buf);
  return hex(buf);
}

function newLicenseKey() {
  const raw = randomHex(12).toUpperCase();
  return `CGO-${raw.slice(0,6)}-${raw.slice(6,12)}-${raw.slice(12,18)}-${raw.slice(18,24)}`;
}

function safeFields(input) {
  if (!input || typeof input !== "object" || Array.isArray(input)) return {};
  const out = {};
  for (const [key, value] of Object.entries(input)) {
    const lower = key.toLowerCase();
    if (
      lower.includes("token") ||
      lower.includes("password") ||
      lower.includes("secret") ||
      lower.includes("license") ||
      lower.includes("authorization")
    ) continue;
    out[key] = value;
  }
  return out;
}

function corsHeaders(request, env) {
  const origin = clean(request.headers.get("Origin")).replace(/\/$/, "");
  const allowed = clean(env.CLASHGO_WEB_ORIGIN).replace(/\/$/, "");
  if (!origin || !allowed || origin !== allowed) return {};
  return {
    "access-control-allow-origin": allowed,
    "access-control-allow-headers": "Content-Type, X-ClashGO-Admin-Key, X-ClashGO-License, X-ClashGO-Machine",
    "access-control-allow-methods": "GET, POST, OPTIONS",
    "vary": "Origin",
  };
}

function responseWithCors(response, request, env) {
  const headers = new Headers(response.headers);
  for (const [k, v] of Object.entries(corsHeaders(request, env))) headers.set(k, v);
  return new Response(response.body, { status: response.status, headers });
}

function unauthorizedAdmin() {
  return json({ message: "admin authorization required" }, 401);
}

async function adminOK(request, env) {
  const expected = clean(env.CLASHGO_ADMIN_KEY);
  const actual = clean(request.headers.get("X-ClashGO-Admin-Key"));
  if (!expected || !actual || expected.length !== actual.length) return false;
  const a = new TextEncoder().encode(actual);
  const b = new TextEncoder().encode(expected);
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a[i] ^ b[i];
  return diff === 0;
}

async function readJSON(request, maxBytes = 262144) {
  const length = Number(request.headers.get("content-length") || "0");
  if (length > maxBytes) throw new Error("request too large");
  return await request.json();
}

async function findLicenseByKey(env, key) {
  const hash = await sha256(key);
  return await env.DB.prepare(
    "SELECT id, hint, role, active, machine_id, created_at, last_seen_at, app_version, plan, duration_days, activated_at, expires_at FROM licenses WHERE license_hash = ?1"
  ).bind(hash).first();
}

async function requireDeveloper(request, env) {
  const key = clean(request.headers.get("X-ClashGO-License"));
  const machine = clean(request.headers.get("X-ClashGO-Machine"));
  if (!key || !machine) return null;
  const license = await findLicenseByKey(env, key);
  if (!license || Number(license.active) !== 1 || isExpired(license.expires_at)) return null;
  if (license.role !== "developer" && license.role !== "admin") return null;
  if (!license.machine_id || license.machine_id !== machine) return null;
  return license;
}

async function activateLicense(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }

  const key = clean(body.license_key).toUpperCase();
  const machine = clean(body.machine_id);
  const appVersion = clean(body.app_version);
  if (!key || !machine) {
    return json({ message: "license key and machine id are required" }, 400);
  }

  const hash = await sha256(key);
  const license = await env.DB.prepare(`
    SELECT l.id, l.hint, l.role, l.active, l.machine_id, l.plan, l.duration_days,
           l.activated_at, l.expires_at, c.display_name AS member_name
    FROM licenses l
    LEFT JOIN customers c ON c.id = l.customer_id
    WHERE l.license_hash = ?1
  `).bind(hash).first();

  if (!license || Number(license.active) !== 1) {
    return json({ message: "license is invalid or revoked" }, 403);
  }
  if (isExpired(license.expires_at)) {
    return json({ message: "license has expired" }, 403);
  }
  if (license.machine_id && license.machine_id !== machine) {
    return json({ message: "license is already activated on another machine" }, 409);
  }

  const now = new Date();
  let activatedAt = license.activated_at;
  let expiresAt = license.expires_at;
  if (!activatedAt) {
    activatedAt = now.toISOString();
    expiresAt = expiryFromActivation(activatedAt, license.duration_days);
  }

  await env.DB.prepare(
    "UPDATE licenses SET machine_id = ?1, last_seen_at = ?2, app_version = ?3, activated_at = ?4, expires_at = ?5 WHERE id = ?6"
  ).bind(machine, now.toISOString(), appVersion, activatedAt, expiresAt, license.id).run();

  let offlineUntil = new Date(now.getTime() + 72 * 60 * 60 * 1000);
  if (expiresAt) {
    const expiry = new Date(expiresAt);
    if (expiry < offlineUntil) offlineUntil = expiry;
  }

  return json({
    ok: true,
    role: license.role,
    member_name: license.member_name || "",
    plan: license.plan || "lifetime",
    expires_at: expiresAt,
    offline_until: offlineUntil.toISOString(),
  });
}

async function ingestIncident(request, env) {
  const key = clean(request.headers.get("X-ClashGO-License"));
  if (!key) return json({ message: "valid license required" }, 401);

  const license = await findLicenseByKey(env, key);
  if (!license || Number(license.active) !== 1 || isExpired(license.expires_at)) {
    return json({ message: "valid license required" }, 401);
  }

  let body;
  try { body = await readJSON(request); }
  catch { return json({ message: "invalid incident" }, 400); }

  const machine = clean(body.machine_id);
  if (license.machine_id && machine && license.machine_id !== machine) {
    return json({ message: "machine mismatch" }, 409);
  }

  const message = clean(body.message).slice(0, 4000) || "Unknown ClashGO error";
  const levelRaw = clean(body.level).toLowerCase();
  const level = ["error","fatal","panic"].includes(levelRaw) ? levelRaw : "error";
  const id = randomHex(8);
  const received = new Date().toISOString();
  const fields = safeFields(body.fields);

  await env.DB.prepare(`
    INSERT INTO incidents (
      id, at, received_at, license_id, license_hint, role,
      machine_id, app_version, level, message, fields_json
    ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)
  `).bind(
    id,
    clean(body.at),
    received,
    license.id,
    license.hint,
    license.role,
    machine,
    clean(body.app_version),
    level,
    message,
    JSON.stringify(fields)
  ).run();

  return json({ ok: true, id }, 202);
}

async function listLicenses(env) {
  const result = await env.DB.prepare(`
    SELECT
      l.id, l.hint, l.role, l.active, l.machine_id, l.created_at, l.last_seen_at, l.app_version,
      l.plan, l.duration_days, l.activated_at, l.expires_at, l.customer_id,
      c.display_name AS customer_name, c.contact AS customer_contact
    FROM licenses l
    LEFT JOIN customers c ON c.id = l.customer_id
    ORDER BY l.created_at DESC LIMIT 1000
  `).all();
  return result.results || [];
}

async function listCustomers(env) {
  const result = await env.DB.prepare(`
    SELECT
      c.id, c.display_name, c.contact, c.notes, c.payment_status,
      c.total_paid_cents, c.next_due_at, c.created_at, c.updated_at,
      COUNT(l.id) AS license_count,
      SUM(CASE WHEN l.active = 1 THEN 1 ELSE 0 END) AS active_license_count,
      MAX(l.last_seen_at) AS last_seen_at
    FROM customers c
    LEFT JOIN licenses l ON l.customer_id = c.id
    GROUP BY c.id
    ORDER BY c.updated_at DESC
    LIMIT 1000
  `).all();
  return result.results || [];
}

async function createCustomer(body, env) {
  const name = clean(body.display_name);
  if (!name) return { error: json({ message: "display_name is required" }, 400) };
  const now = new Date().toISOString();
  const id = randomHex(8);
  await env.DB.prepare(`
    INSERT INTO customers (
      id, display_name, contact, notes, payment_status, total_paid_cents, next_due_at, created_at, updated_at
    ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?8)
  `).bind(
    id,
    name.slice(0, 120),
    clean(body.contact).slice(0, 180),
    clean(body.notes).slice(0, 1000),
    clean(body.payment_status) || "unknown",
    Math.max(0, Number(body.total_paid_cents || 0)),
    clean(body.next_due_at) || null,
    now
  ).run();
  return { customer: {
    id,
    display_name: name.slice(0,120),
    contact: clean(body.contact).slice(0,180),
    notes: clean(body.notes).slice(0,1000),
    payment_status: clean(body.payment_status) || "unknown",
    total_paid_cents: Math.max(0, Number(body.total_paid_cents || 0)),
    next_due_at: clean(body.next_due_at) || null,
    created_at: now,
    updated_at: now
  } };
}

async function createCustomerRequest(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }
  const result = await createCustomer(body, env);
  if (result.error) return result.error;
  return json({ customer: result.customer }, 201);
}

async function updateCustomer(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }
  const id = clean(body.customer_id);
  const name = clean(body.display_name);
  if (!id || !name) return json({ message: "customer_id and display_name are required" }, 400);
  const result = await env.DB.prepare(`
    UPDATE customers
    SET display_name = ?1,
        contact = ?2,
        notes = ?3,
        payment_status = ?4,
        total_paid_cents = ?5,
        next_due_at = ?6,
        updated_at = ?7
    WHERE id = ?8
  `).bind(
    name.slice(0,120),
    clean(body.contact).slice(0,180),
    clean(body.notes).slice(0,1000),
    clean(body.payment_status) || "unknown",
    Math.max(0, Number(body.total_paid_cents || 0)),
    clean(body.next_due_at) || null,
    new Date().toISOString(),
    id
  ).run();
  if (!result.meta?.changes) return json({ message: "customer not found" }, 404);
  return json({ ok: true });
}

async function listIncidents(env, limit = 500) {
  const result = await env.DB.prepare(`
    SELECT
      i.id, i.at, i.received_at, i.license_hint, i.role, i.machine_id,
      i.app_version, i.level, i.message,
      c.id AS customer_id, c.display_name AS customer_name, c.contact AS customer_contact
    FROM incidents i
    LEFT JOIN licenses l ON l.id = i.license_id
    LEFT JOIN customers c ON c.id = l.customer_id
    ORDER BY i.received_at DESC
    LIMIT ?1
  `).bind(limit).all();
  return result.results || [];
}

async function createLicenses(request, env) {
  let body = {};
  try { body = await readJSON(request, 65536); } catch {}
  const role = normalizeRole(body.role);
  const licensePlan = normalizePlan(body.plan);
  const count = Math.max(1, Math.min(100, Number(body.count || 1)));
  const created = [];
  const now = new Date().toISOString();

  let customerId = clean(body.customer_id);
  if (!customerId && clean(body.customer_name)) {
    const customerResult = await createCustomer({
      display_name: body.customer_name,
      contact: body.customer_contact,
      notes: body.customer_notes,
    }, env);
    if (customerResult.error) return customerResult.error;
    customerId = customerResult.customer.id;
  }
  if (customerId) {
    const exists = await env.DB.prepare("SELECT id FROM customers WHERE id = ?1").bind(customerId).first();
    if (!exists) return json({ message: "customer not found" }, 404);
  }

  for (let i = 0; i < count; i++) {
    const key = newLicenseKey();
    const hash = await sha256(key);
    const id = hash.slice(0, 16);
    await env.DB.prepare(`
      INSERT INTO licenses (
        id, license_hash, hint, role, active, created_at, plan, duration_days, customer_id
      ) VALUES (?1, ?2, ?3, ?4, 1, ?5, ?6, ?7, ?8)
    `).bind(id, hash, licenseHint(key), role, now, licensePlan.plan, licensePlan.days, customerId || null).run();
    await recordLicenseEvent(env, {
      licenseId: id,
      customerId: customerId || null,
      eventType: "created",
      plan: licensePlan.plan,
      paymentStatus: clean(body.payment_status) || null,
      amountCents: body.amount_cents == null ? null : Number(body.amount_cents),
      note: clean(body.note) || null,
    });
    created.push(key);
  }
  return json({ role, plan: licensePlan.plan, duration_days: licensePlan.days, customer_id: customerId || null, licenses: created }, 201);
}

async function recordLicenseEvent(env, {
  licenseId,
  customerId = null,
  eventType,
  plan = null,
  amountCents = null,
  paymentStatus = null,
  note = null,
  expiresAt = null,
}) {
  await env.DB.prepare(`
    INSERT INTO license_events (
      id, license_id, customer_id, event_type, plan,
      amount_cents, payment_status, note, created_at, expires_at
    ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
  `).bind(
    randomHex(8),
    licenseId,
    customerId || null,
    eventType,
    plan || null,
    amountCents == null ? null : Math.max(0, Number(amountCents)),
    paymentStatus || null,
    note ? String(note).slice(0, 1000) : null,
    new Date().toISOString(),
    expiresAt || null
  ).run();
}

async function listLicenseEvents(env, limit = 500) {
  const result = await env.DB.prepare(`
    SELECT
      e.id, e.license_id, e.customer_id, e.event_type, e.plan,
      e.amount_cents, e.payment_status, e.note, e.created_at, e.expires_at,
      l.hint AS license_hint,
      c.display_name AS customer_name,
      c.contact AS customer_contact
    FROM license_events e
    LEFT JOIN licenses l ON l.id = e.license_id
    LEFT JOIN customers c ON c.id = e.customer_id
    ORDER BY e.created_at DESC
    LIMIT ?1
  `).bind(limit).all();
  return result.results || [];
}

async function customerDetail(env, customerId) {
  const id = clean(customerId);
  if (!id) return null;

  const customer = await env.DB.prepare(`
    SELECT id, display_name, contact, notes, payment_status,
           total_paid_cents, next_due_at, created_at, updated_at
    FROM customers
    WHERE id = ?1
  `).bind(id).first();
  if (!customer) return null;

  const licenses = await env.DB.prepare(`
    SELECT id, hint, role, active, machine_id, created_at, last_seen_at,
           app_version, plan, duration_days, activated_at, expires_at
    FROM licenses
    WHERE customer_id = ?1
    ORDER BY created_at DESC
  `).bind(id).all();

  const history = await env.DB.prepare(`
    SELECT id, license_id, event_type, plan, amount_cents, payment_status,
           note, created_at, expires_at
    FROM license_events
    WHERE customer_id = ?1
    ORDER BY created_at DESC
    LIMIT 200
  `).bind(id).all();

  const incidents = await env.DB.prepare(`
    SELECT i.id, i.at, i.received_at, i.license_hint, i.role, i.machine_id,
           i.app_version, i.level, i.message
    FROM incidents i
    INNER JOIN licenses l ON l.id = i.license_id
    WHERE l.customer_id = ?1
    ORDER BY i.received_at DESC
    LIMIT 100
  `).bind(id).all();

  return {
    customer,
    licenses: licenses.results || [],
    history: history.results || [],
    incidents: incidents.results || [],
  };
}

async function customerDetailRequest(request, env) {
  const url = new URL(request.url);
  const id = clean(url.searchParams.get("id"));
  if (!id) return json({ message: "customer id is required" }, 400);
  const detail = await customerDetail(env, id);
  if (!detail) return json({ message: "customer not found" }, 404);
  return json(detail);
}

async function dashboardSummary(env) {
  const now = new Date();
  const in7 = new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000).toISOString();
  const nowISO = now.toISOString();

  const [customers, activeLicenses, expiredLicenses, expiringSoon, neverActivated, incidents, paidCustomers] = await Promise.all([
    env.DB.prepare("SELECT COUNT(*) AS n FROM customers").first(),
    env.DB.prepare("SELECT COUNT(*) AS n FROM licenses WHERE active = 1 AND (expires_at IS NULL OR expires_at > ?1)").bind(nowISO).first(),
    env.DB.prepare("SELECT COUNT(*) AS n FROM licenses WHERE expires_at IS NOT NULL AND expires_at <= ?1").bind(nowISO).first(),
    env.DB.prepare("SELECT COUNT(*) AS n FROM licenses WHERE expires_at IS NOT NULL AND expires_at > ?1 AND expires_at <= ?2").bind(nowISO, in7).first(),
    env.DB.prepare("SELECT COUNT(*) AS n FROM licenses WHERE activated_at IS NULL").first(),
    env.DB.prepare("SELECT COUNT(*) AS n FROM incidents").first(),
    env.DB.prepare("SELECT COUNT(*) AS n FROM customers WHERE payment_status = 'paid'").first(),
  ]);

  const errorsBySignature = await env.DB.prepare(`
    SELECT level, app_version, message, COUNT(*) AS count
    FROM incidents
    GROUP BY level, app_version, message
    ORDER BY count DESC, received_at DESC
    LIMIT 20
  `).all();

  const freeToPaid = await env.DB.prepare(`
    SELECT COUNT(DISTINCT customer_id) AS n
    FROM license_events
    WHERE customer_id IS NOT NULL
      AND event_type = 'renewal'
      AND plan IN ('week_1','month_1','lifetime')
  `).first();

  return {
    customers: Number(customers?.n || 0),
    active_licenses: Number(activeLicenses?.n || 0),
    expired_licenses: Number(expiredLicenses?.n || 0),
    expiring_7d: Number(expiringSoon?.n || 0),
    never_activated: Number(neverActivated?.n || 0),
    incidents: Number(incidents?.n || 0),
    paid_customers: Number(paidCustomers?.n || 0),
    free_to_paid_customers: Number(freeToPaid?.n || 0),
    top_errors: errorsBySignature.results || [],
  };
}

async function resetMachine(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }
  const id = clean(body.license_id);
  if (!id) return json({ message: "license_id is required" }, 400);
  const result = await env.DB.prepare(
    "UPDATE licenses SET machine_id = NULL, last_seen_at = NULL, app_version = NULL WHERE id = ?1"
  ).bind(id).run();
  if (!result.meta?.changes) return json({ message: "license not found" }, 404);
  return json({ ok: true });
}

async function revokeLicense(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }
  const id = clean(body.license_id);
  if (!id) return json({ message: "license_id is required" }, 400);
  const result = await env.DB.prepare(
    "UPDATE licenses SET active = 0 WHERE id = ?1"
  ).bind(id).run();
  if (!result.meta?.changes) return json({ message: "license not found" }, 404);
  return json({ ok: true });
}


async function setLicenseRole(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }
  const id = clean(body.license_id);
  const role = normalizeRole(body.role);
  if (!id) return json({ message: "license_id is required" }, 400);
  const result = await env.DB.prepare(
    "UPDATE licenses SET role = ?1 WHERE id = ?2"
  ).bind(role, id).run();
  if (!result.meta?.changes) return json({ message: "license not found" }, 404);
  return json({ ok: true, role });
}

async function setLicenseActive(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }
  const id = clean(body.license_id);
  const active = body.active === true ? 1 : 0;
  if (!id) return json({ message: "license_id is required" }, 400);
  const result = await env.DB.prepare(
    "UPDATE licenses SET active = ?1 WHERE id = ?2"
  ).bind(active, id).run();
  if (!result.meta?.changes) return json({ message: "license not found" }, 404);
  return json({ ok: true, active: active === 1 });
}


async function renewLicense(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }

  const id = clean(body.license_id);
  if (!id) return json({ message: "license_id is required" }, 400);

  const current = await env.DB.prepare(
    "SELECT id, plan, duration_days, expires_at, activated_at, active FROM licenses WHERE id = ?1"
  ).bind(id).first();
  if (!current) return json({ message: "license not found" }, 404);

  const requestedPlan = clean(body.plan);
  const planInfo = requestedPlan ? normalizePlan(requestedPlan) : normalizePlan(current.plan);

  if (planInfo.plan === "lifetime") {
    await env.DB.prepare(
      "UPDATE licenses SET plan = 'lifetime', duration_days = NULL, expires_at = NULL, active = 1 WHERE id = ?1"
    ).bind(id).run();

    const licenseOwner = await env.DB.prepare("SELECT customer_id FROM licenses WHERE id = ?1").bind(id).first();
    const customerId = licenseOwner?.customer_id || null;
    const amountCents = body.amount_cents == null ? null : Math.max(0, Number(body.amount_cents));
    const paymentStatus = clean(body.payment_status) || null;

    await recordLicenseEvent(env, {
      licenseId: id,
      customerId,
      eventType: "renewal",
      plan: "lifetime",
      amountCents,
      paymentStatus,
      note: clean(body.note) || null,
      expiresAt: null,
    });

    if (customerId) {
      await env.DB.prepare(`
        UPDATE customers
        SET payment_status = COALESCE(?1, payment_status),
            total_paid_cents = total_paid_cents + ?2,
            next_due_at = NULL,
            updated_at = ?3
        WHERE id = ?4
      `).bind(paymentStatus, amountCents || 0, new Date().toISOString(), customerId).run();
    }

    return json({ ok: true, plan: "lifetime", expires_at: null });
  }

  const now = new Date();
  const currentExpiry = current.expires_at ? new Date(current.expires_at) : null;
  const base = currentExpiry && currentExpiry > now ? currentExpiry : now;
  const expiresAt = new Date(base.getTime() + planInfo.days * 24 * 60 * 60 * 1000).toISOString();

  await env.DB.prepare(
    "UPDATE licenses SET plan = ?1, duration_days = ?2, expires_at = ?3, active = 1 WHERE id = ?4"
  ).bind(planInfo.plan, planInfo.days, expiresAt, id).run();

  const licenseOwner = await env.DB.prepare("SELECT customer_id FROM licenses WHERE id = ?1").bind(id).first();
  const customerId = licenseOwner?.customer_id || null;
  const amountCents = body.amount_cents == null ? null : Math.max(0, Number(body.amount_cents));
  const paymentStatus = clean(body.payment_status) || null;

  await recordLicenseEvent(env, {
    licenseId: id,
    customerId,
    eventType: "renewal",
    plan: planInfo.plan,
    amountCents,
    paymentStatus,
    note: clean(body.note) || null,
    expiresAt,
  });

  if (customerId) {
    await env.DB.prepare(`
      UPDATE customers
      SET payment_status = COALESCE(?1, payment_status),
          total_paid_cents = total_paid_cents + ?2,
          next_due_at = ?3,
          updated_at = ?4
      WHERE id = ?5
    `).bind(
      paymentStatus,
      amountCents || 0,
      expiresAt,
      new Date().toISOString(),
      customerId
    ).run();
  }

  return json({
    ok: true,
    plan: planInfo.plan,
    duration_days: planInfo.days,
    expires_at: expiresAt,
  });
}

async function assignCustomer(request, env) {
  let body;
  try { body = await readJSON(request, 65536); }
  catch { return json({ message: "invalid request" }, 400); }
  const licenseId = clean(body.license_id);
  const customerId = clean(body.customer_id);
  if (!licenseId) return json({ message: "license_id is required" }, 400);
  if (customerId) {
    const exists = await env.DB.prepare("SELECT id FROM customers WHERE id = ?1").bind(customerId).first();
    if (!exists) return json({ message: "customer not found" }, 404);
  }
  const result = await env.DB.prepare(
    "UPDATE licenses SET customer_id = ?1 WHERE id = ?2"
  ).bind(customerId || null, licenseId).run();
  if (!result.meta?.changes) return json({ message: "license not found" }, 404);
  return json({ ok: true, customer_id: customerId || null });
}

async function router(request, env) {
  const url = new URL(request.url);
  const path = url.pathname;

  if (request.method === "OPTIONS") {
    const headers = corsHeaders(request, env);
    if (!headers["access-control-allow-origin"]) return new Response(null, { status: 403 });
    return new Response(null, { status: 204, headers });
  }

  if (request.method === "GET" && path === "/healthz") {
    return json({ ok: true, service: "clashgo-control-api" });
  }

  if (request.method === "POST" && path === "/v1/license/activate") {
    return activateLicense(request, env);
  }

  if (request.method === "POST" && path === "/v1/support/incidents") {
    return ingestIncident(request, env);
  }

  if (path.startsWith("/v1/admin/")) {
    if (!(await adminOK(request, env))) return unauthorizedAdmin();

    if (request.method === "GET" && path === "/v1/admin/licenses") {
      return json({ licenses: await listLicenses(env) });
    }
    if (request.method === "GET" && path === "/v1/admin/incidents") {
      return json({ incidents: await listIncidents(env, 500) });
    }
    if (request.method === "GET" && path === "/v1/admin/customers") {
      return json({ customers: await listCustomers(env) });
    }
    if (request.method === "GET" && path === "/v1/admin/customer") {
      return customerDetailRequest(request, env);
    }
    if (request.method === "GET" && path === "/v1/admin/history") {
      return json({ events: await listLicenseEvents(env, 500) });
    }
    if (request.method === "GET" && path === "/v1/admin/summary") {
      return json(await dashboardSummary(env));
    }
    if (request.method === "POST" && path === "/v1/admin/customers") {
      return createCustomerRequest(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/customers/update") {
      return updateCustomer(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/licenses") {
      return createLicenses(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/licenses/reset-machine") {
      return resetMachine(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/licenses/revoke") {
      return revokeLicense(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/licenses/set-role") {
      return setLicenseRole(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/licenses/set-active") {
      return setLicenseActive(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/licenses/renew") {
      return renewLicense(request, env);
    }
    if (request.method === "POST" && path === "/v1/admin/licenses/assign-customer") {
      return assignCustomer(request, env);
    }
  }

  if (path.startsWith("/v1/developer/")) {
    const dev = await requireDeveloper(request, env);
    if (!dev) return json({ message: "developer license required" }, 403);

    if (request.method === "GET" && path === "/v1/developer/licenses") {
      return json({ licenses: await listLicenses(env) });
    }
    if (request.method === "GET" && path === "/v1/developer/incidents") {
      return json({ incidents: await listIncidents(env, 500) });
    }
    if (request.method === "POST" && path === "/v1/developer/licenses") {
      if (dev.role !== "admin") {
        return json({ message: "admin license required" }, 403);
      }
      return createLicenses(request, env);
    }
    if (dev.role === "admin" && request.method === "POST" && path === "/v1/developer/licenses/reset-machine") {
      return resetMachine(request, env);
    }
    if (dev.role === "admin" && request.method === "POST" && path === "/v1/developer/licenses/set-active") {
      return setLicenseActive(request, env);
    }
    if (dev.role === "admin" && request.method === "POST" && path === "/v1/developer/licenses/renew") {
      return renewLicense(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/reset-machine") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return resetMachine(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/set-role") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return setLicenseRole(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/set-active") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return setLicenseActive(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/renew") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return renewLicense(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/reset-machine") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return resetMachine(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/set-active") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return setLicenseActive(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/set-role") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return setLicenseRole(request, env);
    }
    if (request.method === "POST" && path === "/v1/developer/licenses/renew") {
      if (dev.role !== "admin") return json({ message: "admin license required" }, 403);
      return renewLicense(request, env);
    }
  }

  return json({ message: "not found" }, 404);
}

export default {
  async fetch(request, env) {
    try {
      const response = await router(request, env);
      return responseWithCors(response, request, env);
    } catch (error) {
      console.error("Unhandled ClashGO API error", error);
      return responseWithCors(
        json({ message: "internal server error" }, 500),
        request,
        env
      );
    }
  },
};
