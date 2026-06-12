import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react-native';
import { MemeDetailModal } from './MemeDetailModal';
import type { DisplayMeme } from '@/types';

const meme: DisplayMeme = {
  id: 'meme-1',
  url: 'https://r2.emomo.net/meme-1.png',
  width: 320,
  height: 240,
  format: 'png',
  description: '测试表情',
  tags: ['测试'],
};

test('offers image copy instead of raw link copy in meme detail actions', () => {
  const onCopyImage = jest.fn();

  render(
    <MemeDetailModal
      meme={meme}
      onClose={jest.fn()}
      onShare={jest.fn()}
      onSave={jest.fn()}
      onCopyImage={onCopyImage}
    />
  );

  expect(screen.getByText('复制图片')).toBeTruthy();
  expect(screen.queryByText('复制链接')).toBeNull();

  fireEvent.press(screen.getByText('复制图片'));

  expect(onCopyImage).toHaveBeenCalledWith(meme);
});

test('closes from an iPhone-style left edge swipe', () => {
  const onClose = jest.fn();

  render(
    <MemeDetailModal
      meme={meme}
      onClose={onClose}
      onShare={jest.fn()}
      onSave={jest.fn()}
      onCopyImage={jest.fn()}
    />
  );

  const gestureSurface = screen.getByTestId('meme-detail-gesture-surface');

  gestureSurface.props.onStartShouldSetResponderCapture({ nativeEvent: { pageX: 12, pageY: 24 } });

  expect(gestureSurface.props.onMoveShouldSetResponder({ nativeEvent: { pageX: 52, pageY: 28 } })).toBe(true);

  gestureSurface.props.onResponderRelease({ nativeEvent: { pageX: 108, pageY: 28 } });

  expect(onClose).toHaveBeenCalledTimes(1);
});
