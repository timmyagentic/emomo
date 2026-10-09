import { readFile, stat } from 'node:fs/promises';
import { resolve } from 'node:path';
import { importRows } from './local.js';
import { REFINED_VOCABULARY } from './refined-vocabulary.js';
import { EmomoError } from './error.js';

const subjectTerms = new Set([...Object.values(REFINED_VOCABULARY.subjects).flat(), '企鹅','熊猫','熊猫头','草莓熊','海绵宝宝','派大星','章鱼哥','男人','女人','女子','卡通女孩','卡通动物','白色卡通动物','鸡','鸭','牛','兔子','青蛙','佩佩蛙','蜡笔小新','大雄','哆啦A梦','汤姆','白猫','白色小狗','小人'].map(s=>s.normalize('NFKC').toLowerCase()));
const bad = () => new EmomoError('INVALID_REVIEWED_METADATA', 'Expected complete reviewed JSONL with valid canonical links and OCR evidence.');
export async function importReviewed(source, directory) {
  const path=resolve(source);
  if ((await stat(path)).size > 32*1024*1024) throw bad();
  let rows;
  try { rows=(await readFile(path,'utf8')).trim().split('\n').map(line=>JSON.parse(line)); } catch { throw bad(); }
  if (!rows.length || rows.length>100000) throw bad();
  const byId=new Map();
  for (const r of rows) {
    if (!r || typeof r.id!=='string' || !/^[\w-]{1,150}$/.test(r.id) || byId.has(r.id) || !['canonical','duplicate_alias','excluded'].includes(r.disposition) || !['verified_visible_text','normalized_repetition','partially_illegible'].includes(r.ocr_review) || typeof r.image_text!=='string' || typeof r.search_text!=='string' || !Array.isArray(r.tags) || !r.tags.length || r.tags.some(t=>typeof t!=='string' || !t.trim()) || r.frames!==1) throw bad();
    if (r.disposition==='excluded' ? r.decision!=='exclude' || r.searchable!==false : r.decision!=='keep' || r.searchable!==(r.disposition==='canonical')) throw bad();
    if (r.ocr_review==='partially_illegible' ? r.search_text!=='' : r.search_text!==r.image_text) throw bad();
    byId.set(r.id,r);
  }
  const kept=rows.filter(r=>r.disposition!=='excluded');
  for (const r of kept) {
    const primary=byId.get(r.canonical_id);
    if (!primary || primary.disposition!=='canonical' || primary.canonical_id!==primary.id || (r.disposition==='canonical')!==(r.id===r.canonical_id)) throw bad();
  }
  const input=kept.map(r=>({ id:r.id, file_path:r.path, sha256:r.sha256, width:r.width, height:r.height, frames:1, media_kind:'image', category:'usable',
    image_text:r.search_text, description:r.tags.join('；'), intent_tags:r.tags, subjects:r.tags.filter(t=>subjectTerms.has(t.normalize('NFKC').toLowerCase())), canonical_id:r.canonical_id, searchable:r.searchable, ocr_review:r.ocr_review,
    quality:r.ocr_review, public_release_clearance:r.publicReleaseClearance ?? 'UNVERIFIED' }));
  const result=await importRows(input,directory,REFINED_VOCABULARY);
  return { ...result, sourceRecords:rows.length, defaultSearchMemes:kept.filter(r=>r.searchable).length, duplicateAliases:kept.filter(r=>!r.searchable).length, excluded:rows.length-kept.length, uncertainOcr:kept.filter(r=>r.ocr_review==='partially_illegible').length };
}
