import { ImageFormat, TextPresence } from '@gen/emomo/v1/types_pb';
import type { Meme as PbMeme, SearchResult as PbSearchResult } from '@gen/emomo/v1/meme_pb';

export type ImageFormatSlug = 'jpg' | 'png' | 'webp' | 'unknown';
export type TextPresenceFilter = 'all' | 'with_text' | 'without_text';

export interface DisplayMeme {
  id: string;
  url: string;
  score?: number;
  description: string;
  category?: string;
  tags: string[];
  width?: number;
  height?: number;
  format?: ImageFormatSlug;
  textPresence?: 'unknown' | 'with_text' | 'without_text';
}

export interface StatsView {
  totalMemes: number;
  totalCategories: number;
  availableCollections: string[];
  availableProfiles: string[];
}

export type SearchStageSlug =
  | 'query_expansion_start'
  | 'thinking'
  | 'query_expansion_done'
  | 'embedding'
  | 'searching'
  | 'enriching'
  | 'complete'
  | 'error';

export interface SearchProgressView {
  stage: SearchStageSlug;
  message?: string;
  thinkingText?: string;
  isDelta?: boolean;
  expandedQuery?: string;
  results?: DisplayMeme[];
  total?: number;
  query?: string;
  collection?: string;
  profile?: string;
  error?: string;
}

export function imageFormatToSlug(format?: ImageFormat): ImageFormatSlug | undefined {
  switch (format) {
    case ImageFormat.JPEG:
      return 'jpg';
    case ImageFormat.PNG:
      return 'png';
    case ImageFormat.WEBP:
      return 'webp';
    default:
      return undefined;
  }
}

export function textPresenceToSlug(presence?: TextPresence): DisplayMeme['textPresence'] {
  switch (presence) {
    case TextPresence.WITH_TEXT:
      return 'with_text';
    case TextPresence.WITHOUT_TEXT:
      return 'without_text';
    case TextPresence.UNKNOWN:
      return 'unknown';
    default:
      return undefined;
  }
}

export function pbMemeToDisplay(
  meme: PbMeme,
  extras?: { score?: number; description?: string; textPresence?: TextPresence }
): DisplayMeme {
  return {
    id: meme.id,
    url: meme.url,
    score: extras?.score,
    description: extras?.description ?? '',
    category: meme.category || undefined,
    tags: meme.tags ?? [],
    width: meme.imageInfo?.width || undefined,
    height: meme.imageInfo?.height || undefined,
    format: imageFormatToSlug(meme.imageInfo?.format),
    textPresence: textPresenceToSlug(extras?.textPresence),
  };
}

export function pbSearchResultToDisplay(result: PbSearchResult): DisplayMeme {
  if (!result.meme) {
    return {
      id: '',
      url: '',
      description: result.description ?? '',
      tags: [],
    };
  }

  return pbMemeToDisplay(result.meme, {
    score: result.score,
    description: result.description,
    textPresence: result.textPresence,
  });
}
