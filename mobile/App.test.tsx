import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react-native';
import App from './App';
import { searchMemesStream } from './src/api';

const searchMemesStreamMock = searchMemesStream as jest.MockedFunction<typeof searchMemesStream>;

jest.mock('./src/api', () => ({
  getMemes: jest.fn().mockResolvedValue({ results: [], total: 0 }),
  getStats: jest.fn().mockResolvedValue({
    totalMemes: 5791,
    totalCategories: 0,
    availableCollections: [],
    availableProfiles: [],
  }),
  searchMemesStream: jest.fn(),
}));

jest.mock('./src/storage/searchHistory', () => ({
  addSearchHistory: jest.fn().mockResolvedValue([]),
  clearSearchHistory: jest.fn().mockResolvedValue(undefined),
  getSearchHistory: jest.fn().mockResolvedValue([{ query: '老板突然沉默', createdAt: '2026-05-16T08:00:00.000Z' }]),
  normalizeSearchQuery: jest.fn((query: string) => query.trim()),
}));

beforeEach(() => {
  searchMemesStreamMock.mockReset();
});

test('opens directly to the conversational search experience', async () => {
  render(<App />);

  expect(await screen.findByText('今天想找什么表情？')).toBeTruthy();
  expect(screen.getByPlaceholderText('描述一个情绪、场景或者想发的话')).toBeTruthy();
  expect(screen.queryByText('有文字')).toBeNull();
  expect(await screen.findByText('老板突然沉默')).toBeTruthy();
});

test('opens app store readiness information from the header', async () => {
  render(<App />);

  fireEvent.press(await screen.findByLabelText('打开关于与隐私信息'));

  expect(screen.getByText('关于 emomo')).toBeTruthy();
  expect(screen.getByText('版本 1.0.0')).toBeTruthy();
  expect(screen.getByText('隐私政策')).toBeTruthy();
  expect(screen.getByText('搜索历史只保存在本机，可以随时清空。')).toBeTruthy();
  expect(screen.getByText('支持与反馈')).toBeTruthy();
});

test('filters existing search results by text presence without searching again', async () => {
  searchMemesStreamMock.mockImplementation(async (_query, _options, onProgress) => {
    onProgress({
      stage: 'complete',
      results: [
        {
          id: 'cat-with-text',
          url: 'https://cdn.example.com/with-text.png',
          score: 0.81,
          description: '带文字猫咪测试表情',
          tags: ['猫咪'],
          textPresence: 'with_text',
        },
        {
          id: 'cat-without-text',
          url: 'https://cdn.example.com/without-text.png',
          score: 0.42,
          description: '无文字猫咪测试表情',
          tags: ['猫咪'],
          textPresence: 'without_text',
        },
      ],
      total: 2,
      query: '猫咪',
    });
  });

  render(<App />);

  fireEvent.changeText(await screen.findByLabelText('搜索表情'), '猫咪');
  fireEvent.press(screen.getByText('搜索'));

  await waitFor(() => expect(searchMemesStreamMock).toHaveBeenCalledTimes(1));
  expect(await screen.findByText('81%')).toBeTruthy();
  expect(screen.getByText('42%')).toBeTruthy();
  expect(screen.getByText('展示')).toBeTruthy();

  fireEvent.press(screen.getByText('有文字'));

  expect(screen.getByText('81%')).toBeTruthy();
  await waitFor(() => expect(screen.queryByText('42%')).toBeNull());
  await waitFor(() => expect(searchMemesStreamMock).toHaveBeenCalledTimes(1));
});
