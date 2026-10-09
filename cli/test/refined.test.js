import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createHash } from 'node:crypto';
import { mkdtemp,writeFile,readFile,rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { run } from '../src/cli.js';
import { prepareSnapshot,validateSnapshot } from '../src/library.js';
import { PNG } from './helpers.js';

async function fixture(t) {
  const root=await mkdtemp(join(tmpdir(),'emomo-refined-'));t.after(()=>rm(root,{recursive:true,force:true}));
  const path=join(root,'original.png');await writeFile(path,PNG);
  const row=(id,tags,text='',extra={})=>({id,canonical_id:id,path,sha256:createHash('sha256').update(PNG).digest('hex'),width:1,height:1,frames:1,tags,image_text:text,search_text:text,ocr_review:'verified_visible_text',decision:'keep',disposition:'canonical',searchable:true,...extra});
  const rows=[row('cat',['猫','生气'],'火大'),row('panda',['熊猫','生气'],'别惹我'),row('happy',['猫','开心'],'开心'),row('alias',['猫','生气'],'火大',{canonical_id:'cat',disposition:'duplicate_alias',searchable:false}),row('unclear',['疑惑'],'秘密火星文字',{search_text:'',ocr_review:'partially_illegible'}),row('excluded',['广告'],'广告',{decision:'exclude',disposition:'excluded',searchable:false}),row('refusal',['礼貌','拒绝'],'谢谢，不用了')];
  const input=join(root,'reviewed.jsonl');await writeFile(input,rows.map(r=>JSON.stringify(r)).join('\n'));
  return {root,rows,input,catalog:join(root,'catalog')};
}

test('reviewed import preserves primary/alias retrieval, excludes rejected/uncertain text, and requires every facet',async t=>{
  const f=await fixture(t),fetch=globalThis.fetch;globalThis.fetch=()=>{throw Error('network forbidden');};t.after(()=>{globalThis.fetch=fetch;});
  const result=await run(['catalog','import-reviewed',f.input,'--dir',f.catalog]);assert.equal(result.data.defaultSearchMemes,5);assert.equal(result.data.duplicateAliases,1);assert.equal(result.data.excluded,1);
  const env={EMOMO_CATALOG:f.catalog};
  assert.equal((await run(['stats'],{env})).data.totalMemes,'6');assert.equal((await run(['stats'],{env})).data.defaultSearchMemes,'5');
  const search=async q=>(await run(['search',q],{env})).data;
  assert.deepEqual((await search('猫 生气')).results.map(r=>r.id),['local-cat']);
  assert.equal((await search('猫 生日快乐')).total,0);assert.equal((await search('火星文字')).total,0);assert.equal((await search('广告')).total,0);
  assert.equal((await search('礼貌拒绝')).results[0].id,'local-refusal');
  assert.equal((await search('疑惑')).results[0].ocrReview,'partially_illegible');
  const get=(await run(['get','local-alias'],{env})).data.meme;assert.deepEqual(get.subjects,['猫']);assert.equal(get.canonicalId,'local-cat');assert.equal(get.versions.length,2);assert.deepEqual(await readFile(get.localPath),PNG);
  const download=(await run(['download','local-alias','--dir',join(f.root,'download')],{env})).data;assert.deepEqual(await readFile(download.path),PNG);
  await assert.rejects(run(['get','local-excluded'],{env}),{code:'NOT_FOUND'});
  assert.equal((await run(['search','疑惑','--text','with'],{env})).data.total,0);
  assert.equal((await run(['search','猫','--media','animation'],{env})).data.total,0);
  await prepareSnapshot(f.catalog,join(f.root,'snapshot'),{collection:'refined',revision:'v1'});
  const snapshot=await validateSnapshot(join(f.root,'snapshot'));assert.equal(snapshot.rows.find(r=>r.id==='local-alias').canonicalId,'local-cat');assert.equal(snapshot.vocabulary.searchMode,'facets');
});

test('reviewed importer rejects broken groups, duplicate IDs and leaked uncertain OCR before catalog writes',async t=>{
  const f=await fixture(t);
  for (const mutation of [rows=>rows[3].canonical_id='missing',rows=>rows[3].canonical_id='alias',rows=>rows[1].id='cat',rows=>rows[4].search_text=rows[4].image_text,rows=>rows[5].searchable=true]) {
    const rows=structuredClone(f.rows);mutation(rows);await writeFile(f.input,rows.map(r=>JSON.stringify(r)).join('\n'));
    await assert.rejects(run(['catalog','import-reviewed',f.input,'--dir',f.catalog]),{code:'INVALID_REVIEWED_METADATA'});
  }
});
