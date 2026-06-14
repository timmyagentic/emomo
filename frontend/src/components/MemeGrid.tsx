import { useEffect, useRef } from 'react';
import { motion } from 'framer-motion';
import type { DisplayMeme, TextPresenceFilter } from '../types';
import MemeCard from './MemeCard';
import styles from './MemeGrid.module.css';

const countFormatter = new Intl.NumberFormat('en-US');
const textPresenceOptions: Array<{ value: TextPresenceFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'with_text', label: '有文字' },
  { value: 'without_text', label: '无文字' },
];

/**
 * Props for the MemeGrid component.
 */
interface MemeGridProps {
  /** The list of memes to display in the grid. */
  memes: DisplayMeme[];
  /**
   * Indicates whether the grid is in a loading state.
   * If true, displays loading skeletons instead of memes.
   * @default false
   */
  isLoading?: boolean;
  /**
   * Callback function triggered when a meme card is clicked.
   * @param meme - The meme data associated with the clicked card.
   */
  onMemeClick?: (meme: DisplayMeme) => void;
  /**
   * Message to display when the meme list is empty.
   * @default '暂无表情包'
   */
  emptyMessage?: string;
  /** The search query string, used to display results information. */
  searchQuery?: string;
  /** An optional title for the grid section (e.g., "Recommended"). */
  title?: string;
  /** Total number of available memes for non-search browsing. */
  total?: number | null;
  /** Whether more memes can be loaded. */
  hasMore?: boolean;
  /** Whether the next page is currently loading. */
  isLoadingMore?: boolean;
  /** Optional error message for loading the next page. */
  loadMoreError?: string;
  /** Callback triggered by the footer button or near-bottom auto loading. */
  onLoadMore?: () => void;
  /** Message shown when all items have been loaded. */
  endMessage?: string;
  /** Current result-side text-presence display filter. */
  textPresenceFilter?: TextPresenceFilter;
  /** Callback triggered when the result-side text-presence filter changes. */
  onTextPresenceFilterChange?: (filter: TextPresenceFilter) => void;
  /** Total number of search results before result-side display filtering. */
  searchResultTotal?: number;
  /** Total number of search results after result-side display filtering. */
  filteredResultTotal?: number;
}

/**
 * A loading skeleton component for a meme card.
 *
 * @param props - The component props.
 * @param props.index - The index for animation delay.
 * @returns The rendered SkeletonCard component.
 */
function SkeletonCard({ index }: { index: number }) {
  return (
    <motion.div
      className={styles.skeletonCard}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ delay: index * 0.05 }}
    >
      <div className={`${styles.skeletonImage} skeleton`} />
    </motion.div>
  );
}

/**
 * A component that displays a responsive grid of meme cards.
 * Handles loading states, empty states, and section titles.
 *
 * @param props - The component props.
 * @param props.memes - The list of memes to display.
 * @param props.isLoading - Whether the data is loading.
 * @param props.onMemeClick - Handler for meme click events.
 * @param props.emptyMessage - Custom empty state message.
 * @param props.searchQuery - The current search query.
 * @param props.title - Optional section title.
 * @returns The rendered MemeGrid component.
 */
