import { StatusBar } from 'expo-status-bar';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Keyboard,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { getMemes, getStats, searchMemesStream } from './src/api';
import {
  AboutModal,
  InlineState,
  MemeDetailModal,
  MemeMasonryList,
  RecentSearchChips,
  SearchComposer,
  SearchProgressPanel,
} from './src/components';
import {
  addSearchHistory,
  clearSearchHistory,
  getSearchHistory,
  normalizeSearchQuery,
  type SearchHistoryEntry,
} from './src/storage/searchHistory';
import type { DisplayMeme, SearchProgressView } from './src/types';
import { copyMemeImage, saveMemeToLibrary, shareMeme } from './src/utils/imageActions';

const INITIAL_PAGE_SIZE = 20;
const SEARCH_TOP_K = 50;

export default function App() {
  const [inputQuery, setInputQuery] = useState('');
  const [lastQuery, setLastQuery] = useState('');
  const [memeCount, setMemeCount] = useState(0);
  const [history, setHistory] = useState<SearchHistoryEntry[]>([]);
  const [feedMemes, setFeedMemes] = useState<DisplayMeme[]>([]);
  const [results, setResults] = useState<DisplayMeme[]>([]);
  const [selectedMeme, setSelectedMeme] = useState<DisplayMeme | null>(null);
  const [hasSearched, setHasSearched] = useState(false);
  const [isSearching, setIsSearching] = useState(false);
  const [isInitialLoading, setIsInitialLoading] = useState(true);
  const [error, setError] = useState('');
  const [progress, setProgress] = useState<SearchProgressView | null>(null);
  const [thinkingText, setThinkingText] = useState('');
  const [isAboutVisible, setIsAboutVisible] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    const abortController = new AbortController();

    async function loadInitialData() {
      setIsInitialLoading(true);
      try {
        const [stats, memes, storedHistory] = await Promise.all([
          getStats(abortController.signal).catch(() => null),
          getMemes(INITIAL_PAGE_SIZE, 0, undefined, abortController.signal).catch(() => ({ results: [], total: 0 })),
          getSearchHistory(),
        ]);

        if (stats?.totalMemes) {
          setMemeCount(stats.totalMemes);
        }
        setFeedMemes(memes.results);
        setHistory(storedHistory);
      } finally {
        setIsInitialLoading(false);
      }
    }

    loadInitialData();
    return () => {
      abortController.abort();
      abortRef.current?.abort();
    };
  }, []);

  const visibleMemes = hasSearched ? results : feedMemes;
  const emptyTitle = hasSearched ? '没有找到合适的表情' : isInitialLoading ? '正在加载表情库' : '还没有可展示的表情';
  const emptyMessage = hasSearched ? '换一种描述试试，比如说清楚情绪、场景和语气。' : '稍后下拉或重新打开 App 再试。';

  const subtitle = useMemo(() => {
    if (memeCount > 0) {
      return `${memeCount.toLocaleString()} 张表情可搜`;
    }
    return '自然语言搜索表情包';
  }, [memeCount]);

  const cancelSearch = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setIsSearching(false);
    setProgress(null);
  }, []);

  const runSearch = useCallback(
    async (rawQuery?: string) => {
      const query = normalizeSearchQuery(rawQuery ?? inputQuery);
      if (!query || isSearching) {
        return;
      }

      Keyboard.dismiss();
      abortRef.current?.abort();
      const abortController = new AbortController();
      abortRef.current = abortController;

      setInputQuery(query);
      setLastQuery(query);
      setHasSearched(true);
      setResults([]);
      setError('');
      setThinkingText('');
      setIsSearching(true);
      setProgress({
        stage: 'query_expansion_start',
        message: 'AI 正在理解搜索意图...',
      });

      try {
        setHistory(await addSearchHistory(query));
        let accumulatedThinking = '';
        await searchMemesStream(
          query,
          { topK: SEARCH_TOP_K },
          (event) => {
            if (abortController.signal.aborted) {
              return;
            }
            if (event.stage === 'thinking' && event.thinkingText) {
              accumulatedThinking += event.thinkingText;
              setThinkingText(accumulatedThinking);
              setProgress({ ...event, thinkingText: accumulatedThinking });
              return;
            }
            if (event.stage === 'complete') {
              setResults(event.results ?? []);
              setProgress(null);
              return;
            }
            if (event.stage === 'error') {
              setError(event.error ?? '搜索失败，请稍后再试。');
              setProgress(null);
              return;
            }
            setProgress(event);
          },
          abortController.signal
        );
      } catch (searchError) {
        if ((searchError as Error).name !== 'AbortError') {
          setError('搜索失败，请检查网络后重试。');
          setProgress(null);
        }
      } finally {
        if (abortRef.current === abortController) {
          abortRef.current = null;
          setIsSearching(false);
        }
      }
    },
    [inputQuery, isSearching]
  );

  const clearHistory = useCallback(async () => {
    await clearSearchHistory();
    setHistory([]);
  }, []);

  const retrySearch = useCallback(() => {
    if (lastQuery) {
      runSearch(lastQuery);
    }
  }, [lastQuery, runSearch]);

  const handleShare = useCallback(async (meme: DisplayMeme) => {
    try {
      await shareMeme(meme);
    } catch {
      Alert.alert('分享失败', '暂时无法打开系统分享面板。');
    }
  }, []);

  const handleSave = useCallback(async (meme: DisplayMeme) => {
    try {
      await saveMemeToLibrary(meme);
      Alert.alert('已保存', '表情包已保存到相册。');
    } catch {
      Alert.alert('保存失败', '请确认相册权限后再试。');
    }
  }, []);

  const handleCopyImage = useCallback(async (meme: DisplayMeme) => {
    try {
      await copyMemeImage(meme);
      Alert.alert('已复制', '表情包图片已复制，可以直接粘贴到聊天应用。');
    } catch {
      Alert.alert('复制失败', '当前系统暂时无法复制这张图片，可以先使用分享或保存。');
    }
  }, []);

  return (
    <SafeAreaView style={styles.safeArea}>
      <StatusBar style="dark" />
      <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={styles.content}>
        <View style={styles.header}>
          <View>
            <Text style={styles.brand}>emomo</Text>
            <Text style={styles.subtitle}>{subtitle}</Text>
          </View>
          <View style={styles.headerActions}>
            <Text style={styles.modeLabel}>AI Search</Text>
            <Pressable
              accessibilityLabel="打开关于与隐私信息"
              accessibilityRole="button"
              onPress={() => setIsAboutVisible(true)}
              style={styles.aboutButton}
            >
              <Text style={styles.aboutLabel}>i</Text>
            </Pressable>
          </View>
        </View>

        <View style={styles.hero}>
          <Text style={styles.title}>今天想找什么表情？</Text>
          <Text style={styles.copy}>描述一个情绪、场景或想发的话，emomo 会把语义和画面一起找出来。</Text>
          <SearchComposer
            value={inputQuery}
            onChangeText={setInputQuery}
            onSubmit={() => runSearch()}
            onCancel={cancelSearch}
            isLoading={isSearching}
          />
        </View>

        <RecentSearchChips items={history} onPick={(query) => runSearch(query)} onClear={clearHistory} />

        <SearchProgressPanel progress={progress} thinkingText={thinkingText} />

        {error ? <InlineState title="搜索出错" message={error} actionLabel="重试" onAction={retrySearch} /> : null}

        <View style={styles.sectionHeader}>
          <Text style={styles.sectionTitle}>{hasSearched ? `“${lastQuery}” 的结果` : '随便逛逛'}</Text>
          <Text style={styles.sectionMeta}>{visibleMemes.length > 0 ? `${visibleMemes.length} 张` : ''}</Text>
        </View>

        <MemeMasonryList
          data={visibleMemes}
          isLoading={isInitialLoading || isSearching}
          emptyTitle={emptyTitle}
          emptyMessage={emptyMessage}
          onPick={setSelectedMeme}
        />
      </ScrollView>

      <MemeDetailModal
        meme={selectedMeme}
        onClose={() => setSelectedMeme(null)}
        onShare={handleShare}
        onSave={handleSave}
        onCopyImage={handleCopyImage}
      />
      <AboutModal visible={isAboutVisible} onClose={() => setIsAboutVisible(false)} onClearHistory={clearHistory} />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: '#f7f8f6',
  },
  content: {
    gap: 16,
    paddingHorizontal: 18,
    paddingTop: 18,
    paddingBottom: 28,
  },
  header: {
    alignItems: 'center',
    flexDirection: 'row',
    justifyContent: 'space-between',
  },
  headerActions: {
    alignItems: 'center',
    flexDirection: 'row',
    gap: 8,
  },
  brand: {
    color: '#111111',
    fontSize: 24,
    fontWeight: '900',
  },
  subtitle: {
    color: '#58635d',
    fontSize: 12,
    fontWeight: '700',
    marginTop: 2,
  },
  modeLabel: {
    borderRadius: 8,
    backgroundColor: '#dff4e9',
    color: '#174d3d',
    fontSize: 12,
    fontWeight: '900',
    overflow: 'hidden',
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  aboutButton: {
    alignItems: 'center',
    justifyContent: 'center',
    width: 32,
    height: 32,
    borderRadius: 16,
    backgroundColor: '#111111',
  },
  aboutLabel: {
    color: '#ffffff',
    fontSize: 16,
    fontWeight: '900',
  },
  hero: {
    gap: 12,
  },
  title: {
    color: '#111111',
    fontSize: 32,
    fontWeight: '900',
    lineHeight: 38,
  },
  copy: {
    color: '#4f5a54',
    fontSize: 15,
    lineHeight: 22,
  },
  sectionHeader: {
    alignItems: 'center',
    flexDirection: 'row',
    justifyContent: 'space-between',
  },
  sectionTitle: {
    color: '#111111',
    flex: 1,
    fontSize: 18,
    fontWeight: '900',
  },
  sectionMeta: {
    color: '#68736c',
    fontSize: 12,
    fontWeight: '800',
  },
});
