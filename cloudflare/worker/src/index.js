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
  const license = await env.DB.prepare(
    "SELECT id, hint, role, active, machine_id, plan, duration_days, activated_at, expires_at FROM licenses WHERE license_hash = ?1"
  ).bind(hash).first();

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
      c.id, c.display_name, c.contact, c.notes, c.created_at, c.updated_at,
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
    INSERT INTO customers (id, display_name, contact, notes, created_at, updated_at)
    VALUES (?1, ?2, ?3, ?4, ?5, ?5)
  `).bind(id, name.slice(0, 120), clean(body.contact).slice(0, 180), clean(body.notes).slice(0, 1000), now).run();
  return { customer: { id, display_name: name.slice(0,120), contact: clean(body.contact).slice(0,180), notes: clean(body.notes).slice(0,1000), created_at: now, updated_at: now } };
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
    SET display_name = ?1, contact = ?2, notes = ?3, updated_at = ?4
    WHERE id = ?5
  `).bind(name.slice(0,120), clean(body.contact).slice(0,180), clean(body.notes).slice(0,1000), new Date().toISOString(), id).run();
  if (!result.meta?.changes) return json({ message: "customer not found" }, 404);
  return json({ ok: true });
}

async function listIncidents(env, limit = 500) {
  const result = await env.DB.prepare(`
    SELECT id, at, received_at, license_hint, role, machine_id, app_version, level, message
    FROM incidents ORDER BY received_at DESC LIMIT ?1
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
    created.push(key);
  }
  return json({ role, plan: licensePlan.plan, duration_days: licensePlan.days, customer_id: customerId || null, licenses: created }, 201);
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
    return json({ ok: true, plan: "lifetime", expires_at: null });
  }

  const now = new Date();
  const currentExpiry = current.expires_at ? new Date(current.expires_at) : null;
  const base = currentExpiry && currentExpiry > now ? currentExpiry : now;
  const expiresAt = new Date(base.getTime() + planInfo.days * 24 * 60 * 60 * 1000).toISOString();

  await env.DB.prepare(
    "UPDATE licenses SET plan = ?1, duration_days = ?2, expires_at = ?3, active = 1 WHERE id = ?4"
  ).bind(planInfo.plan, planInfo.days, expiresAt, id).run();

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
