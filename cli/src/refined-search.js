// Faceted keyword search for visually reviewed catalogs. The calling Agent owns intent reasoning.
import { normalize, tokens } from './local.js';

function groupsFor(query, tags, lexicon) {
  const aliases = { ...lexicon.intents, ...lexicon.subjects }, reverse = new Map();
  for (const [key, values] of Object.entries(aliases)) for (const value of values) if (!reverse.has(normalize(value))) reverse.set(normalize(value), key);
  for (const key of Object.keys(aliases)) reverse.set(normalize(key), key);
  const vocab = [...new Set([...tags].filter(t => t.length > 1).concat([...reverse.keys()]))].sort((a,b) => b.length-a.length || a.localeCompare(b));
  const q = normalize(query).replace(/^(?:(?:请|帮我|给我|我想|想要|找一张|找个|找|来一张|来个))+/, '').replace(/(?:的表情包|的表情|表情包|图片|表情)$/, '').trim() || normalize(query).trim();
  const parts = [], push = s => { if (s && !/^(?:[的了我你他她很]+|一个|一只|特别|正在)$/.test(s)) parts.push(s); };
  for (const chunk of q.match(/[\u3400-\u9fffa-z0-9]+/g) ?? []) {
    if (reverse.has(chunk) || tags.has(chunk)) { parts.push(chunk); continue; }
    let pos=0, unknown='';
    while (pos < chunk.length) {
      const word = vocab.find(t => chunk.startsWith(t,pos)) ?? ('猫狗牛熊'.includes(chunk[pos]) ? chunk[pos] : undefined);
      if (word) { push(unknown); unknown=''; parts.push(word); pos += word.length; }
      else { unknown += chunk[pos]; pos++; }
    }
    push(unknown);
  }
  return parts.map(part => {
    const key = reverse.get(part) ?? part;
    return { terms: [...new Set([part, ...(aliases[key] ?? [])].map(normalize))], subject: Object.hasOwn(lexicon.subjects,key) };
  });
}

export function refinedSearch(catalog, query, { limit, category, textPresence, media, subject, intent, includeObjects }) {
  const empty = reason => ({ query, expandedQuery: '', total: 0, results: [], reason, source: 'local' });
  if (textPresence === 3) return empty('text_absence_unverified');
  const gap = Object.entries(catalog.lexicon.gaps).find(([term]) => normalize(query).includes(normalize(term)));
  if (gap) return { ...empty('gallery_gap'), detail: gap[1] };
  // Cache only for this read-only invocation. Every candidate still comes through the FTS index.
  const rows = catalog.db.prepare('SELECT data FROM items').all().map(row => JSON.parse(row.data));
  const tags = new Set(rows.filter(r => r.searchable !== false).flatMap(r => r.tags.map(normalize)));
  const groups = groupsFor(query, tags, catalog.lexicon);
  if (!groups.length) return empty('no_match');
  const ts = [...new Set(groups.flatMap(g => g.terms.flatMap(tokens)))];
  if (!ts.length) return empty('no_match');
  const clauses = ['search MATCH ?'], params = [ts.map(t => `"${t}"`).join(' OR ')];
  if (category) { clauses.push('items.category = ?'); params.push(category); }
  else if (!includeObjects) clauses.push("items.category = 'usable'");
  if (media) { clauses.push('items.media = ?'); params.push(media); }
  if (textPresence === 2) clauses.push('items.text_present = 1');
  if (textPresence === 1) clauses.push('items.text_present = 0');
  const candidates = catalog.db.prepare(`SELECT items.data FROM search JOIN items ON items.id=search.rowid WHERE ${clauses.join(' AND ')}`).all(...params);
  const ranked = [], q = normalize(query.trim());
  for (const row of candidates) {
    const r = JSON.parse(row.data), rt = r.tags.map(normalize), txt = normalize(r.imageText), matched = [];
    if (r.searchable === false || subject && !r.subjects.includes(subject) || intent && !r.tags.includes(intent)) continue;
    let score = 0;
    for (const group of groups) {
      const hits = group.terms.filter(t => rt.includes(t) || (!group.subject && (txt.includes(t) || t.length > 1 && rt.some(tag => tag.includes(t)))));
      if (!hits.length) break;
      hits.sort((a,b) => Number(rt.includes(b))-Number(rt.includes(a)) || b.length-a.length);
      const best=hits[0]; matched.push(best); score += 12*Number(rt.includes(best)) + 8*Number(txt.includes(best)) + Math.min(best.length,10);
    }
    if (matched.length !== groups.length) continue;
    if (txt.includes(q)) score += 30;
    if (['不','没','未'].some(prefix => txt.includes(prefix+q))) score -= 60;
    ranked.push({ ...catalog.candidate(r,score), match: { terms: matched, subjects: groups.filter(g=>g.subject).flatMap(g=>g.terms.filter(t=>rt.includes(t))), literalFields: txt.includes(q) ? ['imageText'] : [] } });
  }
  ranked.sort((a,b)=>b.score-a.score || b.image.width*b.image.height-a.image.width*a.image.height || a.id.localeCompare(b.id));
  return { query, expandedQuery: groups.map(g=>g.terms.join('|')).join(' '), total: ranked.length, results: ranked.slice(0,limit), reason: ranked.length ? 'matched' : 'no_match', source: 'local' };
}
