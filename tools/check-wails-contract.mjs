import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const appPath = path.join(root, 'app.go');
const webSrc = path.join(root, 'web', 'src');

const appSource = fs.readFileSync(appPath, 'utf8');
const appMethods = new Set();
for (const match of appSource.matchAll(/func\s+\(a\s+\*App\)\s+([A-Z][A-Za-z0-9_]*)\s*\(/g)) {
  appMethods.add(match[1]);
}

const files = [];
const walk = (dir) => {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full);
    else if (entry.isFile() && /\.(ts|tsx)$/.test(entry.name)) files.push(full);
  }
};
walk(webSrc);

const importRE = /import\s*\{([^}]*)\}\s*from\s*['"][^'"]*wailsjs\/go\/main\/App['"]/gs;
const missing = [];

for (const file of files) {
  const source = fs.readFileSync(file, 'utf8');
  for (const match of source.matchAll(importRE)) {
    for (const raw of match[1].split(',')) {
      let name = raw.trim();
      if (!name) continue;
      name = name.split(/\s+as\s+/)[0].trim();
      if (!/^[A-Za-z_$][A-Za-z0-9_$]*$/.test(name)) continue;
      if (!appMethods.has(name)) {
        missing.push(path.relative(root, file) + ': ' + name);
      }
    }
  }
}

if (missing.length) {
  missing.sort();
  console.error('Frontend imports Wails App methods not exposed by Go:');
  for (const item of missing) console.error(' - ' + item);
  process.exit(1);
}

console.log('Wails App contract OK: ' + appMethods.size + ' exported App methods scanned.');
