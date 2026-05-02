import type {
  Meme,
  SearchResponse,
  Category,
  CategoriesResponse,
  MemesListResponse,
  SearchResult,
  StatsResponse,
} from '../types';

/**
 * The base URL for the API, loaded from environment variables.
 * Defaults to 'http://localhost:8080/api/v1'.
 */
const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080/api/v1';

/**
 * The API token for authentication, loaded from environment variables.
 */
const API_TOKEN = import.meta.env.VITE_API_TOKEN || '';

/**
 * Generates request headers, including the Authorization header if an API token is present.
 *
 * @param contentType - The optional Content-Type header value (e.g., 'application/json').
 * @returns A HeadersInit object containing the configured headers.
 */
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

/**
 * Normalizes API search results into the unified `Meme` format for UI consistency.
 *
 * @param results - The list of results from the backend (SearchResponse['results']).
 * @returns An array of normalized `Meme` objects.
 */
function normalizeResults(results: SearchResult[]): Meme[] {
  return results.map((result) => ({
    id: result.id,
    url: result.url,
    score: result.score,
    description: result.description,
    vlm_description: result.description, // keep for backward compatibility
    category: result.category,
    tags: result.tags,
    width: result.image_info?.width ?? result.width,
    height: result.image_info?.height ?? result.height,
    image_info: result.image_info,
    format: normalizeImageFormat(result.image_info?.format),
  }));
}

function normalizeImageFormat(format?: number | string): string | undefined {
  if (format === undefined || format === null) {
    return undefined;
  }
  switch (format) {
    case 1:
    case '1':
    case 'jpg':
    case 'jpeg':
    case 'IMAGE_FORMAT_JPEG':
      return 'jpg';
    case 2:
    case '2':
    case 'png':
    case 'IMAGE_FORMAT_PNG':
      return 'png';
    case 3:
    case '3':
    case 'webp':
    case 'IMAGE_FORMAT_WEBP':
      return 'webp';
    default:
      return undefined;
  }
}

/**
 * Searches for memes based on a text query.
 *
 * @param query - The search query text.
 * @param topK - The number of top results to return. Defaults to 20.
 * @param category - An optional category filter.
 * @returns A promise that resolves to an object containing the list of found memes and the total count.
 * @throws An error if the search request fails.
 */
export async function searchMemes(
  query: string,
  topK: number = 20,
  category?: string,
  profile?: string
): Promise<{ results: Meme[]; total: number }> {
  const response = await fetch(`${API_BASE}/search`, {
    method: 'POST',
    headers: getHeaders('application/json'),
    body: JSON.stringify({
      query,
      top_k: topK,
      category,
      profile,
    }),
  });

  if (!response.ok) {
    throw new Error(`Search failed: ${response.statusText}`);
  }

  const data: SearchResponse = await response.json();
  return {
    results: normalizeResults(data.results),
    total: data.total,
  };
}

/**
 * Retrieves all available meme categories.
 *
 * @returns A promise that resolves to an array of `Category` objects.
 * @throws An error if the request fails.
 */
export async function getCategories(): Promise<Category[]> {
  const response = await fetch(`${API_BASE}/categories`, {
    headers: getHeaders(),
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch categories: ${response.statusText}`);
  }

  // Backend returns { categories: string[], total: number }
  const data: CategoriesResponse = await response.json();
  
  // Convert string array to Category objects
  return data.categories.map((name) => ({
    name,
    count: undefined, // Backend doesn't provide count per category in this endpoint
  }));
}

/**
 * Retrieves a paginated list of memes.
 *
 * @param limit - The maximum number of memes to return. Defaults to 30.
 * @param offset - The number of memes to skip (for pagination). Defaults to 0.
 * @param category - An optional category filter.
 * @param signal - An optional AbortSignal to cancel the request.
 * @returns A promise that resolves to an object containing the list of memes and the total count.
 * @throws An error if the request fails.
 */
export async function getMemes(
  limit: number = 30,
  offset: number = 0,
  category?: string,
  signal?: AbortSignal
): Promise<{ results: Meme[]; total: number }> {
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

  if (!response.ok) {
    throw new Error(`Failed to fetch memes: ${response.statusText}`);
  }

  const data: MemesListResponse = await response.json();
  return {
    results: normalizeResults(data.results),
    total: data.total,
  };
}

/**
 * Retrieves a single meme by its ID.
 *
 * @param id - The unique identifier of the meme.
 * @returns A promise that resolves to the `Meme` object.
 * @throws An error if the request fails.
 */
export async function getMeme(id: string): Promise<Meme> {
  const response = await fetch(`${API_BASE}/memes/${id}`, {
    headers: getHeaders(),
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch meme: ${response.statusText}`);
  }

  return response.json();
}

/**
 * Retrieves aggregate backend stats for the header and system status.
 *
 * @returns A promise that resolves to aggregate stats.
 * @throws An error if the request fails.
 */
export async function getStats(): Promise<StatsResponse> {
  const response = await fetch(`${API_BASE}/stats`, {
    headers: getHeaders(),
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch stats: ${response.statusText}`);
  }

  return response.json();
}

// Search stage types
export type SearchStage =
  | 'query_expansion_start'
  | 'thinking'
  | 'query_expansion_done'
  | 'embedding'
  | 'searching'
  | 'enriching'
  | 'complete'
  | 'error';

// Search progress event from SSE
export interface SearchProgressEvent {
  stage: SearchStage;
  message?: string;
  thinking_text?: string;
  is_delta?: boolean;
  expanded_query?: string;
  results?: Meme[];
  total?: number;
  query?: string;
  collection?: string;
  profile?: string;
  error?: string;
}

// Stream search memes with progress updates
export async function searchMemesStream(
  query: string,
  topK: number = 20,
  onProgress: (event: SearchProgressEvent) => void,
  signal?: AbortSignal,
  profile?: string
): Promise<void> {
  const response = await fetch(`${API_BASE}/search/stream`, {
    method: 'POST',
    headers: getHeaders('application/json'),
    body: JSON.stringify({
      query,
      top_k: topK,
      profile,
    }),
    signal,
  });

  if (!response.ok) {
    throw new Error(`Search failed: ${response.statusText}`);
  }

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

      // Process complete SSE events
      const lines = buffer.split('\n');
      buffer = lines.pop() || ''; // Keep incomplete line in buffer

      let currentEventType = 'progress';

      for (const line of lines) {
        if (line.startsWith('event: ')) {
          currentEventType = line.slice(7).trim();
        } else if (line.startsWith('data: ')) {
          const data = line.slice(6);
          try {
            const event = JSON.parse(data) as SearchProgressEvent;
            
            // Normalize results if present
            if (event.results) {
              event.results = normalizeResults(event.results as unknown as SearchResponse['results']);
            }
            
            // Set stage from event type if not present
            if (!event.stage && currentEventType) {
              event.stage = currentEventType as SearchStage;
            }
            
            onProgress(event);
          } catch (e) {
            console.warn('Failed to parse SSE data:', data, e);
          }
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}
