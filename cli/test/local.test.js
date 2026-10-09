import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createHash } from 'node:crypto';
import { mkdtemp, writeFile, readFile, rm, rename, symlink } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { main, run } from '../src/cli.js';
import { PNG } from './helpers.js';
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const ONE_GIF = Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAAAAAAALAAAAAABAAEAAAIBRAA7', 'base64');
const GIF = Buffer.concat([ONE_GIF.subarray(0,-1), ONE_GIF.subarray(19,-1), Buffer.from([0x3b])]);
async function setup(t) {
  const root = await mkdtemp(join(tmpdir(), 'emomo-local-')); t.after(() => rm(root,{recursive:true,force:true}));
  await writeFile(join(root,'one.png'),PNG); await writeFile(join(root,'one.gif'),GIF);
  const row=(id,description,category='usable')=>({id,description,category,image_text:id===1?'对对对':'',intent_tags:id===1?['赞同','讽刺同意']:[],subjects:id===1?['猫']:[],scenarios:['聊天'],query_aliases:[],content_flags:[],media_kind:'image',frames:1,width:1,height:1,file_path:join(root,'one.png'),sha256:digest(PNG),public_release_clearance:'UNVERIFIED'});
  const rows=[row(1,'猫讽刺同意'),{...row(2,'猜拳动画'),intent_tags:['猜拳'],media_kind:'animation',file_path:join(root,'one.gif'),sha256:digest(GIF),frames:2},row(3,'饺子食物贴图','object_sticker'),row(4,'私密合影','excluded')];
  const metadata=join(root,'metadata.json'),vocabulary=join(root,'vocabulary.json');await writeFile(metadata,JSON.stringify(rows));
  await writeFile(vocabulary,JSON.stringify({intents:{赞同:['同意'],讽刺同意:['阴阳怪气'],猜拳:['石头剪刀布'],饭食:['饺子']},subjects:{猫:['猫']},gaps:{礼貌拒绝:'图库缺口'}}));
  const catalog=join(root,'catalog'),env={EMOMO_CONFIG_DIR:join(root,'config')};
  await run(['catalog','import',metadata,'--dir',catalog,'--vocabulary',vocabulary],{env});return {root,catalog,env,metadata,vocabulary};
}
async function cli(args,env) { let output='';const exit=await main(args,{env,write:s=>{output+=s;}});assert.equal(output.trim().split('\n').length,1);return {exit,...JSON.parse(output)}; }

test('local synonyms, metadata, GIF download and no-overwrite never access the network',async t=>{
  const s=await setup(t),old=globalThis.fetch;globalThis.fetch=()=>{throw new Error('network forbidden');};t.after(()=>{globalThis.fetch=old;});
  const env={...s.env,EMOMO_CATALOG:s.catalog,EMOMO_API_TOKEN:'unused-caller-token'};
  assert.equal((await run(['stats'],{env})).data.totalMemes,'3');
  const a=await run(['search','阴阳怪气地同意'],{env});assert.equal(a.data.results[0].id,'local-1');assert.deepEqual(a.data.results[0].match.intents,['赞同','讽刺同意']);
  assert.equal((await run(['search','石头剪刀布','--media','animation'],{env})).data.results[0].id,'local-2');
  const detail=await run(['get','local-2'],{env});assert.equal(detail.data.meme.image.format,'gif');
  const args=['download','local-2','--dir',join(s.root,'downloads')],download=await run(args,{env});assert.deepEqual(await readFile(download.data.path),GIF);assert.equal(download.data.mimeType,'image/gif');
  assert.equal((await cli(args,env)).error.code,'FILE_EXISTS');assert.equal((await run(['doctor'],{env})).data.checkedImages,3);
});
test('saved local catalog fails explicitly when disk is missing without remote fallback',async t=>{
  const s=await setup(t);await run(['catalog','use',s.catalog],{env:s.env});assert.equal((await run(['catalog','path'],{env:s.env})).data.path,s.catalog);await rename(s.catalog,s.catalog+'-offline');
  const old=globalThis.fetch;globalThis.fetch=()=>{throw new Error('no fallback');};t.after(()=>{globalThis.fetch=old;});const a=await cli(['search','同意'],s.env);assert.equal(a.exit,1);assert.equal(a.error.code,'LOCAL_CATALOG_UNAVAILABLE');
});
test('objects, media, subjects, text evidence and catalog gaps have explicit boundaries',async t=>{
  const s=await setup(t),env={...s.env,EMOMO_CATALOG:s.catalog};
  assert.equal((await run(['search','饺子'],{env})).data.total,0);assert.equal((await run(['search','饺子','--include-objects'],{env})).data.results[0].id,'local-3');
  assert.equal((await run(['search','猫同意','--subject','猫','--text','with'],{env})).data.results[0].id,'local-1');
  assert.equal((await run(['search','石头剪刀布','--media','preview'],{env})).data.total,0);assert.equal((await run(['search','礼貌拒绝'],{env})).data.reason,'gallery_gap');
  assert.equal((await run(['search','同意','--text','without'],{env})).data.reason,'text_absence_unverified');
  assert.equal((await cli(['search','同意','--media','video'],env)).error.code,'INVALID_ARGUMENT');assert.equal((await cli(['search','同意','--profile','vector'],env)).error.code,'INVALID_ARGUMENT');
});
test('import preserves existing catalogs and rejects changed source or corrupted index',async t=>{
  const s=await setup(t),before=await readFile(join(s.catalog,'manifest.json'));
  assert.equal((await cli(['catalog','import',s.metadata,'--dir',s.catalog],s.env)).error.code,'CATALOG_EXISTS');assert.deepEqual(await readFile(join(s.catalog,'manifest.json')),before);
  await writeFile(join(s.root,'one.png'),'corrupted');assert.equal((await cli(['catalog','import',s.metadata,'--dir',join(s.root,'new')],s.env)).error.code,'IMAGE_INTEGRITY_ERROR');
  await writeFile(join(s.catalog,'search.sqlite3'),'damaged');assert.equal((await cli(['stats','--catalog',s.catalog],s.env)).error.code,'INVALID_CATALOG');
});
test('local image symlink escapes and changed bytes are rejected',async t=>{
  const s=await setup(t),env={...s.env,EMOMO_CATALOG:s.catalog};const a=await run(['get','local-1'],{env}),asset=a.data.meme.localPath;
  await rm(asset);await symlink(join(s.root,'one.png'),asset);assert.equal((await cli(['get','local-1'],env)).error.code,'INVALID_CATALOG');
  await rm(asset);await writeFile(asset,'corrupt');assert.equal((await cli(['download','local-1','--dir',join(s.root,'download')],env)).error.code,'IMAGE_INTEGRITY_ERROR');
});
test('search SQL stays a read-only parameter and explicit local/API options cannot conflict',async t=>{
  const s=await setup(t),env={...s.env,EMOMO_CATALOG:s.catalog},before=await readFile(join(s.catalog,'search.sqlite3'));
  await run(['search',"' OR 1=1; DROP TABLE items; --"],{env});assert.deepEqual(await readFile(join(s.catalog,'search.sqlite3')),before);
  assert.equal((await cli(['search','猫','--catalog',s.catalog,'--api-url','https://example.com'],env)).error.code,'INVALID_ARGUMENT');
});
