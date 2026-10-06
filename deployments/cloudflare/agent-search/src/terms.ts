const CJK = /([\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]+)/u;
const WORDS = /[\p{L}\p{N}]+/gu;

function terms(text: string, includeSingleCharacters: boolean): string[] {
  const result: string[] = [];
  const pieces = text.normalize('NFKC').toLowerCase().split(CJK);
  for (let i = 0; i < pieces.length; i++) {
    if (i % 2 === 0) {
      result.push(...(pieces[i].match(WORDS) ?? []).filter(word => [...word].length <= 64));
      continue;
    }
    const characters = [...pieces[i]];
    if (includeSingleCharacters || characters.length === 1) result.push(...characters);
    for (let j = 0; j < characters.length - 1; j++) result.push(characters[j] + characters[j + 1]);
  }
  return result;
}

// Index unigrams as well, so a one-character subject such as 猫 is findable.
export function indexTerms(text: string): string {
  return terms(text, true).join(' ');
}

// Users cannot supply FTS operators: all terms are quoted literal tokens.
export function matchExpression(query: string): string {
  const tokens = [...new Set(terms(query, false))];
  if (tokens.length === 0 || tokens.length > 64) throw new Error('Use 1–64 short keyword terms.');
  return tokens.map(token => `"${token}"`).join(' OR ');
}
