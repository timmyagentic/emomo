export type Vocabulary = { subjects?: Record<string, string[]>; intents?: Record<string, string[]> };
export type KeywordPlan = { expression: string; groups: { key: string; terms: string[]; subject: boolean; intent: boolean; expression: string }[] };
export function normalize(text: string): string;
export function tokens(text: string): string[];
export function indexTerms(text: string): string;
export function keywordPlan(query: string, vocabulary?: Vocabulary): KeywordPlan;
export function matchExpression(query: string): string;
export function subjectFilter(plan: KeywordPlan, tagsSql: string): { sql: string; values: string[] };
