import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react-native';
import { SearchComposer } from './SearchComposer';

test('renders mobile text presence filter choices', () => {
  const onTextPresenceFilterChange = jest.fn();

  render(
    <SearchComposer
      value="猫咪"
      onChangeText={jest.fn()}
      onSubmit={jest.fn()}
      textPresenceFilter="all"
      onTextPresenceFilterChange={onTextPresenceFilterChange}
    />
  );

  expect(screen.getByText('文字')).toBeTruthy();
  expect(screen.getByText('全部')).toBeTruthy();
  expect(screen.getByText('有文字')).toBeTruthy();
  expect(screen.getByText('无文字')).toBeTruthy();

  fireEvent.press(screen.getByText('有文字'));

  expect(onTextPresenceFilterChange).toHaveBeenCalledWith('with_text');
});
