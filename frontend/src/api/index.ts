import { create, fromJson, toJson } from '@bufbuild/protobuf';
import {
  SearchRequestSchema,
  SearchResponseSchema,
  ListMemesResponseSchema,
  GetMemeResponseSchema,
  GetCategoriesResponseSchema,
  GetStatsResponseSchema,
  SearchProgressEventSchema,
  SearchStage,
} from '@gen/emomo/v1/api_pb';
import type { SearchRequest } from '@gen/emomo/v1/api_pb';
import { TextPresence } from '@gen/emomo/v1/types_pb';
import {
  pbSearchResultToDisplay,
  pbMemeToDisplay,
  type DisplayMeme,
} from '../types';
import { logWarn } from '../utils/logger';

/**
 * The base URL for the API, loaded from environment variables.
 * Defaults to 'http://localhost:8080/api/v1'.
 */
const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080/api/v1';

/**
 * The API token for authentication, loaded from environment variables.
 */
const API_TOKEN = import.meta.env.VITE_API_TOKEN || '';

export class ApiError extends Error {
  readonly status: number;
  readonly requestId: string;

  constructor(message: string, response: Response) {
    const requestId = response.headers.get('X-Request-ID') ?? '';
    const suffix = requestId ? ` (request_id=${requestId})` : '';
    super(`${message}: ${response.status} ${response.statusText}${suffix}`);
    this.name = 'ApiError';
    this.status = response.status;
    this.requestId = requestId;
  }
}

function ensureOK(response: Response, message: string): void {
  if (!response.ok) {
    throw new ApiError(message, response);
  }
}

function getHeaders(contentType?: string): HeadersInit {
  const headers: HeadersInit = {};
  if (contentType) {
    headers['Content-Type'] = contentType;
  }
  if (API_TOKEN) {
    headers['Authorization'] = `Bearer ${API_TOKEN}`;
  }
  return headers;
}

/** Aggregate stats returned by GET /api/v1/stats, projected for UI consumption. */
export interface StatsView {
  totalMemes: number;
  totalCategories: number;
  availableCollections: string[];
  availableProfiles: string[];
}

/** Categories list projected for UI consumption. */
export interface CategoryView {
  name: string;
  count?: number;
}

/** UI-friendly stage slug carried alongside SearchProgressEvent. */
export type SearchStageSlug =
  | 'query_expansion_start'
  | 'thinking'
  | 'query_expansion_done'
  | 'embedding'
  | 'searching'
  | 'enriching'
  | 'complete'
  | 'error';

/** Streaming progress event surface used by App.tsx — flattens the protobuf
 *  oneof payload into a denormalized view that maps cleanly to UI state. */
export interface SearchProgressView {
  stage: SearchStageSlug;
  /** Human-readable message attached to the stage (Chinese strings from backend). */
  message?: string;
  /** Incremental LLM thinking chunk; only set on stage === 'thinking'. */
  thinkingText?: string;
  /** Whether the chunk is an incremental delta (always true for thinking events). */
  isDelta?: boolean;
  /** Final expanded query, set on stage === 'query_expansion_done'. */
  expandedQuery?: string;
  /** Final search results, set on stage === 'complete'. */
  results?: DisplayMeme[];
  /** Total result count, set on stage === 'complete'. */
  total?: number;
  /** Original query echoed back, set on stage === 'complete'. */
  query?: string;
  /** Backend collection used for this search, set on stage === 'complete'. */
  collection?: string;
  /** Backend profile used for this search, set on stage === 'complete'. */
  profile?: string;
  /** Error message, set on stage === 'error'. */
  error?: string;
}

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

/** Builds a SearchRequest message body for the search endpoints. Empty fields
 *  default to proto3 zero values (UNSPECIFIED / 0 / "") which the backend
 *  treats as "not provided". */
function buildSearchRequest(query: string, topK: number, profile?: string, category?: string): SearchRequest {
  return create(SearchRequestSchema, {
    query,
    topK,
    category: category ?? '',
    textPresence: TextPresence.UNSPECIFIED,
    collection: '',
    profile: profile ?? '',
  });
}

