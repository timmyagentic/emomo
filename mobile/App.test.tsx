import React from 'react';
import { render, screen } from '@testing-library/react-native';
import App from './App';

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
}));

test('opens directly to the conversational search experience', async () => {
  render(<App />);

  expect(await screen.findByText('今天想找什么表情？')).toBeTruthy();
  expect(screen.getByPlaceholderText('描述一个情绪、场景或者想发的话')).toBeTruthy();
  expect(await screen.findByText('老板突然沉默')).toBeTruthy();
});

