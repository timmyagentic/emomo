import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react-native';
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

test('opens app store readiness information from the header', async () => {
  render(<App />);

  fireEvent.press(await screen.findByLabelText('打开关于与隐私信息'));

  expect(screen.getByText('关于 emomo')).toBeTruthy();
  expect(screen.getByText('版本 1.0.0 (1)')).toBeTruthy();
  expect(screen.getByText('隐私政策')).toBeTruthy();
  expect(screen.getByText('搜索历史只保存在本机，可以随时清空。')).toBeTruthy();
  expect(screen.getByText('支持与反馈')).toBeTruthy();
});
