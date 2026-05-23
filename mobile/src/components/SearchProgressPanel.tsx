import { StyleSheet, Text, View } from 'react-native';
import type { SearchProgressView } from '@/types';

interface SearchProgressPanelProps {
  progress: SearchProgressView | null;
  thinkingText: string;
}

const STAGE_LABELS: Record<SearchProgressView['stage'], string> = {
  query_expansion_start: '理解意图',
  thinking: 'AI 思考',
  query_expansion_done: '扩写完成',
  embedding: '向量检索',
  searching: '搜索中',
  enriching: '整理结果',
  complete: '完成',
  error: '出错',
};

export function SearchProgressPanel({ progress, thinkingText }: SearchProgressPanelProps) {
  if (!progress) {
    return null;
  }

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.stage}>{STAGE_LABELS[progress.stage]}</Text>
        <View style={styles.dot} />
      </View>
      <Text style={styles.message}>{progress.message ?? '正在处理搜索请求...'}</Text>
      {progress.expandedQuery ? <Text style={styles.expanded}>扩写：{progress.expandedQuery}</Text> : null}
      {thinkingText ? <Text style={styles.thinking} numberOfLines={5}>{thinkingText}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    borderRadius: 8,
    backgroundColor: '#151817',
    padding: 14,
    gap: 8,
  },
  header: {
    alignItems: 'center',
    flexDirection: 'row',
    justifyContent: 'space-between',
  },
  stage: {
    color: '#ffffff',
    fontSize: 14,
    fontWeight: '900',
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#65d6a5',
  },
  message: {
    color: '#dce4df',
    fontSize: 13,
    lineHeight: 19,
  },
  expanded: {
    color: '#f6d86b',
    fontSize: 12,
    lineHeight: 18,
  },
  thinking: {
    color: '#aebbb4',
    fontSize: 12,
    lineHeight: 18,
  },
});

