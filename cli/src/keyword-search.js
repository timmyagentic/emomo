// One deterministic keyword plan for local SQLite and hosted D1.
// Aliases are curated data. No inference, corpus-dependent rules or network calls.
import { REFINED_VOCABULARY } from './refined-vocabulary.js';

export const normalize = text => text.normalize('NFKC').toLowerCase();
function terms(text, singles) {
  const result = [];
  const pieces = normalize(text).split(/([\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]+)/u);
  for (let i = 0; i < pieces.length; i++) {
    if (i % 2 === 0) result.push(...(pieces[i].match(/[\p{L}\p{N}]+/gu) ?? []).filter(word => [...word].length <= 64));
    else {
      const chars = [...pieces[i]];
      if (singles || chars.length === 1) result.push(...chars);
      for (let j = 0; j < chars.length - 1; j++) result.push(chars[j] + chars[j + 1]);
    }
  }
  return result;
}
export const indexTerms = text => terms(text, true).join(' ');
export const tokens = text => [...new Set(terms(text, false))];

export function keywordPlan(query, vocabulary = {}) {
  const raw = tokens(query);
  if (!raw.length || raw.length > 64) throw new Error('Use 1–64 short keyword terms.');
  const subjects = { ...REFINED_VOCABULARY.subjects, ...vocabulary.subjects };
  const intents = { ...REFINED_VOCABULARY.intents, ...vocabulary.intents };
  const aliases = { ...intents, ...subjects }, reverse = new Map();
  // Explicit keys win over aliases shared by multiple entries.
  for (const [key, values] of Object.entries(aliases)) for (const value of values) if (!reverse.has(normalize(value))) reverse.set(normalize(value), key);
  for (const key of Object.keys(aliases)) reverse.set(normalize(key), key);
  const words = [...reverse.keys()].filter(Boolean).sort((a, b) => b.length - a.length || a.localeCompare(b));
  const groups = [];
  const add = word => {
    if (!word || /^[的了我你他她很地]+$/.test(word) && normalize(query).trim() !== word) return;
    const key = reverse.get(word);
    const alternatives = [...new Set([word, ...(key ? [key, ...aliases[key]] : [])].map(normalize))];
    const expressions = alternatives.map(value => tokens(value).map(token => `"${token}"`).join(' AND ')).filter(Boolean);
    if (expressions.length) groups.push({ key: key ?? word, terms: alternatives, subject: !!key && Object.hasOwn(subjects, key), intent: !!key && Object.hasOwn(intents, key), literal: '(' + tokens(word).map(token => `"${token}"`).join(' AND ') + ')', expression: '(' + expressions.map(expr => '(' + expr + ')').join(' OR ') + ')' });
  };
  const text = normalize(query).replace(/^(?:(?:请|帮我|给我|我想|想要|找一张|找个|找|来一张|来个))+/, '').replace(/(?:的表情包|的表情|表情包|图片|表情)$/, '').trim() || normalize(query);
  for (const chunk of text.match(/[\p{L}\p{N}]+/gu) ?? []) {
    let pos = 0, unknown = '';
    while (pos < chunk.length) {
      const word = words.find(value => chunk.startsWith(value, pos) && (!/^[a-z0-9]+$/.test(value) || value === chunk) && (!unknown || !Object.hasOwn(subjects, reverse.get(value))));
      if (word) { add(unknown); unknown = ''; add(word); pos += word.length; }
      else { unknown += chunk[pos]; pos++; }
    }
    add(unknown);
  }
  const expression = groups.map(group => group.expression).join(' AND ');
  if (!expression || expression.length > 16384) throw new Error('Use fewer short keyword terms.');
  return { expression, literalExpression: groups.map(group => group.literal).join(' AND '), groups };
}
export const matchExpression = query => keywordPlan(query).expression;

// Restrict explicit subjects to reviewed tags when present; untagged legacy
// records retain text search. Both adapters use the same parameterized clause.
export function subjectFilter(plan, tagsSql) {
  const clauses = [], values = [];
  for (const group of plan.groups.filter(group => group.subject)) {
    clauses.push(`(json_array_length(${tagsSql}) = 0 OR EXISTS (SELECT 1 FROM json_each(${tagsSql}) WHERE lower(value) IN (${group.terms.map(() => '?').join(',')})))`);
    values.push(...group.terms);
  }
  return { sql: clauses.length ? ' AND ' + clauses.join(' AND ') : '', values };
}
