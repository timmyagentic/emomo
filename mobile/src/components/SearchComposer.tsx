import { Pressable, StyleSheet, Text, TextInput, View } from 'react-native';

interface SearchComposerProps {
  value: string;
  onChangeText: (value: string) => void;
  onSubmit: () => void;
  onCancel?: () => void;
  isLoading?: boolean;
}

export function SearchComposer({ value, onChangeText, onSubmit, onCancel, isLoading = false }: SearchComposerProps) {
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

