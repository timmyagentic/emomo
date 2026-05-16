import { Image, Pressable, StyleSheet, Text, View } from 'react-native';
import type { DisplayMeme } from '@/types';

interface MemeTileProps {
  meme: DisplayMeme;
  onPress: (meme: DisplayMeme) => void;
}

export function MemeTile({ meme, onPress }: MemeTileProps) {
  const aspectRatio = meme.width && meme.height ? meme.width / meme.height : 1;

  return (
    <Pressable accessibilityRole="button" onPress={() => onPress(meme)} style={styles.tile}>
      <Image source={{ uri: meme.url }} style={[styles.image, { aspectRatio }]} resizeMode="cover" />
      <View style={styles.meta}>
        <Text numberOfLines={2} style={styles.description}>
          {meme.description || meme.category || '表情包'}
        </Text>
        {typeof meme.score === 'number' ? <Text style={styles.score}>{Math.round(meme.score * 100)}%</Text> : null}
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  tile: {
    flex: 1,
    borderRadius: 8,
    backgroundColor: '#ffffff',
    borderColor: '#e0e4e1',
    borderWidth: 1,
    marginBottom: 10,
    overflow: 'hidden',
  },
  image: {
    width: '100%',
    minHeight: 118,
    backgroundColor: '#e8ece9',
  },
  meta: {
    gap: 6,
    padding: 9,
  },
  description: {
    color: '#151817',
    fontSize: 12,
    fontWeight: '700',
    lineHeight: 17,
  },
  score: {
    color: '#4c6f64',
    fontSize: 11,
    fontWeight: '900',
  },
});

