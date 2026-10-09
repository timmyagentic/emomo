import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir, mkdtemp, rename, rm } from 'node:fs/promises';
import { dirname, resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { validateSnapshot, type LibraryRecord } from '../../../../cli/src/library.js';
import { ID, indexRow, importSQL } from '../src/metadata.js';
import type { JsonValue } from '@bufbuild/protobuf';

const digest = (bytes: Uint8Array | string) => createHash('sha256').update(bytes).digest('hex');
const formats = { jpeg: 1, png: 2, webp: 3 };
export function staticRecord(collection: string, row: LibraryRecord): JsonValue {
  if (row.image.frames !== 1 || row.mediaKind === 'animation' || row.image.format === 'gif') throw new Error('GIF and animation are excluded from the first release.');
  const id = `${collection}-${row.id}`;
  if (!ID.test(id)) throw new Error('Collection-qualified image ID exceeds canonical limits.');
  const extension = row.image.format === 'jpeg' ? 'jpg' : row.image.format;
  const text = row.ocrReview === 'partially_illegible' ? '' : row.imageText.trim();
  return {
    meme: { id, storage_key: `collections/${collection}/${row.sha256}.${extension}`, content_hash: row.sha256,
      image_info: { width: row.image.width, height: row.image.height, format: formats[row.image.format] },
      tags: [...new Set([...row.tags, ...row.subjects])], category: row.category },
    annotation: { meme_id: id, description: row.description, ocr_text: text, ...(text ? { labels: { has_text: true } } : {}) },
    text_presence: text ? 2 : 1,
    // Reviewed aliases/scenarios are private index terms, never asserted as image facts.
    search_aliases: [...new Set([...row.aliases, ...row.scenarios])],
  };
}
export async function prepareStatic(source: string, output: string) {
  const snapshot = await validateSnapshot(source);
  const selected = snapshot.rows.filter(row => row.searchable !== false && row.image.frames === 1 && row.mediaKind !== 'animation' && row.image.format !== 'gif');
  if (!selected.length) throw new Error('No eligible static records.');
  const inputs = selected.map(row => staticRecord(snapshot.manifest.collection, row));
  const rows = inputs.map(indexRow);
  const lines = inputs.map(row => JSON.stringify(row));
  const statements = rows.map(importSQL);
  if (lines.some(s => Buffer.byteLength(s) > 128 * 1024) || statements.some(s => Buffer.byteLength(s) > 96 * 1024)) throw new Error('Metadata exceeds index importer limits.');
  const metadata = lines.join('\n')+'\n', sql = statements.join('\n')+'\n';
  const target = resolve(output); await mkdir(dirname(target), { recursive: true });
  const stage = await mkdtemp(join(dirname(target), '.emomo-static-'));
  let reserved = false;
  try {
    const objects: { id: string; key: string; sha256: string; bytes: number; relativeFile: string }[] = [];
    const written = new Set<string>();
    for (let i=0;i<selected.length;i++) {
      const sourceRow = selected[i];
      // Source was containment-checked; revalidate all sources after copying as well.
      const bytes = await readFile(join(snapshot.root, sourceRow.asset));
      if (bytes.length > 25 * 1024 * 1024 || digest(bytes) !== sourceRow.sha256) throw new Error('Source image changed during preparation.');
      const key = `collections/${snapshot.manifest.collection}/${sourceRow.sha256}.${sourceRow.image.format === 'jpeg' ? 'jpg' : sourceRow.image.format}`;
      const relativeFile = `images/${key}`;
      if (!written.has(key)) { await mkdir(dirname(join(stage, relativeFile)), { recursive: true }); await writeFile(join(stage,relativeFile),bytes,{flag:'wx',mode:0o600}); written.add(key); }
      objects.push({ id: rows[i].id, key, sha256: sourceRow.sha256, bytes: bytes.length, relativeFile });
    }
    const report = { schemaVersion: 1, kind: 'emomo-static-preparation', collection: snapshot.manifest.collection,
      sourceContentRevision: snapshot.manifest.contentRevision, records: rows.length,
      usable: selected.filter(r => r.category === 'usable').length, objects: selected.filter(r => r.category === 'object_sticker').length,
      excluded: snapshot.rows.filter(r => !selected.includes(r)).map(r => ({ id: r.id, reason: r.searchable === false ? 'DUPLICATE_ALIAS' : 'GIF_OR_ANIMATION_DEFERRED' })),
      textWith: rows.filter(r => r.text_presence === 2).length, textUnknown: rows.filter(r => r.text_presence === 1).length,
      uniqueAssets: written.size, totalBytes: [...new Map(objects.map(o=>[o.key,o.bytes])).values()].reduce((a,b)=>a+b,0),
      metadataSha256: digest(metadata), sqlSha256: digest(sql), publicReleaseReady: false, networkCalls: 0, modelsUsed: [] };
    await writeFile(join(stage,'metadata.jsonl'),metadata,{flag:'wx',mode:0o600});
    await writeFile(join(stage,'index.sql'),sql,{flag:'wx',mode:0o600});
    await writeFile(join(stage,'objects.json'),JSON.stringify(objects,null,2)+'\n',{flag:'wx',mode:0o600});
    await writeFile(join(stage,'preparation.json'),JSON.stringify(report,null,2)+'\n',{flag:'wx',mode:0o600});
    const finalSource = await validateSnapshot(source);
    if (finalSource.manifest.contentRevision !== snapshot.manifest.contentRevision) throw new Error('Source revision changed during preparation.');
    try { await mkdir(target); reserved = true; } catch { throw new Error('Destination already exists or is unavailable; choose a new directory.'); }
    await rename(stage,target); reserved = false;
    return { path: target, ...report };
  } finally {
    if (reserved) await rm(target,{recursive:true,force:true});
    await rm(stage,{recursive:true,force:true});
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [source,output,...extra] = process.argv.slice(2);
  if (!source || !output || extra.length) throw new Error('Usage: npm run static:prepare -- <snapshot> <new-output-directory>');
  console.log(JSON.stringify(await prepareStatic(source,output)));
}
