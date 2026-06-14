import { useState, useRef, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import styles from './SearchHero.module.css';

/**
 * Props for the SearchHero component.
 */
interface SearchHeroProps {
  /** Current search box value. */
  value: string;
  /** Callback triggered when the search box value changes. */
  onValueChange: (value: string) => void;
  /**
   * Callback function triggered when a search is performed.
   * @param query - The search query string.
   */
  onSearch: (query: string) => void;
  /**
   * Indicates whether a search is currently in progress.
   * Controls loading states in the search bar.
   * @default false
   */
  isLoading?: boolean;
  /**
   * Uses a denser layout after a search has been performed.
   * @default false
   */
  compact?: boolean;
  /**
   * A list of suggested tags to display below the search bar.
   * @default ['开心', '无语', '狗头', '猫咪', '熊猫头', '沙雕']
   */
  suggestedTags?: string[];
}

const placeholders = [
  '描述你想要的表情包...',
  '比如：一只超级开心的柴犬',
  '试试：无语、摊手、叹气',
  '或者：熊猫头说谢谢',
  '还有：猫咪翻白眼',
];

/**
 * The hero section component featuring the main search bar and suggested tags.
 * Includes animations for visual appeal and placeholder rotation.
 *
 * @param props - The component props.
 * @param props.onSearch - Handler for search submissions.
 * @param props.isLoading - Whether a search is loading.
 * @param props.suggestedTags - List of tags to suggest.
 * @returns The rendered SearchHero component.
 */
export default function SearchHero({
  value,
  onValueChange,
  onSearch,
  isLoading = false,
  compact = false,
  suggestedTags = ['开心', '无语', '狗头', '猫咪', '熊猫头', '沙雕']
}: SearchHeroProps) {
  const [isFocused, setIsFocused] = useState(false);
  const [placeholderIndex, setPlaceholderIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // Rotate placeholders
  useEffect(() => {
    if (!isFocused && !value) {
      const timer = setInterval(() => {
        setPlaceholderIndex((prev) => (prev + 1) % placeholders.length);
      }, 3000);
      return () => clearInterval(timer);
    }
    // Explicitly return undefined when condition is false
    return undefined;
  }, [isFocused, value]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (value.trim()) {
      onSearch(value.trim());
    }
  };

  const handleTagClick = (tag: string) => {
    onValueChange(tag);
    onSearch(tag); // 自动触发搜索
  };

  return (
    <section className={`${styles.hero} ${compact ? styles.compact : ''}`}>
      {/* Decorative elements */}
      <div className={styles.decorations}>
        <motion.div
          className={styles.blob1}
          animate={{
            x: [0, 30, 0],
            y: [0, -20, 0],
            scale: [1, 1.1, 1],
          }}
          transition={{ duration: 8, repeat: Infinity, ease: 'easeInOut' }}
        />
        <motion.div
          className={styles.blob2}
          animate={{
            x: [0, -20, 0],
            y: [0, 30, 0],
            scale: [1, 0.9, 1],
          }}
          transition={{ duration: 10, repeat: Infinity, ease: 'easeInOut' }}
        />
        <motion.div
          className={styles.blob3}
          animate={{
            x: [0, 15, 0],
            y: [0, 15, 0],
          }}
          transition={{ duration: 6, repeat: Infinity, ease: 'easeInOut' }}
        />
      </div>

      {/* Main content */}
      <motion.div
        className={styles.content}
        initial={{ opacity: 0, y: 30 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.8, ease: [0.16, 1, 0.3, 1] }}
      >
        {/* Title */}
        <motion.h1
          className={styles.title}
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2, duration: 0.6 }}
        >
          <span className={styles.titleEmoji}>
            <motion.span
              animate={{ rotate: [0, 10, -10, 0] }}
              transition={{ duration: 2, repeat: Infinity, repeatDelay: 3 }}
            >
              ✨
            </motion.span>
          </span>
          用文字找表情
        </motion.h1>

        <motion.p
          className={styles.subtitle}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.4, duration: 0.6 }}
        >
          AI 驱动的语义搜索，让表情包触手可及
        </motion.p>

        {/* Search Form */}
        <motion.form
          className={styles.searchForm}
          onSubmit={handleSubmit}
          initial={{ opacity: 0, scale: 0.95 }}
          animate={{ opacity: 1, scale: 1 }}
          transition={{ delay: 0.5, duration: 0.5 }}
        >
          <div
            className={`${styles.searchBox} ${isFocused ? styles.focused : ''} ${isLoading ? styles.loading : ''}`}
          >
            {/* Animated border */}
            <div className={styles.borderGlow} />

            {/* Search icon */}
            <div className={styles.searchIcon}>
              {isLoading ? (
                <motion.div
                  className={styles.spinner}
                  animate={{ rotate: 360 }}
                  transition={{ duration: 1, repeat: Infinity, ease: 'linear' }}
                />
              ) : (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                  <circle cx="11" cy="11" r="8" />
                  <path d="M21 21l-4.35-4.35" />
                </svg>
              )}
            </div>

            {/* Input */}
            <input
              ref={inputRef}
              type="text"
              value={value}
              onChange={(e) => onValueChange(e.target.value)}
              onFocus={() => setIsFocused(true)}
              onBlur={() => setIsFocused(false)}
              placeholder={placeholders[placeholderIndex]}
              className={styles.input}
              disabled={isLoading}
              aria-label="搜索表情包"
            />

            {/* Clear button */}
            <AnimatePresence>
              {value && (
                <motion.button
                  type="button"
                  className={styles.clearBtn}
                  onClick={() => onValueChange('')}
                  aria-label="清空搜索"
                  initial={{ opacity: 0, scale: 0.8 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.8 }}
                  whileHover={{ scale: 1.1 }}
                  whileTap={{ scale: 0.9 }}
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M18 6L6 18M6 6l12 12" />
                  </svg>
                </motion.button>
              )}
            </AnimatePresence>

            {/* Submit button */}
            <motion.button
              type="submit"
              className={styles.submitBtn}
              disabled={!value.trim() || isLoading}
              whileHover={{ scale: 1.02 }}
              whileTap={{ scale: 0.98 }}
            >
              搜索
            </motion.button>
          </div>
        </motion.form>

        {/* Suggested tags */}
        <motion.div
          className={styles.tags}
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.7, duration: 0.5 }}
        >
          <span className={styles.tagsLabel}>热门:</span>
          <div className={styles.tagsList}>
            {suggestedTags.map((tag, index) => (
              <motion.button
                key={tag}
                type="button"
                className={`${styles.tag} ${value === tag ? styles.tagActive : ''}`}
                onClick={() => handleTagClick(tag)}
                aria-pressed={value === tag}
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.8 + index * 0.05 }}
                whileHover={{ scale: 1.05, y: -2 }}
                whileTap={{ scale: 0.95 }}
              >
                {tag}
              </motion.button>
            ))}
          </div>
        </motion.div>
      </motion.div>
    </section>
  );
}
