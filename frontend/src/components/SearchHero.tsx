import { useMemo, useRef, useState } from 'react';
import { motion, useReducedMotion } from 'framer-motion';
import { Search, Sparkles, X } from 'lucide-react';
import styles from './SearchHero.module.css';

interface SearchHeroProps {
  value: string;
  onValueChange: (value: string) => void;
  onSearch: (query: string) => void;
  isLoading?: boolean;
  compact?: boolean;
  suggestedTags?: string[];
}

const defaultTags = [
  '开心',
  '无语',
  '狗头',
  '猫咪',
  '熊猫头',
  '沙雕',
  '惊讶',
  '委屈',
  '得意',
  '加油',
];

function getDateLabel(): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date());
  const values = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${values.year}-${values.month}-${values.day}`;
}

export default function SearchHero({
  value,
  onValueChange,
  onSearch,
  isLoading = false,
  compact = false,
  suggestedTags = defaultTags,
}: SearchHeroProps) {
  const [isFocused, setIsFocused] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const shouldReduceMotion = useReducedMotion();
  const dateLabel = useMemo(() => getDateLabel(), []);

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    if (value.trim()) {
      onSearch(value.trim());
    }
  };

  const handleTagClick = (tag: string) => {
    onValueChange(tag);
    onSearch(tag);
  };

  return (
    <section className={`${styles.hero} ${compact ? styles.compact : ''}`}>
      <div className={styles.dateLine}>
        <span className={styles.dateDot} aria-hidden="true" />
        <time dateTime={dateLabel}>{dateLabel}</time>
      </div>

      <motion.div
        className={styles.content}
        initial={{
          opacity: 0,
          transform: shouldReduceMotion ? 'none' : 'translateY(14px)',
        }}
        animate={{ opacity: 1, transform: 'translateY(0)' }}
        transition={{ duration: 0.26, ease: [0.23, 1, 0.32, 1] }}
      >
        <h1 className={styles.title}>
          用文字找表情
          <Sparkles className={styles.titleSpark} aria-hidden="true" />
        </h1>

        <p className={styles.subtitle}>AI 驱动的语义搜索，让表情包触手可及</p>

        <form className={styles.searchForm} onSubmit={handleSubmit} role="search">
          <div
            className={`${styles.searchBox} ${isFocused ? styles.focused : ''} ${isLoading ? styles.loading : ''}`}
          >
            <div className={styles.searchIcon} aria-hidden="true">
              {isLoading ? (
                <span className={styles.spinner} />
              ) : (
                <Search />
              )}
            </div>

            <input
              ref={inputRef}
              type="text"
              value={value}
              onChange={(event) => onValueChange(event.target.value)}
              onFocus={() => setIsFocused(true)}
              onBlur={() => setIsFocused(false)}
              placeholder="比如：一只超级开心的柴犬"
              className={styles.input}
              disabled={isLoading}
              aria-label="搜索表情包"
            />

            {value && (
              <button
                type="button"
                className={styles.clearBtn}
                onClick={() => onValueChange('')}
                disabled={isLoading}
                aria-label="清空搜索"
              >
                <X />
              </button>
            )}

            <button
              type="submit"
              className={styles.submitBtn}
              disabled={!value.trim() || isLoading}
            >
              {isLoading ? '搜索中' : '搜索'}
            </button>
          </div>
        </form>

        <div className={styles.tags} aria-label="推荐搜索">
          <span className={styles.tagsLabel}>热门:</span>
          <div className={styles.tagsList}>
            {suggestedTags.map((tag) => (
              <button
                key={tag}
                type="button"
                className={`${styles.tag} ${value === tag ? styles.tagActive : ''}`}
                onClick={() => handleTagClick(tag)}
                aria-pressed={value === tag}
              >
                {tag}
              </button>
            ))}
          </div>
        </div>
      </motion.div>
    </section>
  );
}
