import { StyleSheet, Text, View, type DimensionValue } from 'react-native';
import type { SearchProgressView, SearchStageSlug } from '@/types';

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

const PROGRESS_STEPS: { key: SearchStageSlug; label: string }[] = [
  { key: 'query_expansion_start', label: '理解' },
  { key: 'embedding', label: '向量' },
  { key: 'searching', label: '搜索' },
  { key: 'enriching', label: '整理' },
];

function getStepIndex(stage: SearchProgressView['stage']): number {
  if (stage === 'thinking' || stage === 'query_expansion_done') {
    return 0;
  }
  if (stage === 'complete') {
    return PROGRESS_STEPS.length - 1;
  }
  const index = PROGRESS_STEPS.findIndex((step) => step.key === stage);
  return index >= 0 ? index : 0;
}

export function SearchProgressPanel({ progress, thinkingText }: SearchProgressPanelProps) {
  if (!progress) {
    return null;
  }

  const currentIndex = getStepIndex(progress.stage);
  const progressPercent: DimensionValue = `${(currentIndex / (PROGRESS_STEPS.length - 1)) * 100}%`;

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.stage}>{STAGE_LABELS[progress.stage]}</Text>
        <Text style={styles.counter}>{currentIndex + 1}/{PROGRESS_STEPS.length}</Text>
      </View>
      <View style={styles.steps}>
        <View style={styles.progressLine}>
          <View style={[styles.progressFill, { width: progressPercent }]} />
        </View>
        <View style={styles.stepRow}>
          {PROGRESS_STEPS.map((step, index) => {
            const isCompleted = index < currentIndex;
            const isCurrent = index === currentIndex;
            return (
              <View key={step.key} style={styles.stepItem}>
                <View
                  style={[
                    styles.stepDot,
                    isCompleted && styles.stepDotCompleted,
                    isCurrent && styles.stepDotCurrent,
                  ]}
                >
                  {isCurrent ? <View style={styles.stepDotInner} /> : null}
                </View>
                <Text
                  style={[
                    styles.stepLabel,
                    isCompleted && styles.stepLabelCompleted,
                    isCurrent && styles.stepLabelCurrent,
                  ]}
                >
                  {step.label}
                </Text>
              </View>
            );
          })}
        </View>
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
  counter: {
    color: '#8e9b94',
    fontSize: 12,
    fontWeight: '800',
  },
  steps: {
    minHeight: 44,
    justifyContent: 'center',
    position: 'relative',
  },
  progressLine: {
    position: 'absolute',
    left: 18,
    right: 18,
    top: 11,
    height: 2,
    borderRadius: 1,
    backgroundColor: '#3a403d',
  },
  progressFill: {
    height: 2,
    borderRadius: 1,
    backgroundColor: '#65d6a5',
  },
  stepRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
  },
  stepItem: {
    alignItems: 'center',
    flex: 1,
    gap: 6,
    zIndex: 1,
  },
  stepDot: {
    alignItems: 'center',
    justifyContent: 'center',
    width: 22,
    height: 22,
    borderRadius: 11,
    borderColor: '#3a403d',
    borderWidth: 2,
    backgroundColor: '#151817',
  },
  stepDotCompleted: {
    borderColor: '#65d6a5',
    backgroundColor: '#65d6a5',
  },
  stepDotCurrent: {
    borderColor: '#f6d86b',
    backgroundColor: '#151817',
  },
  stepDotInner: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#f6d86b',
  },
  stepLabel: {
    color: '#8e9b94',
    fontSize: 11,
    fontWeight: '800',
  },
  stepLabelCompleted: {
    color: '#65d6a5',
  },
  stepLabelCurrent: {
    color: '#f6d86b',
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