export default function MemeGrid({
  memes,
  isLoading = false,
  onMemeClick,
  emptyMessage = '暂无表情包',
  searchQuery,
  title,
  total,
  hasMore = false,
  isLoadingMore = false,
  loadMoreError = '',
  onLoadMore,
  endMessage = '已展示全部结果',
  textPresenceFilter,
  onTextPresenceFilterChange,
  searchResultTotal,
  filteredResultTotal,
}: MemeGridProps) {
  const loadMoreRef = useRef<HTMLDivElement | null>(null);
  const lastAutoLoadCountRef = useRef(-1);
  const hasLeftLoadZoneRef = useRef(true);
  const scoredMemes = memes.filter((meme) => typeof meme.score === 'number');
  const topScore = scoredMemes.length > 0
    ? Math.max(...scoredMemes.map((meme) => meme.score ?? 0))
    : null;
  const hasLowConfidence = !!searchQuery && topScore !== null && topScore < 0.15;
  const isBrowseMode = !searchQuery && !!onLoadMore;
  const isSearchMode = !!searchQuery;
  const resultFilter = isSearchMode && textPresenceFilter && onTextPresenceFilterChange
    ? { value: textPresenceFilter, onChange: onTextPresenceFilterChange }
    : null;
  const loadedCountText = typeof total === 'number'
    ? `已展示 ${countFormatter.format(memes.length)} / ${countFormatter.format(total)} 个表情包`
    : `已展示 ${countFormatter.format(memes.length)} 个表情包`;
  const searchCountText = typeof filteredResultTotal === 'number'
    ? `显示 ${countFormatter.format(memes.length)} / ${countFormatter.format(filteredResultTotal)} 个`
    : `显示 ${countFormatter.format(memes.length)} 个`;
  const searchFilterText = typeof searchResultTotal === 'number' && typeof filteredResultTotal === 'number'
    ? `从 ${countFormatter.format(searchResultTotal)} 个结果中筛选`
    : '筛选当前结果';
  const resultsHeader = (title || searchQuery || resultFilter || hasLowConfidence) ? (
    <motion.header
      className={styles.resultsHeader}
      initial={{ opacity: 0, y: -10 }}
      animate={{ opacity: 1, y: 0 }}
    >
      {title && (
        <div className={styles.titleGroup}>
          <h2 className={styles.sectionTitle}>{title}</h2>
          {isBrowseMode && (
            <span className={styles.browseCount}>{loadedCountText}</span>
          )}
        </div>
      )}

      {searchQuery && (
        <div className={styles.searchSummary}>
          <span className={styles.searchSummaryLabel}>搜索结果</span>
          <span className={styles.resultsQuery}>「{searchQuery}」</span>
          <span className={styles.searchCount}>{searchCountText}</span>
        </div>
      )}

      {resultFilter && (
        <div className={styles.resultFilter}>
          <div className={styles.resultFilterCopy}>
            <span className={styles.resultFilterLabel}>结果筛选</span>
            <span className={styles.resultFilterMeta}>{searchFilterText}</span>
          </div>
          <div className={styles.segmentedControl} role="radiogroup" aria-label="筛选当前搜索结果">
            {textPresenceOptions.map((option) => {
              const selected = resultFilter.value === option.value;
              return (
                <button
                  key={option.value}
                  type="button"
                  role="radio"
                  aria-checked={selected}
                  className={`${styles.segmentButton} ${selected ? styles.segmentButtonActive : ''}`}
                  onClick={() => resultFilter.onChange(option.value)}
                >
                  {option.label}
                </button>
              );
            })}
          </div>
        </div>
      )}

      {hasLowConfidence && (
        <p className={styles.qualityNotice}>
          匹配度偏低，当前结果更像相近情绪或相近语境。
        </p>
      )}
    </motion.header>
  ) : null;

  useEffect(() => {
    if (!onLoadMore || !hasMore || isLoading || isLoadingMore || loadMoreError) {
      return;
    }

    const sentinel = loadMoreRef.current;
    if (!sentinel) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry.isIntersecting) {
          hasLeftLoadZoneRef.current = true;
          return;
        }

        if (hasLeftLoadZoneRef.current && lastAutoLoadCountRef.current !== memes.length) {
          lastAutoLoadCountRef.current = memes.length;
          hasLeftLoadZoneRef.current = false;
          onLoadMore();
        }
      },
      {
        rootMargin: '180px 0px',
        threshold: 0,
      }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasMore, isLoading, isLoadingMore, loadMoreError, memes.length, onLoadMore]);

  // Show loading skeletons
  if (isLoading) {
    return (
      <section className={styles.container}>
        {title && (
          <motion.h2
            className={styles.sectionTitle}
            initial={{ opacity: 0, y: -10 }}
            animate={{ opacity: 1, y: 0 }}
          >
            {title}
          </motion.h2>
        )}
        <div className={styles.grid}>
          {Array.from({ length: 12 }).map((_, i) => (
            <SkeletonCard key={i} index={i} />
          ))}
        </div>
      </section>
    );
  }

  // Empty state
  if (memes.length === 0) {
    return (
      <section className={styles.container}>
        {resultsHeader}
        <motion.div
          className={styles.empty}
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5 }}
        >
          <div className={styles.emptyIcon}>
            <motion.div
              animate={{ rotate: [0, 10, -10, 0] }}
              transition={{ duration: 2, repeat: Infinity, repeatDelay: 1 }}
            >
              🤔
            </motion.div>
          </div>
          <h3 className={styles.emptyTitle}>{emptyMessage}</h3>
          {searchQuery && (
            <p className={styles.emptyText}>
              找不到与「{searchQuery}」相关的表情包，试试其他关键词？
            </p>
          )}
        </motion.div>
      </section>
    );
  }

  return (
    <section className={styles.container}>
      {resultsHeader}

      {/* Grid */}
      <div className={styles.grid}>
        {memes.map((meme, index) => (
          <MemeCard
            key={meme.id}
            meme={meme}
            index={index}
            onClick={onMemeClick}
          />
        ))}
      </div>

      {onLoadMore && (
        <motion.div
          ref={loadMoreRef}
          className={styles.loadMore}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.2 }}
        >
          {loadMoreError && (
            <p className={styles.loadMoreError}>{loadMoreError}</p>
          )}

          {hasMore ? (
            <button
              type="button"
              className={styles.loadMoreButton}
              onClick={onLoadMore}
              disabled={isLoadingMore}
            >
              {isLoadingMore ? (
                <span className={styles.loadingInline}>
                  <span className={styles.loadingDot} aria-hidden="true" />
                  加载中...
                </span>
              ) : loadMoreError ? '重试加载' : '加载更多'}
            </button>
          ) : (
            <div className={styles.endIndicator}>
              <span className={styles.endLine} />
              <span className={styles.endText}>{endMessage}</span>
              <span className={styles.endLine} />
            </div>
          )}
        </motion.div>
      )}

      {!onLoadMore && memes.length > 0 && (
        <motion.div
          className={styles.endIndicator}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.5 }}
        >
          <span className={styles.endLine} />
          <span className={styles.endText}>{endMessage}</span>
          <span className={styles.endLine} />
        </motion.div>
      )}
    </section>
  );
}
