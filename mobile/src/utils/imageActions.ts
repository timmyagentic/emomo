import * as Clipboard from 'expo-clipboard';
import { Directory, File, Paths } from 'expo-file-system';
import * as MediaLibrary from 'expo-media-library';
import * as Sharing from 'expo-sharing';
import { Linking, Platform } from 'react-native';
import type { DisplayMeme } from '@/types';

function extensionFor(meme: DisplayMeme): string {
  switch (meme.format) {
    case 'jpg':
      return 'jpg';
    case 'png':
      return 'png';
    case 'webp':
      return 'webp';
    default:
      return 'png';
  }
}

async function downloadMeme(meme: DisplayMeme) {
  const directory = new Directory(Paths.cache, 'emomo');
  if (!directory.exists) {
    directory.create({ idempotent: true, intermediates: true });
  }
  const file = new File(directory, `${meme.id || Date.now()}.${extensionFor(meme)}`);
  return File.downloadFileAsync(meme.url, file, { idempotent: true });
}

export async function shareMeme(meme: DisplayMeme): Promise<void> {
  if (await Sharing.isAvailableAsync()) {
    const file = await downloadMeme(meme);
    await Sharing.shareAsync(file.uri, {
      dialogTitle: '分享表情包',
      mimeType: mimeTypeFor(meme),
      UTI: Platform.OS === 'ios' ? 'public.image' : undefined,
    });
    return;
  }

  await Linking.openURL(meme.url);
}

export async function saveMemeToLibrary(meme: DisplayMeme): Promise<void> {
  const file = await downloadMeme(meme);
  await MediaLibrary.saveToLibraryAsync(file.uri);
}

export async function copyMemeImage(meme: DisplayMeme): Promise<void> {
  const file = await downloadMeme(meme);
  await Clipboard.setImageAsync(await file.base64());
}

function mimeTypeFor(meme: DisplayMeme): string {
  switch (meme.format) {
    case 'jpg':
      return 'image/jpeg';
    case 'png':
      return 'image/png';
    case 'webp':
      return 'image/webp';
    default:
      return 'image/png';
  }
}
