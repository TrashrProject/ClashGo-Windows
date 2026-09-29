import fs from 'node:fs';

const sources = {
  go: fs.readFileSync('cmd/account_proxy/main.go', 'utf8'),
  worker: fs.readFileSync('cloudflare/worker/src/index.js', 'utf8'),
};

const requiredRoutes = [
  '/v1/license/activate',
  '/v1/support/incidents',
  '/v1/developer/incidents',
  '/v1/developer/licenses',
  '/v1/developer/history',
  '/v1/developer/licenses/reset-machine',
  '/v1/developer/licenses/set-role',
  '/v1/developer/licenses/set-active',
  '/v1/developer/licenses/renew',
  '/v1/developer/licenses/update-customer',
];

let failed = false;
for (const route of requiredRoutes) {
  for (const [name, source] of Object.entries(sources)) {
    if (!source.includes(route)) {
      console.error(`Missing control API route in ${name}: ${route}`);
      failed = true;
    }
  }
}

const protections = [
  'admin license required',
  'cannot reset the active admin license machine',
  'cannot revoke the active admin license',
  'cannot remove admin role from the active admin license',
  'lifetime license cannot be downgraded by renewal',
];

for (const marker of protections) {
  for (const [name, source] of Object.entries(sources)) {
    if (!source.includes(marker)) {
      console.error(`Missing control API protection in ${name}: ${marker}`);
      failed = true;
    }
  }
}

const antiSharingFields = [
  'denied_activations',
  'last_denied_at',
  'last_denied_machine',
];

for (const field of antiSharingFields) {
  for (const [name, source] of Object.entries(sources)) {
    if (!source.includes(field)) {
      console.error(`Missing anti-sharing telemetry in ${name}: ${field}`);
      failed = true;
    }
  }
}

if (failed) process.exit(1);
console.log(`Control API parity OK: ${requiredRoutes.length} shared routes, ${protections.length} safety guards and ${antiSharingFields.length} anti-sharing fields.`);
