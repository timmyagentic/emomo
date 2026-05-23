import AsyncStorage from '@react-native-async-storage/async-storage';

const STORAGE_KEY = 'emomo.searchHistory.v1';
const MAX_HISTORY_ENTRIES = 20;

export interface SearchHistoryEntry {
  query: string;
  createdAt: string;
}

export function normalizeSearchQuery(query: string): string {
  return query.trim().replace(/\s+/g, ' ');
}

export async function getSearchHistory(): Promise<SearchHistoryEntry[]> {
  const raw = await AsyncStorage.getItem(STORAGE_KEY);
  if (!raw) {
    return [];
  }

  try {
    const parsed = JSON.parse(raw) as SearchHistoryEntry[];
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter((entry) => typeof entry.query === 'string' && typeof entry.createdAt === 'string');
  } catch {
    return [];
  }
}

export async function addSearchHistory(query: string): Promise<SearchHistoryEntry[]> {
  const normalized = normalizeSearchQuery(query);
  if (!normalized) {
    return getSearchHistory();
  }

  const current = await getSearchHistory();
  const next: SearchHistoryEntry[] = [
    {
      query: normalized,
      createdAt: new Date().toISOString(),
    },
    ...current.filter((entry) => normalizeSearchQuery(entry.query) !== normalized),
  ].slice(0, MAX_HISTORY_ENTRIES);

  await AsyncStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  return next;
}

export async function clearSearchHistory(): Promise<void> {
  await AsyncStorage.removeItem(STORAGE_KEY);
}

