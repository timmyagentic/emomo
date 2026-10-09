// End-to-end receipt against a packed installed CLI and private reviewed fixture.
// node verify-reviewed.mjs <emomo-binary> <catalog> <metadata.jsonl> <cases.json> <new-output-dir>
import { execFileSync } from 'node:child_process';
import { readFileSync,writeFileSync,mkdirSync } from 'node:fs';
import { join,resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { DatabaseSync } from 'node:sqlite';
import assert from 'node:assert/strict';
const [binary,catalog,input,caseFile,out]=process.argv.slice(2);
assert.ok(binary&&catalog&&input&&caseFile&&out);
mkdirSync(out);
const hash=b=>createHash('sha256').update(b).digest('hex');
const invoke=args=>{
  const result=JSON.parse(execFileSync(resolve(binary),[...args,'--catalog',resolve(catalog)],{cwd:out,encoding:'utf8'}));
  assert.equal(result.ok,true);return result.data;
};
const source=readFileSync(input),rows=source.toString().trim().split('\n').map(JSON.parse),kept=rows.filter(r=>r.disposition!=='excluded');
const db=new DatabaseSync(join(catalog,'search.sqlite3'),{readOnly:true});
const actual=db.prepare('SELECT data FROM items').all().map(r=>JSON.parse(r.data));
assert.equal(actual.length,kept.length);
const byId=new Map(actual.map(r=>[r.id,r]));
for (const r of rows) {
  const item=byId.get('local-'+r.id);
  if (r.disposition==='excluded') {assert.equal(item,undefined);continue;}
  assert.equal(item.sha256,r.sha256);assert.equal(item.canonicalId,'local-'+r.canonical_id);assert.equal(item.searchable,r.searchable);assert.equal(item.imageText,r.search_text);assert.deepEqual(item.tags,r.tags);
  if (r.ocr_review==='partially_illegible') assert.equal(item.imageText,'');
}
assert.equal(db.prepare('SELECT count(*) AS n FROM search').get().n,rows.filter(r=>r.searchable).length);
db.close();
const cases=JSON.parse(readFileSync(caseFile)),verified=[];
for (const c of cases) {
  const search=invoke(['search',c.query,'--limit',String(c.limit??4)]);
  if (c.selectedId) {
    assert.ok(search.results.some(r=>r.id===c.selectedId),c.query);
    const detail=invoke(['get',c.selectedId]).meme,download=invoke(['download',c.selectedId,'--dir',join(out,'images')]);
    assert.equal(hash(readFileSync(download.path)),detail.sha256);
    assert.equal(detail.sha256,byId.get(c.selectedId).sha256);
    verified.push({...c,total:search.total,download,visualReview:'agent_inspected_complete_candidate',match:search.results.find(r=>r.id===c.selectedId).match});
  } else { assert.equal(search.total,0); verified.push({...c,total:0,reason:search.reason}); }
}
const alias=actual.find(r=>r.searchable===false),detail=invoke(['get',alias.id]).meme;
assert.equal(detail.canonicalId,alias.canonicalId);assert.ok(detail.versions.some(v=>v.id===alias.id));assert.ok(detail.versions.some(v=>v.id===alias.canonicalId));
const aliasFile=invoke(['download',alias.id,'--dir',join(out,'versions')]);assert.equal(hash(readFileSync(aliasFile.path)),alias.sha256);
let overwrite;
try { invoke(['download',alias.id,'--dir',join(out,'versions')]);assert.fail('must reject overwrite'); }
catch(e) {overwrite=JSON.parse(e.stdout?.toString()??'{}');assert.equal(overwrite.error?.code,'FILE_EXISTS');}
const receipt={ok:true,version:invoke(['capabilities']).version,catalog:resolve(catalog),sourceSha256:hash(source),sourceRecords:rows.length,storedOriginals:actual.length,defaultSearchMemes:rows.filter(r=>r.searchable).length,excluded:rows.length-kept.length,duplicateAliases:actual.filter(r=>!r.searchable).length,uncertainStored:actual.filter(r=>r.ocrReview==='partially_illegible').length,cases:verified,aliasOriginalHashVerified:true,noOverwriteVerified:true,createdAt:new Date().toISOString()};
writeFileSync(join(out,'receipt.json'),JSON.stringify(receipt,null,2)+'\n');
console.log(JSON.stringify({ok:true,sourceRecords:rows.length,cases:verified.length,selected:verified.filter(c=>c.selectedId).length,receipt:join(out,'receipt.json')}));
