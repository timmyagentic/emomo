import { Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import type { TextPresenceFilter } from '@/types';

interface SearchComposerProps {
  value: string;
  onChangeText: (value: string) => void;
  onSubmit: () => void;
  textPresenceFilter: TextPresenceFilter;
  onTextPresenceFilterChange: (filter: TextPresenceFilter) => void;
  onCancel?: () => void;
  isLoading?: boolean;
}

const textPresenceOptions: { value: TextPresenceFilter; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'with_text', label: '有文字' },
  { value: 'without_text', label: '无文字' },
];

export function SearchComposer({
  value,
  onChangeText,
  onSubmit,
  textPresenceFilter,
  onTextPresenceFilterChange,
  onCancel,
  isLoading = false,
}: SearchComposerProps) {
  const canSubmit = value.trim().length > 0 && !isLoading;

  return (
    <View style={styles.container}>
      <TextInput
        accessibilityLabel="搜索表情"
        autoCapitalize="none"
        autoCorrect={false}
        multiline
        onChangeText={onChangeText}
        onSubmitEditing={onSubmit}
        placeholder="描述一个情绪、场景或者想发的话"
        placeholderTextColor="#8c928c"
        returnKeyType="search"
        style={styles.input}
        value={value}
      />
      <View style={styles.filterRow}>
        <Text style={styles.filterLabel}>文字</Text>
        <View style={styles.segmentedControl}>
          {textPresenceOptions.map((option) => {
            const selected = option.value === textPresenceFilter;
            return (
              <Pressable
                key={option.value}
                accessibilityRole="button"
                accessibilityState={{ selected, disabled: isLoading }}
                disabled={isLoading}
                onPress={() => onTextPresenceFilterChange(option.value)}
                style={[styles.segmentButton, selected && styles.segmentButtonActive, isLoading && styles.segmentButtonDisabled]}
              >
                <Text style={[styles.segmentLabel, selected && styles.segmentLabelActive]}>{option.label}</Text>
              </Pressable>
            );
          })}
        </View>
      </View>
      {isLoading ? (
        <Pressable accessibilityRole="button" onPress={onCancel} style={styles.cancelButton}>
          <Text style={styles.cancelLabel}>取消</Text>
        </Pressable>
      ) : (
        <Pressable
          accessibilityRole="button"
          disabled={!canSubmit}
          onPress={onSubmit}
          style={[styles.submitButton, !canSubmit && styles.submitDisabled]}
        >
          <Text style={styles.submitLabel}>搜索</Text>
        </Pressable>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    borderRadius: 8,
    backgroundColor: '#ffffff',
    borderColor: '#d9dfda',
    borderWidth: 1,
    padding: 10,
    gap: 10,
  },
  input: {
    minHeight: 52,
    maxHeight: 104,
    color: '#111111',
    fontSize: 17,
    lineHeight: 23,
    padding: 0,
    textAlignVertical: 'top',
  },
  filterRow: {
    alignItems: 'center',
    flexDirection: 'row',
    gap: 10,
  },
  filterLabel: {
    color: '#68736c',
    fontSize: 13,
    fontWeight: '800',
  },
  segmentedControl: {
    flex: 1,
    flexDirection: 'row',
    gap: 4,
    padding: 4,
    borderRadius: 8,
    backgroundColor: '#f2f5f1',
  },
  segmentButton: {
    alignItems: 'center',
    justifyContent: 'center',
    flex: 1,
    minHeight: 32,
    borderRadius: 6,
    paddingHorizontal: 8,
  },
  segmentButtonActive: {
    backgroundColor: '#f6d86b',
  },
  segmentButtonDisabled: {
    opacity: 0.65,
  },
  segmentLabel: {
    color: '#58635d',
    fontSize: 13,
    fontWeight: '800',
  },
  segmentLabelActive: {
    color: '#111111',
  },
  submitButton: {
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: 42,
    borderRadius: 8,
    backgroundColor: '#111111',
  },
  submitDisabled: {
    backgroundColor: '#9ca3a0',
  },
  submitLabel: {
    color: '#ffffff',
    fontSize: 15,
    fontWeight: '800',
  },
  cancelButton: {
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: 42,
    borderRadius: 8,
    backgroundColor: '#f6d86b',
  },
  cancelLabel: {
    color: '#111111',
    fontSize: 15,
    fontWeight: '800',
  },
});
