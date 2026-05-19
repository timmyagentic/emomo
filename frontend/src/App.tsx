import { useState, useCallback, useEffect, useRef } from 'react';
import { AnimatePresence } from 'framer-motion';
import { Header, SearchHero, MemeGrid, MemeModal } from './components';
import SearchProgress from './components/SearchProgress';
import {
  searchMemesStream,
  getMemes,
  getStats,
  type SearchStageSlug,
  type SearchProgressView,
} from './api';
import { curatedMemes } from './data/curatedMemes';
import type { DisplayMeme } from './types';
import { logError } from './utils/logger';
import './App.css';

// Search state for streaming progress
interface SearchState {
  isStreaming: boolean;
  stage: SearchStageSlug;
  message: string;
  thinkingText: string;
  expandedQuery?: string;
}

const FEED_PAGE_SIZE = 12;
const SEARCH_TOP_K = 100;
const SEARCH_PAGE_SIZE = 30;

function App() {
  const [memes, setMemes] = useState<DisplayMeme[]>([]);
  const [feedMemes, setFeedMemes] = useState<DisplayMeme[]>(curatedMemes.slice(0, FEED_PAGE_SIZE));
  const [feedTotal, setFeedTotal] = useState<number | null>(null);
  const [hasFeedMore, setHasFeedMore] = useState(true);
  const [isFeedLoading, setIsFeedLoading] = useState(true);
  const [isFeedLoadingMore, setIsFeedLoadingMore] = useState(false);
  const [feedError, setFeedError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [inputQuery, setInputQuery] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [memeCount, setMemeCount] = useState(5791);
  const [selectedMeme, setSelectedMeme] = useState<DisplayMeme | null>(null);
  const [hasSearched, setHasSearched] = useState(false);
  const [searchVisibleCount, setSearchVisibleCount] = useState(SEARCH_PAGE_SIZE);
  const [searchState, setSearchState] = useState<SearchState | null>(null);
  const hasFetchedRef = useRef(false);
  const abortControllerRef = useRef<AbortController | null>(null);
  const feedAbortControllerRef = useRef<AbortController | null>(null);
  const isFeedRequestInFlightRef = useRef(false);
  const feedOffsetRef = useRef(0);

  const loadFeedPage = useCallback(async (offset: number) => {
    if (isFeedRequestInFlightRef.current) return;

    const abortController = new AbortController();
    feedAbortControllerRef.current = abortController;
    isFeedRequestInFlightRef.current = true;
    setFeedError('');

    if (offset === 0) {
      setIsFeedLoading(true);
    } else {
      setIsFeedLoadingMore(true);
    }

    try {
      const response = await getMemes(FEED_PAGE_SIZE, offset, undefined, abortController.signal);

      if (abortController.signal.aborted) {
        return;
      }

      setFeedMemes((currentMemes) => {
        if (offset === 0) {
          return response.results.length > 0 ? response.results : currentMemes;
        }

        const existingIds = new Set(currentMemes.map((meme) => meme.id));
        const newMemes = response.results.filter((meme) => !existingIds.has(meme.id));
        return [...currentMemes, ...newMemes];
      });
      setHasFeedMore(response.results.length === FEED_PAGE_SIZE);
      feedOffsetRef.current = offset + response.results.length;
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        return;
      }

      logError('Failed to load browse memes', { error });
      setFeedError(offset === 0 ? '随便逛逛加载失败，正在显示本地兜底内容。' : '加载失败，可以点按钮重试。');
    } finally {
      if (feedAbortControllerRef.current === abortController) {
        feedAbortControllerRef.current = null;
      }
      isFeedRequestInFlightRef.current = false;
      setIsFeedLoading(false);
      setIsFeedLoadingMore(false);
    }
  }, []);

  // 加载随便逛逛首屏
  useEffect(() => {
    if (hasFetchedRef.current) return;
    hasFetchedRef.current = true;

    const loadStats = async () => {
      try {
        const stats = await getStats();
        if (stats.totalMemes > 0) {
          setMemeCount(stats.totalMemes);
          setFeedTotal(stats.totalMemes);
        }
      } catch (error) {
        logError('Failed to load stats', { error });
      }
    };

    loadFeedPage(0);
    loadStats();
  }, [loadFeedPage]);

  // Handle cancel search
  const handleCancelSearch = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setSearchState(null);
    setIsLoading(false);
  }, []);

  // Handle search with streaming progress
  const handleSearch = useCallback(async (query: string) => {
    // Cancel any existing search
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    setInputQuery(query);
    setSearchQuery(query);
    setIsLoading(true);
    setHasSearched(true);
    setMemes([]); // Clear previous results
    setSearchVisibleCount(SEARCH_PAGE_SIZE);

    // Initialize search state
    setSearchState({
      isStreaming: true,
      stage: 'query_expansion_start',
      message: 'AI 正在理解搜索意图...',
      thinkingText: '',
    });

    // Accumulate thinking text
    let accumulatedThinking = '';

    try {
      await searchMemesStream(
        query,
        SEARCH_TOP_K,
        (event: SearchProgressView) => {
          if (abortController.signal.aborted) {
            return;
          }

          // Update search state based on event
          if (event.stage === 'thinking') {
            // Accumulate thinking text for typewriter effect
            if (event.isDelta && event.thinkingText) {
              accumulatedThinking += event.thinkingText;
              setSearchState((prev) =>
                prev
                  ? {
                      ...prev,
                      stage: 'thinking',
                      thinkingText: accumulatedThinking,
                    }
                  : null
              );
            }
          } else if (event.stage === 'complete') {
            // Search complete - update results
            if (event.results) {
              setMemes(event.results);
            }
            setSearchState(null);
            setIsLoading(false);
          } else if (event.stage === 'error') {
            logError('Search error', { error: event.error });
            setSearchState(null);
            setIsLoading(false);
            // Use curated data as fallback
            const filtered = curatedMemes.filter(
              (m) =>
                m.description?.toLowerCase().includes(query.toLowerCase()) ||
                m.tags?.some((t) => t.toLowerCase().includes(query.toLowerCase())) ||
                m.category?.toLowerCase().includes(query.toLowerCase())
            );
            setMemes(filtered.length > 0 ? filtered : curatedMemes);
          } else {
            // Progress update
            setSearchState((prev) =>
              prev
                ? {
                    ...prev,
                    stage: event.stage,
                    message: event.message || prev.message,
                    expandedQuery: event.expandedQuery || prev.expandedQuery,
                  }
                : null
            );
          }
        },
        abortController.signal
      );
    } catch (error) {
      if ((error as Error).name === 'AbortError') {
        // Search was cancelled - do nothing
        return;
      }
      logError('Search failed', { error });
      setSearchState(null);
      // Use curated data as fallback
      const filtered = curatedMemes.filter(
        (m) =>
          m.description?.toLowerCase().includes(query.toLowerCase()) ||
          m.tags?.some((t) => t.toLowerCase().includes(query.toLowerCase())) ||
          m.category?.toLowerCase().includes(query.toLowerCase())
      );
      setMemes(filtered.length > 0 ? filtered : curatedMemes);
    } finally {
      if (abortControllerRef.current === abortController) {
        abortControllerRef.current = null;
        setIsLoading(false);
      }
    }
  }, []);

  const resetToBrowse = useCallback(() => {
    // Cancel any ongoing search
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setMemes([]);
    setInputQuery('');
    setSearchQuery('');
    setHasSearched(false);
    setSearchVisibleCount(SEARCH_PAGE_SIZE);
    setSelectedMeme(null);
    setSearchState(null);
    setIsLoading(false);
  }, []);

  const handleInputQueryChange = useCallback((value: string) => {
    if (value) {
      setInputQuery(value);
      return;
    }

    resetToBrowse();
  }, [resetToBrowse]);

  const handleLoadMoreFeed = useCallback(() => {
    if (hasSearched) return;

    if (!hasFeedMore || (feedTotal !== null && feedOffsetRef.current >= feedTotal)) {
      return;
    }

    loadFeedPage(feedOffsetRef.current);
  }, [feedTotal, hasFeedMore, hasSearched, loadFeedPage]);

  // 搜索结果一次召回后，用本地 slice 渐进展示，模拟无限滚动。
  const handleLoadMoreSearch = useCallback(() => {
    setSearchVisibleCount((current) => Math.min(current + SEARCH_PAGE_SIZE, memes.length));
  }, [memes.length]);

  const visibleSearchMemes = memes.slice(0, searchVisibleCount);
  const hasMoreSearchResults = searchVisibleCount < memes.length;

  // Handle meme click
  const handleMemeClick = useCallback((meme: DisplayMeme) => {
    setSelectedMeme(meme);
  }, []);

  // Handle modal close
  const handleModalClose = useCallback(() => {
    setSelectedMeme(null);
  }, []);

  return (
    <div className="app">
      <Header memeCount={memeCount} onLogoClick={resetToBrowse} />

      <main className="main">
        <SearchHero
          value={inputQuery}
          onValueChange={handleInputQueryChange}
          onSearch={handleSearch}
          isLoading={isLoading}
          compact={hasSearched}
        />

        {/* Search Progress */}
        <AnimatePresence>
          {searchState?.isStreaming && (
            <SearchProgress
              stage={searchState.stage}
              message={searchState.message}
              thinkingText={searchState.thinkingText}
              expandedQuery={searchState.expandedQuery}
              onCancel={handleCancelSearch}
            />
          )}
        </AnimatePresence>

        {hasSearched ? (
          <MemeGrid
            memes={visibleSearchMemes}
            isLoading={isLoading || !!searchState?.isStreaming}
            onMemeClick={handleMemeClick}
            searchQuery={searchQuery}
            emptyMessage="没有找到相关表情包"
            hasMore={hasMoreSearchResults}
            onLoadMore={handleLoadMoreSearch}
            endMessage="已展示全部相关结果"
          />
        ) : (
          <MemeGrid
            memes={feedMemes}
            isLoading={isFeedLoading}
            onMemeClick={handleMemeClick}
            searchQuery=""
            emptyMessage=""
            title="随便逛逛"
            total={feedTotal}
            hasMore={hasFeedMore && (feedTotal === null || feedMemes.length < feedTotal)}
            isLoadingMore={isFeedLoadingMore}
            loadMoreError={feedError}
            onLoadMore={handleLoadMoreFeed}
            endMessage="已经刷完全部表情"
          />
        )}
      </main>

      <MemeModal
        meme={selectedMeme}
        isOpen={!!selectedMeme}
        onClose={handleModalClose}
      />
    </div>
  );
}

export default App;
