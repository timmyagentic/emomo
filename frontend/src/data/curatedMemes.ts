import type { DisplayMeme, ImageFormatSlug } from '../types';
import { curatedMemesData } from './curatedMemesData';

function normalizeCuratedFormat(value: string | undefined): ImageFormatSlug | undefined {
  if (!value) return undefined;
  const lower = value.toLowerCase();
  if (lower === 'jpg' || lower === 'jpeg') return 'jpg';
  if (lower === 'png') return 'png';
  if (lower === 'webp') return 'webp';
  return undefined;
}

export const curatedMemes: DisplayMeme[] = curatedMemesData.map((item) => ({
  id: item.id,
  url: item.url,
  width: item.width,
  height: item.height,
  format: normalizeCuratedFormat(item.format),
  description: item.description ?? '',
  tags: item.tags ?? [],
  category: item.category,
}));
