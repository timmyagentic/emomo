import { Image, Pressable, StyleSheet, Text, View } from 'react-native';
import type { DisplayMeme } from '@/types';

interface MemeTileProps {
  meme: DisplayMeme;
  onPress: (meme: DisplayMeme) => void;
}

export function MemeTile({ meme, onPress }: MemeTileProps) {
  const aspectRatio = meme.width && meme.height ? meme.width / meme.height : 1;
  const scoreLabel = typeof meme.score === 'number' ? `${Math.round(meme.score * 100)}%` : null;

  return (
    <Pressable accessibilityRole="button" onPress={() => onPress(meme)} style={styles.tile}>
      <Image source={{ uri: meme.url }} style={[styles.image, { aspectRatio }]} resizeMode="cover" />
      {scoreLabel ? (
        <View style={styles.meta}>
          <Text style={styles.score}>{scoreLabel}</Text>
        </View>
      ) : null}
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
    padding: 9,
  },
  score: {
    color: '#4c6f64',
    fontSize: 11,
    fontWeight: '900',
  },
});
