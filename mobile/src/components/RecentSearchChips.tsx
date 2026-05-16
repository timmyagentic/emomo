import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import type { SearchHistoryEntry } from '@/storage/searchHistory';

interface RecentSearchChipsProps {
  items: SearchHistoryEntry[];
  onPick: (query: string) => void;
  onClear: () => void;
}

export function RecentSearchChips({ items, onPick, onClear }: RecentSearchChipsProps) {
  if (items.length === 0) {
    return null;
  }

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.title}>最近搜索</Text>
        <Pressable accessibilityRole="button" onPress={onClear}>
          <Text style={styles.clear}>清空</Text>
        </Pressable>
      </View>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.chips}>
        {items.slice(0, 8).map((item) => (
          <Pressable key={item.query} accessibilityRole="button" onPress={() => onPick(item.query)} style={styles.chip}>
            <Text numberOfLines={1} style={styles.chipText}>
              {item.query}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    gap: 8,
  },
  header: {
    alignItems: 'center',
    flexDirection: 'row',
    justifyContent: 'space-between',
  },
  title: {
    color: '#59635d',
    fontSize: 12,
    fontWeight: '800',
  },
  clear: {
    color: '#4c6f64',
    fontSize: 12,
    fontWeight: '700',
  },
  chips: {
    gap: 8,
    paddingRight: 12,
  },
  chip: {
    maxWidth: 180,
    borderRadius: 8,
    backgroundColor: '#e9f0ec',
    paddingHorizontal: 11,
    paddingVertical: 8,
  },
  chipText: {
    color: '#27332d',
    fontSize: 13,
    fontWeight: '700',
  },
});

