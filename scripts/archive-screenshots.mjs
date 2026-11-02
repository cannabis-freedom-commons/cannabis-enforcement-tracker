// Convert private browser captures in place; never point this at site assets.
import fs from 'node:fs/promises';
import path from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { randomUUID } from 'node:crypto';
import { pathToFileURL } from 'node:url';
import { zstdCompress, zstdDecompress, constants } from 'node:zlib';
const run = promisify(execFile);
const controller = new AbortController();

export async function archiveScreenshot(file) {
  const target = file.replace(/\.png$/i, '.avif');
  if (target === file) throw new Error('Expected PNG capture');
  const lock = await fs.open(file + '.archive-lock', 'wx', 0o600).catch(e => {
    if (e.code === 'EEXIST' || e.code === 'ENOENT') return null;
    throw e;
  });
  if (!lock) return null;
  const temp = path.join(path.dirname(file), '.' + randomUUID() + '.avif');
  try {
    const before = await fs.lstat(file).catch(e => { if (e.code === 'ENOENT') return null; throw e; });
    if (!before?.isFile()) return null;
    await run('magick', [file, '-resize', '16383x16383>', '-quality', '25', '-define', 'heic:speed=8', temp], {
      timeout: 90000, signal: controller.signal, env: { ...process.env, MAGICK_THREAD_LIMIT: '1' }, maxBuffer: 65536,
    });
    const { stdout } = await run('magick', ['identify', '-format', '%m %w %h', temp], { timeout: 30000, signal: controller.signal });
    const [format, width, height] = stdout.trim().split(/\s+/);
    if (!['AVIF', 'HEIC'].includes(format) || !(+width > 0 && +width <= 16383 && +height > 0 && +height <= 16383)) throw new Error('Invalid AVIF output');
    await fs.chmod(temp, before.mode & 0o777);
    await fs.utimes(temp, before.atime, before.mtime);
    const after = await fs.lstat(file);
    if (after.size !== before.size || after.mtimeMs !== before.mtimeMs) throw new Error('Capture changed during conversion');
    const size = (await fs.stat(temp)).size;
    await fs.rename(temp, target);
    await fs.unlink(file);
    return { before: before.size, after: size };
  } finally {
    await fs.rm(temp, { force: true });
    await lock.close();
    await fs.rm(file + '.archive-lock', { force: true });
  }
}
// HTML is evidence: verify a byte-for-byte round trip before replacing it.
const textArtifact = name => name === 'page.html' || name === 'research-events.jsonl' || /^evidence-\d+-[a-f0-9]+\.json$/.test(name);
export async function archiveHtml(file) {
  if (path.basename(file) !== 'page.html') throw new Error('Expected page.html capture');
  return archiveText(file);
}
export async function archiveText(file) {
  if (!textArtifact(path.basename(file))) throw new Error('Expected archived HTML, research events or evidence JSON');
  const lock = await fs.open(file + '.archive-lock', 'wx', 0o600).catch(e => {
    if (e.code === 'EEXIST' || e.code === 'ENOENT') return null;
    throw e;
  });
  if (!lock) return null;
  const temp = path.join(path.dirname(file), '.' + randomUUID() + '.zst');
  try {
    const before = await fs.lstat(file).catch(e => { if (e.code === 'ENOENT') return null; throw e; });
    if (!before?.isFile()) return null;
    if (before.size > 64 * 1024 * 1024) throw new Error('Text artifact exceeds archive memory bound');
    const input = await fs.readFile(file);
    const compressed = await promisify(zstdCompress)(input, { params: {
      [constants.ZSTD_c_compressionLevel]: 19,
      [constants.ZSTD_c_checksumFlag]: 1,
    } });
    await fs.writeFile(temp, compressed, { mode: 0o600 });
    const restored = await promisify(zstdDecompress)(await fs.readFile(temp), { maxOutputLength: Math.max(1, input.length) });
    if (!restored.equals(input)) throw new Error('Text archive round trip differs');
    await fs.chmod(temp, before.mode & 0o777);
    await fs.utimes(temp, before.atime, before.mtime);
    const after = await fs.lstat(file);
    if (after.size !== before.size || after.mtimeMs !== before.mtimeMs) throw new Error('Capture changed during compression');
    if (controller.signal.aborted) throw new Error('Archive interrupted');
    await fs.rename(temp, file + '.zst');
    await fs.unlink(file);
    return { before: input.length, after: compressed.length };
  } finally {
    await fs.rm(temp, { force: true });
    await lock.close();
    await fs.rm(file + '.archive-lock', { force: true });
  }
}

export async function readCaptureHtml(file) {
  try { return await fs.readFile(file); }
  catch (e) {
    if (e.code !== 'ENOENT') throw e;
    return promisify(zstdDecompress)(await fs.readFile(file + '.zst'), { maxOutputLength: 64 * 1024 * 1024 });
  }
}

async function files(root) {
  const out = [];
  for (const e of await fs.readdir(root, { withFileTypes: true }).catch(e => { if (e.code === 'ENOENT') return []; throw e; })) {
    const file = path.join(root, e.name);
    if (e.isDirectory() && !e.name.startsWith('.') && !process.argv.includes('--logs-only')) out.push(...await files(file));
    else if (e.isFile() && (textArtifact(e.name) || (!process.argv.includes('--html-only') && !process.argv.includes('--logs-only') && e.name.endsWith('.png'))) && (!process.argv.includes('--html-only') || e.name === 'page.html') && (!process.argv.includes('--logs-only') || e.name !== 'page.html')) out.push(file);
  }
  return out;
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  process.on('SIGTERM', () => controller.abort());
  process.on('SIGINT', () => controller.abort());
  const listIndex = process.argv.indexOf('--files-from');
  const pending = listIndex >= 0
    ? JSON.parse(await fs.readFile(process.argv[listIndex + 1], 'utf8'))
    : await files(path.resolve(process.argv[2] || 'backend/data/jobs/sources'));
  let index = 0, converted = 0, failed = 0, before = 0, after = 0;
  console.log(JSON.stringify({ pending: pending.length }));
  const concurrency = Math.min(8, Math.max(1, Number(process.env.ARCHIVE_CONCURRENCY) || 4));
  await Promise.all(Array.from({ length: concurrency }, async () => {
    while (index < pending.length && !controller.signal.aborted) {
      const file = pending[index++];
      try {
        const result = await (textArtifact(path.basename(file)) ? archiveText(file) : archiveScreenshot(file));
        if (result) { converted++; before += result.before; after += result.after; }
      } catch (e) { failed++; console.error(JSON.stringify({ file, error: e.message })); }
      if ((converted + failed) % 100 === 0) console.log(JSON.stringify({ converted, failed, before, after }));
    }
  }));
  console.log(JSON.stringify({ converted, failed, before, after, saved: before - after }));
  if (failed || controller.signal.aborted) process.exitCode = 1;
}
