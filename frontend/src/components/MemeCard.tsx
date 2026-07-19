import { type CSSProperties, useState } from 'react';
import { motion, useReducedMotion } from 'framer-motion';
import { CircleAlert, Copy, Download } from 'lucide-react';
import type { DisplayMeme } from '../types';
import styles from './MemeCard.module.css';

/**
 * Props for the MemeCard component.
 */
interface MemeCardProps {
  /** The meme data to display. */
  meme: DisplayMeme;
  /**
   * The index of the card in the list, used for staggered animation delays.
   * @default 0
   */
  index?: number;
  /**
   * Callback function triggered when the card is clicked.
   * @param meme - The meme data associated with the card.
   */
  onClick?: (meme: DisplayMeme) => void;
  /** Presentation treatment for browsing spotlights versus search results. */
  variant?: 'spotlight' | 'result';
}

/**
 * A component that displays a single meme card with an image, hover effects, and quick actions.
 *
 * @param props - The component props.
 * @param props.meme - The meme object containing details like URL, description, etc.
 * @param props.index - The index for animation timing.
 * @param props.onClick - The click handler for the card.
 * @returns The rendered MemeCard component.
 */
export default function MemeCard({
  meme,
  index = 0,
  onClick,
  variant = 'result',
}: MemeCardProps) {
  const [isLoaded, setIsLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);
  const shouldReduceMotion = useReducedMotion();
  const animationDelay = shouldReduceMotion ? 0 : Math.min(index, 4) * 0.025;
  const description = meme.description || '';
  const detailLabel = description
    ? `查看表情详情：${description.slice(0, 80)}`
    : '查看表情详情';
  const hasKnownSize = typeof meme.width === 'number' && meme.width > 0
    && typeof meme.height === 'number' && meme.height > 0;
  const imageStyle = {
    '--meme-aspect-ratio': hasKnownSize ? `${meme.width} / ${meme.height}` : '1 / 1',
  } as CSSProperties;

  const handleClick = () => {
    onClick?.(meme);
  };

  const handleImageError = () => {
    setImageError(true);
    setIsLoaded(true); // Stop showing skeleton
  };

  return (
    <motion.article
      className={`${styles.card} ${variant === 'spotlight' ? styles.spotlight : styles.result}`}
      initial={{
        opacity: 0,
        transform: shouldReduceMotion ? 'none' : 'translateY(10px) scale(0.99)',
      }}
      animate={{ opacity: 1, transform: 'translateY(0) scale(1)' }}
      exit={{
        opacity: 0,
        transform: shouldReduceMotion ? 'none' : 'translateY(0) scale(0.98)',
      }}
      transition={{
        duration: shouldReduceMotion ? 0.12 : 0.22,
        delay: animationDelay,
        ease: [0.23, 1, 0.32, 1],
      }}
    >
      {/* Image container */}
      <div className={styles.imageWrapper} style={imageStyle}>
        <button
          type="button"
          className={styles.openButton}
          onClick={handleClick}
          aria-label={detailLabel}
        >
          {/* Loading skeleton */}
          {!isLoaded && !imageError && <div className={`${styles.skeleton} skeleton`} />}

          {/* Error placeholder */}
          {imageError && (
            <div className={styles.imageError}>
              <CircleAlert />
            </div>
          )}

          {/* Meme image */}
          {!imageError && (
            <motion.img
              layoutId={shouldReduceMotion ? undefined : `meme-image-${meme.id}`}
              src={meme.url}
              alt={description || 'Meme'}
              className={`${styles.image} ${isLoaded ? styles.imageLoaded : ''}`}
              loading="lazy"
              onLoad={() => setIsLoaded(true)}
              onError={handleImageError}
            />
          )}
        </button>

        {/* Hover overlay */}
        <div className={styles.overlay}>
          <div className={styles.overlayContent}>
            {/* Quick actions */}
            <div className={styles.actions}>
              <button
                type="button"
                className={styles.actionBtn}
                onClick={(e) => {
                  e.stopPropagation();
                  // Copy image URL
                  if (meme.url) {
                    navigator.clipboard.writeText(meme.url);
                  }
                }}
                aria-label="复制表情链接"
                title="复制链接"
              >
                <Copy />
              </button>
              <button
                type="button"
                className={styles.actionBtn}
                onClick={(e) => {
                  e.stopPropagation();
                  // Download image
                  if (meme.url) {
                    const a = document.createElement('a');
                    a.href = meme.url;
                    a.download = `meme-${meme.id}.${meme.format || 'jpg'}`;
                    a.click();
                  }
                }}
                aria-label="下载表情"
                title="下载"
              >
                <Download />
              </button>
            </div>
          </div>
        </div>
      </div>
    </motion.article>
  );
}
