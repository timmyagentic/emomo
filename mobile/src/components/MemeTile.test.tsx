import React from 'react';
import { render, screen } from '@testing-library/react-native';
import { MemeTile } from './MemeTile';
import type { DisplayMeme } from '@/types';

const meme: DisplayMeme = {
  id: 'meme-1',
  url: 'https://r2.emomo.net/meme-1.png',
  width: 320,
  height: 240,
  format: 'png',
  description: '这里是搜索描述',
  category: '开心',
  tags: [],
  score: 0.82,
};

test('keeps search result tiles focused on the image instead of the description', () => {
  render(<MemeTile meme={meme} onPress={jest.fn()} />);

  expect(screen.queryByText('这里是搜索描述')).toBeNull();
  expect(screen.queryByText('开心')).toBeNull();
  expect(screen.getByText('82%')).toBeTruthy();
});
