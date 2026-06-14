import { create, fromJson, toJson } from '@bufbuild/protobuf';
import {
  GetMemeResponseSchema,
  GetStatsResponseSchema,
  ListMemesResponseSchema,
  SearchProgressEventSchema,
  SearchRequestSchema,
  SearchResponseSchema,
  SearchStage,
} from '@gen/emomo/v1/api_pb';
import type { SearchProgressEvent, SearchRequest } from '@gen/emomo/v1/api_pb';
import {
  pbMemeToDisplay,
  pbSearchResultToDisplay,
  type DisplayMeme,
  type SearchProgressView,
  type SearchStageSlug,
  type StatsView,
} from '@/types';

const API_BASE = process.env.EXPO_PUBLIC_API_BASE || 'http://localhost:8080/api/v1';
const DEFAULT_TOP_K = 50;

const STAGE_TO_SLUG: Record<SearchStage, SearchStageSlug | undefined> = {
  [SearchStage.UNSPECIFIED]: undefined,
  [SearchStage.QUERY_EXPANSION_START]: 'query_expansion_start',
  [SearchStage.THINKING]: 'thinking',
  [SearchStage.QUERY_EXPANSION_DONE]: 'query_expansion_done',
  [SearchStage.EMBEDDING]: 'embedding',
  [SearchStage.SEARCHING]: 'searching',
  [SearchStage.ENRICHING]: 'enriching',
  [SearchStage.COMPLETE]: 'complete',
  [SearchStage.ERROR]: 'error',
};

export interface SearchOptions {
  topK?: number;
  category?: string;
  profile?: string;
}

function getHeaders(contentType?: string): HeadersInit {
  const headers: HeadersInit = {};
  if (contentType) {
    headers['Content-Type'] = contentType;
  }
  return headers;
}

function buildSearchRequest(query: string, options: SearchOptions = {}): SearchRequest {
  return create(SearchRequestSchema, {
    query,
    topK: options.topK ?? DEFAULT_TOP_K,
    category: options.category ?? '',
    collection: '',
    profile: options.profile ?? '',
  });
}

export async function getStats(signal?: AbortSignal): Promise<StatsView> {
  const response = await fetch(`${API_BASE}/stats`, {
    headers: getHeaders(),
    signal,
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch stats: ${response.statusText}`);
  }
  const decoded = fromJson(GetStatsResponseSchema, await response.json());
  return {
    totalMemes: Number(decoded.totalMemes),
    totalCategories: decoded.totalCategories,
    availableCollections: decoded.availableCollections,
    availableProfiles: decoded.availableProfiles,
  };
}

export async function getMemes(
  limit: number = 30,
  offset: number = 0,
  category?: string,
  signal?: AbortSignal
): Promise<{ results: DisplayMeme[]; total: number }> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  if (category) {
    params.append('category', category);
  }

  const response = await fetch(`${API_BASE}/memes?${params.toString()}`, {
    headers: getHeaders(),
    signal,
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch memes: ${response.statusText}`);
  }

  const decoded = fromJson(ListMemesResponseSchema, await response.json());
  return {
    results: decoded.results.map(pbSearchResultToDisplay),
    total: decoded.total,
  };
}

export async function getMeme(id: string, signal?: AbortSignal): Promise<DisplayMeme> {
  const response = await fetch(`${API_BASE}/memes/${encodeURIComponent(id)}`, {
    headers: getHeaders(),
    signal,
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch meme: ${response.statusText}`);
  }
  const decoded = fromJson(GetMemeResponseSchema, await response.json());
  if (!decoded.meme) {
    throw new Error('Empty meme response');
  }
  return pbMemeToDisplay(decoded.meme);
}

export async function searchMemes(
  query: string,
  options: SearchOptions = {},
  signal?: AbortSignal
): Promise<{ results: DisplayMeme[]; total: number; query: string; expandedQuery?: string }> {
  const request = buildSearchRequest(query, options);
  const response = await fetch(`${API_BASE}/search`, {
    method: 'POST',
    headers: getHeaders('application/json'),
    body: JSON.stringify(toJson(SearchRequestSchema, request)),
    signal,
  });
  if (!response.ok) {
    throw new Error(`Search failed: ${response.statusText}`);
  }

  const decoded = fromJson(SearchResponseSchema, await response.json());
  return {
    results: decoded.results.map(pbSearchResultToDisplay),
    total: decoded.total,
    query: decoded.query,
    expandedQuery: decoded.expandedQuery || undefined,
  };
}

export async function searchMemesStream(
  query: string,
  options: SearchOptions = {},
  onProgress: (event: SearchProgressView) => void,
  signal?: AbortSignal
): Promise<void> {
  const request = buildSearchRequest(query, options);
  const response = await fetch(`${API_BASE}/search/stream`, {
    method: 'POST',
    headers: getHeaders('application/json'),
    body: JSON.stringify(toJson(SearchRequestSchema, request)),
    signal,
  });

  if (!response.ok) {
    throw new Error(`Search failed: ${response.statusText}`);
  }

  const reader = (response.body as ReadableStream<Uint8Array> | undefined)?.getReader?.();
  if (!reader) {
    await searchMemesFallback(query, options, onProgress, signal);
    return;
  }

  const decoder = new TextDecoder();
  let buffer = '';

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        const view = decodeSSEDataLine(line.slice(6));
        if (view) {
          onProgress(view);
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

async function searchMemesFallback(
  query: string,
  options: SearchOptions,
  onProgress: (event: SearchProgressView) => void,
  signal?: AbortSignal
): Promise<void> {
  onProgress({
    stage: 'query_expansion_start',
    message: 'AI 正在理解搜索意图...',
  });
  onProgress({
    stage: 'searching',
    message: '正在搜索表情包...',
  });

  const result = await searchMemes(query, options, signal);
  onProgress({
    stage: 'complete',
    results: result.results,
    total: result.total,
    query: result.query,
    expandedQuery: result.expandedQuery,
  });
}

function decodeSSEDataLine(dataPart: string): SearchProgressView | undefined {
  if (!dataPart) {
    return undefined;
  }

  try {
    const event = fromJson(SearchProgressEventSchema, JSON.parse(dataPart));
    return projectProgressEvent(event);
  } catch {
    return undefined;
  }
}

function projectProgressEvent(event: SearchProgressEvent): SearchProgressView | undefined {
  const stage = STAGE_TO_SLUG[event.stage];
  if (!stage) {
    return undefined;
  }

  const view: SearchProgressView = {
    stage,
    message: event.message || undefined,
  };

  switch (event.payload.case) {
    case 'thinking':
      view.thinkingText = event.payload.value.text;
      view.isDelta = event.payload.value.isDelta;
      break;
    case 'expansion':
      view.expandedQuery = event.payload.value.expandedQuery;
      break;
    case 'complete':
      view.results = event.payload.value.results.map(pbSearchResultToDisplay);
      view.total = event.payload.value.total;
      view.query = event.payload.value.query;
      view.expandedQuery = event.payload.value.expandedQuery || undefined;
      view.collection = event.payload.value.collection || undefined;
      view.profile = event.payload.value.profile || undefined;
      break;
    case 'error':
      view.error = event.payload.value.error;
      break;
    default:
      break;
  }

  return view;
}