/** POST /api/v1/search — non-streaming hybrid search. */
export async function searchMemes(
  query: string,
  topK: number = 100,
  category?: string,
  profile?: string,
): Promise<{ results: DisplayMeme[]; total: number }> {
  const request = buildSearchRequest(query, topK, profile, category);
  const response = await fetch(`${API_BASE}/search`, {
    method: 'POST',
    headers: getHeaders('application/json'),
    body: JSON.stringify(toJson(SearchRequestSchema, request)),
  });

  ensureOK(response, 'Search failed');

  const body = await response.json();
  const decoded = fromJson(SearchResponseSchema, body);
  return {
    results: decoded.results.map(pbSearchResultToDisplay),
    total: decoded.total,
  };
}

/** GET /api/v1/categories. */
export async function getCategories(): Promise<CategoryView[]> {
  const response = await fetch(`${API_BASE}/categories`, {
    headers: getHeaders(),
  });
  ensureOK(response, 'Failed to fetch categories');
  const decoded = fromJson(GetCategoriesResponseSchema, await response.json());
  return decoded.categories.map((name) => ({ name }));
}

/** GET /api/v1/memes — paginated browse. */
export async function getMemes(
  limit: number = 30,
  offset: number = 0,
  category?: string,
  signal?: AbortSignal,
): Promise<{ results: DisplayMeme[]; total: number }> {
  const params = new URLSearchParams({
    limit: limit.toString(),
    offset: offset.toString(),
  });
  if (category) {
    params.append('category', category);
  }

  const response = await fetch(`${API_BASE}/memes?${params}`, {
    headers: getHeaders(),
    signal,
  });
  ensureOK(response, 'Failed to fetch memes');

  const decoded = fromJson(ListMemesResponseSchema, await response.json());
  return {
    results: decoded.results.map(pbSearchResultToDisplay),
    total: decoded.total,
  };
}

/** GET /api/v1/memes/:id. */
export async function getMeme(id: string): Promise<DisplayMeme> {
  const response = await fetch(`${API_BASE}/memes/${encodeURIComponent(id)}`, {
    headers: getHeaders(),
  });
  ensureOK(response, 'Failed to fetch meme');
  const decoded = fromJson(GetMemeResponseSchema, await response.json());
  if (!decoded.meme) {
    throw new Error('Empty meme response');
  }
  return pbMemeToDisplay(decoded.meme);
}

/** GET /api/v1/stats. */
export async function getStats(): Promise<StatsView> {
  const response = await fetch(`${API_BASE}/stats`, {
    headers: getHeaders(),
  });
  ensureOK(response, 'Failed to fetch stats');
  const decoded = fromJson(GetStatsResponseSchema, await response.json());
  return {
    totalMemes: Number(decoded.totalMemes),
    totalCategories: decoded.totalCategories,
    availableCollections: decoded.availableCollections,
    availableProfiles: decoded.availableProfiles,
  };
}

/** POST /api/v1/search/stream — SSE streaming search. */
export async function searchMemesStream(
  query: string,
  topK: number = 100,
  onProgress: (event: SearchProgressView) => void,
  signal?: AbortSignal,
  profile?: string,
): Promise<void> {
  const request = buildSearchRequest(query, topK, profile);
  const response = await fetch(`${API_BASE}/search/stream`, {
    method: 'POST',
    headers: getHeaders('application/json'),
    body: JSON.stringify(toJson(SearchRequestSchema, request)),
    signal,
  });

  ensureOK(response, 'Search failed');

  const reader = response.body?.getReader();
  if (!reader) {
    throw new Error('Response body is not readable');
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
        const dataPart = line.slice(6);
        if (!dataPart) continue;

        let json: unknown;
        try {
          json = JSON.parse(dataPart);
        } catch (err) {
          logWarn('Failed to parse SSE data line', { data: dataPart, error: err });
          continue;
        }

        let event;
        try {
          event = fromJson(SearchProgressEventSchema, json as Parameters<typeof fromJson>[1]);
        } catch (err) {
          logWarn('Failed to decode SearchProgressEvent', { error: err, json });
          continue;
        }

        const view = projectProgressEvent(event);
        if (view) {
          onProgress(view);
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

function projectProgressEvent(event: ReturnType<typeof fromJson<typeof SearchProgressEventSchema>>): SearchProgressView | undefined {
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
