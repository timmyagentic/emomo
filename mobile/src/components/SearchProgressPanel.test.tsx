import React from 'react';
import { render, screen } from '@testing-library/react-native';
import { SearchProgressPanel } from './SearchProgressPanel';

test('renders staged search progress on mobile', () => {
  render(
    <SearchProgressPanel
      progress={{
        stage: 'searching',
        message: '正在匹配表情库...',
        expandedQuery: '开心 表情包',
      }}
      thinkingText="用户想找表达开心的表情"
    />
  );

  expect(screen.getByText('搜索中')).toBeTruthy();
  expect(screen.getByText('3/4')).toBeTruthy();
  expect(screen.getByText('理解')).toBeTruthy();
  expect(screen.getByText('向量')).toBeTruthy();
  expect(screen.getByText('搜索')).toBeTruthy();
  expect(screen.getByText('整理')).toBeTruthy();
  expect(screen.getByText('正在匹配表情库...')).toBeTruthy();
  expect(screen.getByText('扩写：开心 表情包')).toBeTruthy();
  expect(screen.getByText('用户想找表达开心的表情')).toBeTruthy();
});
