import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { archiveScreenshot, archiveHtml, archiveText, readCaptureHtml } from './archive-screenshots.mjs';
let available = false;
try { available = execFileSync('magick', ['-list', 'format'], { encoding: 'utf8' }).includes('AVIF'); } catch {}
test('archives a long capture as a private AVIF within the size limit', { skip: !available }, async () => {
 const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'capture-avif-'));
 try {
  const png = path.join(dir, 'page.png');
  execFileSync('magick', ['-size', '20x17000', 'xc:white', png]);
  await fs.chmod(png, 0o600);
  const result = await archiveScreenshot(png);
  assert(result.before > 0 && result.after > 0);
  await assert.rejects(fs.stat(png), { code: 'ENOENT' });
  const avif = path.join(dir, 'page.avif');
  const [width,height] = execFileSync('magick', ['identify','-format','%w %h',avif], { encoding:'utf8' }).split(' ').map(Number);
  assert.equal(height,16383); assert(width < 20);
  assert.equal((await fs.stat(avif)).mode & 0o777, 0o600);
  assert.deepEqual(await fs.readdir(dir), ['page.avif']);
 } finally { await fs.rm(dir, { recursive:true, force:true }); }
});
test('failed conversion retains the original and removes temporary output and lock', { skip: !available }, async () => {
 const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'capture-avif-'));
 try {
  const png = path.join(dir,'page.png');
  await fs.writeFile(png,'not a PNG');
  await assert.rejects(archiveScreenshot(png));
  assert.equal(await fs.readFile(png,'utf8'),'not a PNG');
  assert.deepEqual(await fs.readdir(dir),['page.png']);
 } finally { await fs.rm(dir,{recursive:true,force:true}); }
});

test('HTML archive round-trips exact UTF-8 bytes, permissions and timestamps', async () => {
 const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'capture-zstd-'));
 try {
  const file = path.join(dir, 'page.html');
  const input = Buffer.from('<!doctype html><html>大麻 — česky 🌿</html>\n'.repeat(1000));
  await fs.writeFile(file, input, { mode: 0o600 });
  const date = new Date('2026-01-01T00:00:00Z');
  await fs.utimes(file,date,date);
  assert.deepEqual(await readCaptureHtml(file),input);
  const result = await archiveHtml(file);
  assert(result.after < result.before / 5);
  assert.deepEqual(await readCaptureHtml(file),input);
  const info = await fs.stat(file+'.zst');
  assert.equal(info.mode & 0o777,0o600);
  assert.equal(info.mtimeMs,date.getTime());
  assert.deepEqual(await fs.readdir(dir),['page.html.zst']);
 } finally { await fs.rm(dir,{recursive:true,force:true}); }
});
test('an empty HTML capture can be archived without losing its exact contents', async () => {
 const dir = await fs.mkdtemp(path.join(os.tmpdir(),'capture-zstd-empty-'));
 try {
  const file = path.join(dir,'page.html'); await fs.writeFile(file,'');
  await archiveHtml(file); assert.equal((await readCaptureHtml(file)).length,0);
 } finally { await fs.rm(dir,{recursive:true,force:true}); }
});

test('research events and evidence JSON are archived losslessly',async()=>{
 const dir=await fs.mkdtemp(path.join(os.tmpdir(),'job-log-zstd-'));
 try {
  for(const name of ['research-events.jsonl','evidence-2-56620c6c0a5806a7.json']) {
   const file=path.join(dir,name),input=Buffer.from('{"capture":"大麻 — source text"}\n'.repeat(500));
   await fs.writeFile(file,input,{mode:0o600});await archiveText(file);
   assert.deepEqual(await readCaptureHtml(file),input);
   await assert.rejects(fs.stat(file),{code:'ENOENT'});
  }
 }finally{await fs.rm(dir,{recursive:true,force:true});}
});
