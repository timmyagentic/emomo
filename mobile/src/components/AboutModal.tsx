import { Linking, Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import {
  APP_BUILD_NUMBER,
  APP_PRIVACY_POLICY_URL,
  APP_SUPPORT_URL,
  APP_VERSION,
} from '@/config/appInfo';
import { ActionButton } from './ActionButton';

interface AboutModalProps {
  visible: boolean;
  onClose: () => void;
  onClearHistory: () => void;
}

function openURL(url: string) {
  Linking.openURL(url).catch(() => undefined);
}

export function AboutModal({ visible, onClose, onClearHistory }: AboutModalProps) {
  return (
    <Modal animationType="slide" visible={visible} onRequestClose={onClose}>
      <View style={styles.container}>
        <View style={styles.header}>
          <View>
            <Text style={styles.title}>关于 emomo</Text>
            <Text style={styles.version}>版本 {APP_VERSION} ({APP_BUILD_NUMBER})</Text>
          </View>
          <Pressable accessibilityRole="button" onPress={onClose} style={styles.closeButton}>
            <Text style={styles.closeLabel}>关闭</Text>
          </Pressable>
        </View>

        <ScrollView contentContainerStyle={styles.content}>
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>隐私政策</Text>
            <Text style={styles.body}>
              搜索请求会发送到 emomo 后端用于返回结果。
            </Text>
            <Text style={styles.body}>搜索历史只保存在本机，可以随时清空。</Text>
            <ActionButton label="查看隐私政策" onPress={() => openURL(APP_PRIVACY_POLICY_URL)} />
          </View>

          <View style={styles.section}>
            <Text style={styles.sectionTitle}>支持与反馈</Text>
            <Text style={styles.body}>遇到搜索、保存或分享问题，可以通过项目 Issues 反馈。</Text>
            <ActionButton label="打开支持页面" onPress={() => openURL(APP_SUPPORT_URL)} />
          </View>

          <View style={styles.section}>
            <Text style={styles.sectionTitle}>本机数据</Text>
            <Text style={styles.body}>emomo v1 没有账号系统，不同步搜索历史，也不在 App 内缓存图片文件。</Text>
            <ActionButton label="清空搜索历史" onPress={onClearHistory} />
          </View>
        </ScrollView>
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
    fontSize: 20,
    fontWeight: '900',
  },
  version: {
    color: '#59635d',
    fontSize: 12,
    fontWeight: '700',
    marginTop: 3,
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
    gap: 14,
    padding: 18,
  },
  section: {
    borderRadius: 8,
    backgroundColor: '#ffffff',
    borderColor: '#e0e4e1',
    borderWidth: 1,
    gap: 10,
    padding: 14,
  },
  sectionTitle: {
    color: '#111111',
    fontSize: 16,
    fontWeight: '900',
  },
  body: {
    color: '#59635d',
    fontSize: 13,
    lineHeight: 20,
  },
});
