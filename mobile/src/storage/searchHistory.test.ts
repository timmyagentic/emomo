import AsyncStorage from '@react-native-async-storage/async-storage';
import {
  addSearchHistory,
  clearSearchHistory,
  getSearchHistory,
  normalizeSearchQuery,
} from './searchHistory';

const mockStorage = new Map<string, string>();

jest.mock('@react-native-async-storage/async-storage', () => ({
  __esModule: true,
  default: {
    clear: jest.fn(async () => {
      mockStorage.clear();
    }),
    getItem: jest.fn(async (key: string) => mockStorage.get(key) ?? null),
    removeItem: jest.fn(async (key: string) => {
      mockStorage.delete(key);
    }),
    setItem: jest.fn(async (key: string, value: string) => {
      mockStorage.set(key, value);
    }),
  },
}));

beforeEach(async () => {
  jest.useFakeTimers().setSystemTime(new Date('2026-05-16T08:00:00.000Z'));
  await AsyncStorage.clear();
});

afterEach(() => {
  jest.useRealTimers();
});

test('normalizes whitespace and empty search queries', () => {
  expect(normalizeSearchQuery('  老板   突然\t沉默  ')).toBe('老板 突然 沉默');
  expect(normalizeSearchQuery('   ')).toBe('');
});

test('stores newest searches first and deduplicates by normalized query', async () => {
  await addSearchHistory('老板突然沉默');
  jest.setSystemTime(new Date('2026-05-16T08:01:00.000Z'));
  await addSearchHistory('  老板突然沉默  ');
  await addSearchHistory('同事说下周再议');

  const history = await getSearchHistory();

  expect(history.map((entry) => entry.query)).toEqual(['同事说下周再议', '老板突然沉默']);
  expect(history[1].createdAt).toBe('2026-05-16T08:01:00.000Z');
});

test('caps history at 20 entries and clears all entries', async () => {
  for (let index = 0; index < 25; index++) {
    await addSearchHistory(`query ${index}`);
  }

  const history = await getSearchHistory();

  expect(history).toHaveLength(20);
  expect(history[0].query).toBe('query 24');
  expect(history[19].query).toBe('query 5');

  await clearSearchHistory();
  await expect(getSearchHistory()).resolves.toEqual([]);
});
