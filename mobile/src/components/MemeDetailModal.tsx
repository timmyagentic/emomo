import { useCallback, useRef } from 'react';
import { Image, Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import type { GestureResponderEvent } from 'react-native';
import type { DisplayMeme } from '@/types';
import { ActionButton } from './ActionButton';

const EDGE_SWIPE_WIDTH = 28;
const EDGE_SWIPE_START_DISTANCE = 18;
const EDGE_SWIPE_CLOSE_DISTANCE = 72;
const EDGE_SWIPE_MAX_VERTICAL_DRIFT = 56;

interface SwipePoint {
  x: number;
  y: number;
}

interface MemeDetailModalProps {
  meme: DisplayMeme | null;
  onClose: () => void;
  onShare: (meme: DisplayMeme) => void;
  onSave: (meme: DisplayMeme) => void;
  onCopyImage: (meme: DisplayMeme) => void;
}

function pointFromEvent(event: GestureResponderEvent): SwipePoint {
  return {
    x: event.nativeEvent.pageX ?? event.nativeEvent.locationX,
    y: event.nativeEvent.pageY ?? event.nativeEvent.locationY,
  };
}

function isHorizontalEdgeSwipe(start: SwipePoint, point: SwipePoint, minDistance: number): boolean {
  return (
    start.x <= EDGE_SWIPE_WIDTH &&
    point.x - start.x >= minDistance &&
    Math.abs(point.y - start.y) <= EDGE_SWIPE_MAX_VERTICAL_DRIFT
  );
}

export function MemeDetailModal({ meme, onClose, onShare, onSave, onCopyImage }: MemeDetailModalProps) {
  const edgeSwipeStart = useRef<SwipePoint | null>(null);

  const handleStartShouldSetResponderCapture = useCallback((event: GestureResponderEvent) => {
    const point = pointFromEvent(event);
    edgeSwipeStart.current = point.x <= EDGE_SWIPE_WIDTH ? point : null;
    return false;
  }, []);

  const handleMoveShouldSetResponder = useCallback((event: GestureResponderEvent) => {
    const start = edgeSwipeStart.current;
    return start ? isHorizontalEdgeSwipe(start, pointFromEvent(event), EDGE_SWIPE_START_DISTANCE) : false;
  }, []);

  const handleResponderRelease = useCallback(
    (event: GestureResponderEvent) => {
      const start = edgeSwipeStart.current;
      edgeSwipeStart.current = null;

      if (start && isHorizontalEdgeSwipe(start, pointFromEvent(event), EDGE_SWIPE_CLOSE_DISTANCE)) {
        onClose();
      }
    },
    [onClose]
  );

  const handleResponderTerminate = useCallback(() => {
    edgeSwipeStart.current = null;
  }, []);

  return (
    <Modal animationType="slide" visible={Boolean(meme)} onRequestClose={onClose}>
      <View
        testID="meme-detail-gesture-surface"
        style={styles.container}
        onStartShouldSetResponderCapture={handleStartShouldSetResponderCapture}
        onMoveShouldSetResponder={handleMoveShouldSetResponder}
        onResponderRelease={handleResponderRelease}
        onResponderTerminate={handleResponderTerminate}
      >
        <View style={styles.header}>
          <Text style={styles.title}>表情详情</Text>
          <Pressable accessibilityRole="button" onPress={onClose} style={styles.closeButton}>
            <Text style={styles.closeLabel}>关闭</Text>
          </Pressable>
        </View>
        {meme ? (
          <ScrollView contentContainerStyle={styles.content}>
            <Image source={{ uri: meme.url }} resizeMode="contain" style={styles.image} />
            <View style={styles.actions}>
              <ActionButton label="分享" onPress={() => onShare(meme)} variant="primary" />
              <ActionButton label="保存" onPress={() => onSave(meme)} />
              <ActionButton label="复制图片" onPress={() => onCopyImage(meme)} />
            </View>
            <View style={styles.meta}>
              <Text style={styles.description}>{meme.description || '暂无描述'}</Text>
              {meme.category ? <Text style={styles.metaLine}>分类：{meme.category}</Text> : null}
              {meme.tags.length > 0 ? <Text style={styles.metaLine}>标签：{meme.tags.slice(0, 8).join(' / ')}</Text> : null}
            </View>
          </ScrollView>
        ) : null}
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: '#f7f8f6',
  },
  header: {
    alignItems: 'center',
    borderBottomColor: '#e0e4e1',
    borderBottomWidth: 1,
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingHorizontal: 18,
    paddingTop: 58,
    paddingBottom: 14,
  },
  title: {
    color: '#111111',
    fontSize: 18,
    fontWeight: '900',
  },
  closeButton: {
    borderRadius: 8,
    backgroundColor: '#ffffff',
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  closeLabel: {
    color: '#111111',
    fontSize: 13,
    fontWeight: '800',
  },
  content: {
    gap: 16,
    padding: 18,
  },
  image: {
    width: '100%',
    minHeight: 360,
    borderRadius: 8,
    backgroundColor: '#e8ece9',
  },
  actions: {
    flexDirection: 'row',
    gap: 8,
  },
  meta: {
    borderRadius: 8,
    backgroundColor: '#ffffff',
    borderColor: '#e0e4e1',
    borderWidth: 1,
    gap: 10,
    padding: 14,
  },
  description: {
    color: '#111111',
    fontSize: 15,
    fontWeight: '800',
    lineHeight: 22,
  },
  metaLine: {
    color: '#59635d',
    fontSize: 13,
    lineHeight: 19,
  },
});
