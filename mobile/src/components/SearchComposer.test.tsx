import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react-native';
import { SearchComposer } from './SearchComposer';

test('renders the mobile search input without result filters', () => {
  const onSubmit = jest.fn();

  render(
    <SearchComposer
      value="猫咪"
      onChangeText={jest.fn()}
      onSubmit={onSubmit}
    />
  );

  expect(screen.getByLabelText('搜索表情')).toBeTruthy();
  expect(screen.queryByText('有文字')).toBeNull();

  fireEvent.press(screen.getByText('搜索'));

  expect(onSubmit).toHaveBeenCalledTimes(1);
});
