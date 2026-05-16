import { FlatList, StyleSheet, View } from 'react-native';
import type { DisplayMeme } from '@/types';
import { InlineState } from './InlineState';
import { MemeTile } from './MemeTile';

interface MemeMasonryListProps {
  data: DisplayMeme[];
  isLoading?: boolean;
  emptyTitle: string;
  emptyMessage?: string;
  onPick: (meme: DisplayMeme) => void;
}

export function MemeMasonryList({ data, isLoading = false, emptyTitle, emptyMessage, onPick }: MemeMasonryListProps) {
  if (!isLoading && data.length === 0) {
    return <InlineState title={emptyTitle} message={emptyMessage} />;
  }

  return (
    <FlatList
      data={data}
      keyExtractor={(item) => item.id}
      numColumns={2}
      columnWrapperStyle={styles.row}
      contentContainerStyle={styles.content}
      renderItem={({ item }) => (
        <View style={styles.cell}>
          <MemeTile meme={item} onPress={onPick} />
        </View>
      )}
      scrollEnabled={false}
    />
  );
}

const styles = StyleSheet.create({
  content: {
    paddingBottom: 18,
  },
  row: {
    gap: 10,
  },
  cell: {
    flex: 1,
  },
});

