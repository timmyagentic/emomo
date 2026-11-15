// Shared validation for local preparation and the maintenance API.
export const MAX_IMPORT_IMAGE_BYTES = 25 * 1024 * 1024;
export const SHA256 = /^[a-f0-9]{64}$/;
const invalid = () => new Error('Provide a SHA-256, static image dimensions, category, description, text status, OCR and factual tags.');
export function importMetadata(value) {
  if (!value || !SHA256.test(value.sha256) || !['usable', 'object_sticker'].includes(value.category)) throw invalid();
  const info = value.image_info;
  if (!info || ![1, 2, 3].includes(info.format) || ![info.width, info.height].every(n => Number.isSafeInteger(n) && n > 0 && n <= 100000)) throw invalid();
  if (typeof value.description !== 'string' || !value.description.trim() || value.description.length > 8192) throw invalid();
  if (typeof value.ocr_text !== 'string' || value.ocr_text.length > 4096 || ![1, 2, 3].includes(value.text_presence)) throw invalid();
  if (value.text_presence !== 2 && value.ocr_text.trim()) throw invalid();
  const tags = value.tags, aliases = value.search_aliases ?? [];
  for (const words of [tags, aliases]) {
    if (!Array.isArray(words) || words.length > 64 || words.some(s => typeof s !== 'string' || !s.trim() || s.length > 128 || s.includes('\0'))) throw invalid();
  }
  if (!tags.length || value.description.includes('\0') || value.ocr_text.includes('\0')) throw invalid();
  if (value.preview_only !== undefined && typeof value.preview_only !== 'boolean') throw invalid();
  const normalized = { sha256: value.sha256, image_info: { width: info.width, height: info.height, format: info.format },
    category: value.category, description: value.description.trim(), ocr_text: value.ocr_text.trim(),
    text_presence: value.text_presence, tags: [...new Set(tags.map(s => s.trim()))],
    search_aliases: [...new Set(aliases.map(s => s.trim()))], preview_only: value.preview_only ?? false };
  if (normalized.preview_only && normalized.description.length > 8180) throw invalid();
  if (new TextEncoder().encode(JSON.stringify(normalized)).length > 32768) throw invalid();
  return normalized;
}
