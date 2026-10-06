import { create, fromJson, toJsonString, type JsonValue } from '@bufbuild/protobuf';
import { MemeSchema, MemeAnnotationSchema } from '../gen/emomo/v1/meme_pb.js';
import { indexTerms } from './terms.js';

export const ID = /^[a-zA-Z0-9_-]{1,120}$/;

function bounded(value: string, max: number): string {
  if ([...value].length > max || value.includes('\0')) throw new Error('Metadata field exceeds limits.');
  return value;
}

export function validateStorageKey(key: string): string {
  bounded(key, 1024);
  if (!key || key.includes('\\') || key.split('/').some(part => !part || part === '.' || part === '..')) {
    throw new Error('Invalid image storage key.');
  }
  return key;
}

export type IndexRow = {
  id: string;
  meme_json: string;
  category: string;
  description: string;
  ocr_text: string;
  text_presence: number;
  ocr_terms: string;
  description_terms: string;
  tag_terms: string;
};

function normalizedOCR(text: string): string {
  const value = text.trim().replace(/^[\s"'`.，。;；:：!！?？]+|[\s"'`.，。;；:：!！?？]+$/gu, '').replace(/\s+/gu, ' ');
  return ['none', 'no text', 'no_text', 'n/a', 'null', '无文字', '没有文字', '无内容', '无文本', '无字', '无文字内容'].includes(value.toLowerCase()) ? '' : value;
}

// Offline input: one selected existing annotation per canonical meme, no models.
export function indexRow(input: JsonValue): IndexRow {
  if (!input || typeof input !== 'object' || Array.isArray(input) || !input.meme ||
      Object.keys(input).some(key => key !== 'meme' && key !== 'annotation')) throw new Error('Expected meme and optional annotation.');
  const meme = fromJson(MemeSchema, input.meme);
  if (!ID.test(meme.id)) throw new Error('Invalid meme id.');
  validateStorageKey(meme.storageKey);
  if (meme.url) throw new Error('Export storage_key, not a public or signed URL.');
  bounded(meme.contentHash, 256);
  bounded(meme.category, 80);
  if (meme.tags.length > 64) throw new Error('Too many metadata tags.');
  for (const tag of meme.tags) bounded(tag, 128);
  if (meme.imageInfo && (meme.imageInfo.width < 0 || meme.imageInfo.height < 0 || ![0, 1, 2, 3].includes(meme.imageInfo.format))) {
    throw new Error('Invalid image metadata.');
  }
  const annotation = input.annotation == null ? null : fromJson(MemeAnnotationSchema, input.annotation);
  if (annotation && annotation.memeId !== meme.id) throw new Error('Annotation belongs to another meme.');
  const description = bounded(annotation?.description ?? '', 8192);
  const ocr = normalizedOCR(bounded(annotation?.ocrText ?? '', 4096));
  const textPresence = !annotation ? 1 : /[\p{L}\p{N}]/u.test(ocr) ? 2 : 3;
  return {
    id: meme.id,
    meme_json: toJsonString(MemeSchema, create(MemeSchema, meme), { useProtoFieldName: true, enumAsInteger: true }),
    category: meme.category,
    description,
    ocr_text: ocr,
    text_presence: textPresence,
    ocr_terms: indexTerms(ocr),
    description_terms: indexTerms(description),
    tag_terms: indexTerms(meme.tags.join(' ')),
  };
}

const COLUMNS = 'id, meme_json, category, description, ocr_text, text_presence, ocr_terms, description_terms, tag_terms';
const UPDATE = COLUMNS.split(', ').filter(name => name !== 'id').map(name => `${name}=excluded.${name}`).join(', ');
export const UPSERT = `INSERT INTO memes (${COLUMNS}) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET ${UPDATE}`;
export function rowValues(row: IndexRow): (string | number)[] {
  return [row.id, row.meme_json, row.category, row.description, row.ocr_text, row.text_presence, row.ocr_terms, row.description_terms, row.tag_terms];
}
export function importSQL(row: IndexRow): string {
  const values = rowValues(row).map(value => typeof value === 'number' ? String(value) : `'${value.replaceAll("'", "''")}'`);
  return `INSERT INTO memes (${COLUMNS}) VALUES (${values.join(', ')}) ON CONFLICT(id) DO UPDATE SET ${UPDATE};`;
}
