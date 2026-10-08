import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createHash } from 'node:crypto';
import { mkdtemp, writeFile, readFile, rm, symlink, access, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { importCatalog } from '../src/local.js';
import { prepareSnapshot, validateSnapshot, diffSnapshots } from '../src/library.js';
import { PNG } from './helpers.js';
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const GIF = Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAAAAAAALAAAAAABAAEAAAIBRAA7','base64');
async function setup(t) {
 const root = await mkdtemp(join(tmpdir(),'emomo-library-'));t.after(()=>rm(root,{recursive:true,force:true}));
 await writeFile(join(root,'image.png'),PNG);await writeFile(join(root,'image.gif'),GIF);
 const row = (id) => ({id,description:'猫咪表情',category:'usable',image_text:'',width:1,height:1,frames:1,media_kind:'image',file_path:join(root,'image.png'),sha256:hash(PNG)});
 const rows=[row(1), {...row(2),file_path:join(root,'image.gif'),sha256:hash(GIF)}];
 const catalog=async(name,items=rows)=>{const p=join(root,name+'.json');await writeFile(p,JSON.stringify(items));const dir=join(root,name);await importCatalog(p,dir);return dir;};
 const prepare=(catalog,dir,revision='v1')=>prepareSnapshot(catalog,join(root,dir),{collection:'test',revision});
 return {root,rows,row,catalog,prepare};
}
test('private snapshots retain complete bytes, scoped IDs, deduplicated assets and make no network calls',async t=>{
 const s=await setup(t),old=globalThis.fetch;globalThis.fetch=()=>{throw Error('no network');};t.after(()=>{globalThis.fetch=old;});
 const source=await s.catalog('source',[...s.rows,s.row(3)]),a=await s.prepare(source,'v1'),b=await s.prepare(source,'v2','v2');
 assert.equal(a.records,3);assert.equal(a.uniqueAssets,2);assert.equal(a.publicReleaseUnverified.length,3);assert.deepEqual(a.remoteCompatibilityBlocked,['local-2']);assert.equal(a.publicReleaseReady,false);
 assert.equal(a.contentRevision,b.contentRevision);const snap=await validateSnapshot(a.path);
 assert.deepEqual(await readFile(join(a.path,snap.rows[1].asset)),GIF);assert.equal((await diffSnapshots(a.path,b.path)).unchanged,true);
 await assert.rejects(()=>s.prepare(source,'v1'),{code:'SNAPSHOT_EXISTS'});assert.equal((await validateSnapshot(a.path)).report.records,3);
});
test('diff distinguishes metadata edits, image replacement, reused additions, removals and vocabulary changes',async t=>{
 const s=await setup(t),base=await s.prepare(await s.catalog('base'),'base-snap');
 const rows=[{...s.row(1),description:'修改文案'},s.row(3)];
 const newer=await s.prepare(await s.catalog('next',rows),'next-snap','v2');
 const delta=await diffSnapshots(base.path,newer.path);assert.deepEqual(delta.added,[{id:'local-3',assetReused:true}]);assert.deepEqual(delta.removed,['local-2']);assert.deepEqual(delta.changed,[{id:'local-1',fields:['description'],imageChanged:false}]);assert.equal(delta.newAssetBytes,0);assert.equal(delta.removalsArePlanOnly,true);
 const changed=[{...s.rows[1],id:1}];const replacement=await s.prepare(await s.catalog('replaced',changed),'replacement');
 assert.equal((await diffSnapshots(base.path,replacement.path)).changed[0].imageChanged,true);
 const voc=join(s.root,'words.json');await writeFile(voc,JSON.stringify({intents:{微笑:['笑']}}));
 const meta=join(s.root,'vocmeta.json');await writeFile(meta,JSON.stringify(s.rows));await importCatalog(meta,join(s.root,'vocab-cat'),voc);
 const withVoc=await s.prepare(join(s.root,'vocab-cat'),'vocab-snap');assert.equal((await diffSnapshots(base.path,withVoc.path)).lexiconChanged,true);
});
test('tampered records, assets, escaping links and cross-collection diffs fail closed',async t=>{
 const s=await setup(t),cat=await s.catalog('cat'),a=await s.prepare(cat,'a'),b=await s.prepare(cat,'b');
 const snap=await validateSnapshot(a.path);const asset=join(a.path,snap.rows[0].asset);
 await rm(asset);await symlink(join(s.root,'image.png'),asset);await assert.rejects(()=>validateSnapshot(a.path),{code:'INVALID_SNAPSHOT'});
 await rm(asset);await writeFile(asset,'corrupt');await assert.rejects(()=>validateSnapshot(a.path),{code:'INVALID_SNAPSHOT'});
 await writeFile(join(b.path,'records.json'),'[]');await assert.rejects(()=>validateSnapshot(b.path),{code:'INVALID_SNAPSHOT'});
 const c=await prepareSnapshot(cat,join(s.root,'different'),{collection:'different',revision:'v1'}),d=await s.prepare(cat,'d');
 await assert.rejects(()=>diffSnapshots(c.path,d.path),{code:'COLLECTION_MISMATCH'});
});
test('changed source and invalid revisions never publish partial snapshots or overwrite existing directories',async t=>{
 const s=await setup(t),cat=await s.catalog('cat');
 await assert.rejects(()=>s.prepare(cat,'escape','../bad'),{code:'INVALID_ARGUMENT'});
 await mkdir(join(s.root,'empty'));await assert.rejects(()=>s.prepare(cat,'empty'),{code:'SNAPSHOT_EXISTS'});
 const snapshot=await s.prepare(cat,'good');const r=(await validateSnapshot(snapshot.path)).rows[0];await writeFile(join(cat,r.asset),'corrupt');
 await assert.rejects(()=>s.prepare(cat,'bad'),{code:'INVALID_SNAPSHOT'});await assert.rejects(()=>access(join(s.root,'bad')),{code:'ENOENT'});
});
test('concurrent prepares reserve one destination and leave a fully verified winner',async t=>{
 const s=await setup(t),cat=await s.catalog('race');
 const results=await Promise.allSettled([s.prepare(cat,'same'),s.prepare(cat,'same')]);
 assert.equal(results.filter(r=>r.status==='fulfilled').length,1);
 assert.equal(results.find(r=>r.status==='rejected').reason.code,'SNAPSHOT_EXISTS');
 assert.equal((await validateSnapshot(join(s.root,'same'))).report.records,2);
});
