import fs from 'node:fs';

const checks = [
  {
    file: 'web/src/components/Analytics.tsx',
    forbidden: [
      { re: /\battaques\s*:/g, label: 'internal object key "attaques:"' },
      { re: /\.attaques\b/g, label: 'internal property ".attaques"' },
      { re: /\battaquesPerHour\b/g, label: 'internal identifier "attaquesPerHour"' },
      { re: /\b(?:const|let|var)\s+attaques\b/g, label: 'internal variable "attaques"' },
    ],
  },
  {
    file: 'web/src/components/Dashboard.tsx',
    forbidden: [
      { re: /\?:\s*inconnu\b/g, label: 'TypeScript type "inconnu"' },
      { re: /\binconnu\s*:\s*['"]inconnu['"]/g, label: 'runtime key "inconnu" instead of "unknown"' },
    ],
  },
];

let failed = false;
for (const check of checks) {
  const source = fs.readFileSync(check.file, 'utf8');
  for (const item of check.forbidden) {
    const matches = [...source.matchAll(item.re)];
    if (matches.length === 0) continue;
    failed = true;
    for (const match of matches) {
      const before = source.slice(0, match.index ?? 0);
      const line = before.split('\n').length;
      console.error(`${check.file}:${line}: forbidden localized code token: ${item.label}`);
    }
  }
}

if (failed) {
  console.error('UI localization safety check failed. Translate labels, never runtime/type identifiers.');
  process.exit(1);
}

console.log('UI localization safety OK.');
