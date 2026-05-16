import { searchMemesStream } from './index';

const makeResponse = (body: unknown, init?: { ok?: boolean; body?: unknown; statusText?: string }) => ({
  ok: init?.ok ?? true,
  statusText: init?.statusText ?? 'OK',
  body: init?.body,
  json: jest.fn().mockResolvedValue(body),
});

beforeEach(() => {
  jest.resetAllMocks();
});

test('falls back to non-streaming search when response body is not readable', async () => {
  const fetchMock = jest
    .fn()
    .mockResolvedValueOnce(makeResponse({}, { body: undefined }))
    .mockResolvedValueOnce(
      makeResponse({
        results: [
          {
            meme: {
              id: 'meme-1',
              url: 'https://cdn.example.com/meme.png',
              image_info: { width: 640, height: 480, format: 2 },
              tags: ['职场'],
              category: 'reaction',
            },
            score: 0.91,
            description: '无奈但礼貌',
            text_presence: 2,
          },
        ],
        total: 1,
        query: '老板突然沉默',
        expanded_query: '老板突然沉默 无奈 礼貌',
        collection: 'qwen3vl',
        profile: 'default',
      })
    );
  global.fetch = fetchMock as unknown as typeof fetch;
  const events: { stage: string; total?: number; results?: unknown[] }[] = [];

  await searchMemesStream(
    '老板突然沉默',
    { topK: 20 },
    (event) => {
      events.push(event);
    }
  );

  expect(fetchMock).toHaveBeenCalledTimes(2);
  expect(String(fetchMock.mock.calls[0][0])).toContain('/search/stream');
  expect(String(fetchMock.mock.calls[1][0])).toContain('/search');
  expect(events.map((event) => event.stage)).toEqual(['query_expansion_start', 'searching', 'complete']);
  expect(events[2].total).toBe(1);
  expect(events[2].results?.[0]).toMatchObject({
    id: 'meme-1',
    url: 'https://cdn.example.com/meme.png',
    description: '无奈但礼貌',
    tags: ['职场'],
    width: 640,
    height: 480,
    format: 'png',
    textPresence: 'with_text',
  });
});
