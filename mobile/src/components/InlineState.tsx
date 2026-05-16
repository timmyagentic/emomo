import { Pressable, StyleSheet, Text, View } from 'react-native';

interface InlineStateProps {
  title: string;
  message?: string;
  actionLabel?: string;
  onAction?: () => void;
}

export function InlineState({ title, message, actionLabel, onAction }: InlineStateProps) {
  return (
    <View style={styles.container}>
      <Text style={styles.title}>{title}</Text>
      {message ? <Text style={styles.message}>{message}</Text> : null}
      {actionLabel && onAction ? (
        <Pressable accessibilityRole="button" onPress={onAction} style={styles.action}>
          <Text style={styles.actionLabel}>{actionLabel}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    padding: 18,
    borderRadius: 8,
    backgroundColor: '#f1f5f3',
    borderColor: '#dce5df',
    borderWidth: 1,
    gap: 8,
  },
  title: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '800',
  },
  message: {
    color: '#5c6760',
    fontSize: 13,
    lineHeight: 19,
  },
  action: {
    alignSelf: 'flex-start',
    borderRadius: 8,
    backgroundColor: '#111111',
    paddingHorizontal: 12,
    paddingVertical: 9,
  },
  actionLabel: {
    color: '#ffffff',
    fontSize: 13,
    fontWeight: '700',
  },
});

