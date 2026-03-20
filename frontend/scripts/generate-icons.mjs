import { statSync, copyFileSync, mkdirSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import sharp from 'sharp';
import pngToIco from 'png-to-ico';
import { writeFileSync } from 'fs';

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = resolve(__dirname, '..');
const projectRoot = resolve(root, '..');

const source = resolve(root, 'src/assets/images/bruv-icon.svg');

const targets = [
  { path: resolve(root, 'public/bruv-icon.svg'), type: 'copy' },
  { path: resolve(root, 'public/bruv-icon.png'), type: 'png', size: 256 },
  { path: resolve(root, 'public/icon.ico'),      type: 'ico', sizes: [16, 32, 48, 64, 128, 256] },
  { path: resolve(projectRoot, 'build/appicon.png'),        type: 'png', size: 1024 },
  { path: resolve(projectRoot, 'build/windows/icon.ico'),   type: 'ico', sizes: [16, 32, 48, 64, 128, 256] },
];

function isStale() {
  let srcTime;
  try {
    srcTime = statSync(source).mtimeMs;
  } catch {
    console.error(`Source not found: ${source}`);
    process.exit(1);
  }
  return targets.some(t => {
    try { return statSync(t.path).mtimeMs < srcTime; }
    catch { return true; }
  });
}

async function generatePng(svgPath, outPath, size) {
  mkdirSync(dirname(outPath), { recursive: true });
  await sharp(svgPath, { density: 300 })
    .resize(size, size)
    .png()
    .toFile(outPath);
}

async function generateIco(svgPath, outPath, sizes) {
  mkdirSync(dirname(outPath), { recursive: true });
  const pngBuffers = await Promise.all(
    sizes.map(size =>
      sharp(svgPath, { density: 300 })
        .resize(size, size)
        .png()
        .toBuffer()
    )
  );
  const icoBuffer = await pngToIco(pngBuffers);
  writeFileSync(outPath, icoBuffer);
}

async function main() {
  if (!isStale()) {
    console.log('Icons up to date, skipping.');
    return;
  }

  console.log('Generating icons from SVG...');

  for (const target of targets) {
    switch (target.type) {
      case 'copy':
        mkdirSync(dirname(target.path), { recursive: true });
        copyFileSync(source, target.path);
        console.log(`  Copied → ${target.path}`);
        break;
      case 'png':
        await generatePng(source, target.path, target.size);
        console.log(`  PNG ${target.size}×${target.size} → ${target.path}`);
        break;
      case 'ico':
        await generateIco(source, target.path, target.sizes);
        console.log(`  ICO ${target.sizes.join(',')} → ${target.path}`);
        break;
    }
  }

  console.log('Done.');
}

main().catch(err => { console.error(err); process.exit(1); });
