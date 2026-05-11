import { useEffect, useState, useRef, useMemo } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import type { DisplayMeme } from '../types';
import { logError } from '../utils/logger';
import styles from './MemeModal.module.css';

/**
 * Props for the MemeModal component.
 */
interface MemeModalProps {
  /** The meme to display in the modal. If null, the modal is hidden. */
  meme: DisplayMeme | null;
  /** Whether the modal is currently open. */
  isOpen: boolean;
  /** Callback function to close the modal. */
  onClose: () => void;
}

/**
 * Parses and cleans a tag string to make it user-friendly.
 * Filters out hashes, numeric tags, and redundant information.
 *
 * @param tag - The raw tag string.
 * @returns The cleaned tag string, or null if the tag should be discarded.
 */
function parseTag(tag: string): string | null {
  // 过滤掉 MD5 哈希（32位十六进制字符）
  if (/^[a-f0-9]{32}$/i.test(tag)) {
    return null;
  }

  // 过滤掉纯数字或太短的标签
  if (/^\d+$/.test(tag) || tag.length < 2) {
    return null;
  }

  // 解析格式如 "000Contribution_贡献🇨🇳BQB"
  // 尝试提取中文部分或有意义的部分
  let parsed = tag;

  // 移除开头的数字
  parsed = parsed.replace(/^\d+/, '');

  // 移除末尾的 "BQB"（表情包库标识）
  parsed = parsed.replace(/BQB$/i, '');

  // 如果有下划线，尝试提取中文部分
  if (parsed.includes('_')) {
    const parts = parsed.split('_');
    // 优先选择包含中文的部分
    const chinesePart = parts.find(p => /[\u4e00-\u9fa5]/.test(p));
    if (chinesePart) {
      parsed = chinesePart;
    } else {
      // 否则取最后一个非空部分
      parsed = parts.filter(p => p.trim()).pop() || parsed;
    }
  }

  // 移除表情符号（国旗等）但保留常用表情
  parsed = parsed.replace(/[\u{1F1E0}-\u{1F1FF}]/gu, '');

  // 清理空白
  parsed = parsed.trim();

  // 如果处理后太短或为空，返回 null
  if (parsed.length < 2) {
    return null;
  }

  return parsed;
}

/**
 * Formats a list of tags by cleaning them and removing duplicates.
 *
 * @param tags - The list of raw tags.
 * @returns An array of unique, cleaned tags.
 */
function formatTags(tags: string[] | undefined): string[] {
  if (!tags || tags.length === 0) return [];

  const formatted = tags
    .map(parseTag)
    .filter((tag): tag is string => tag !== null);

  // 去重
  return [...new Set(formatted)];
}

/**
 * A modal component that displays a meme in detail.
 * Allows downloading, copying image/link, and viewing metadata.
 *
 * @param props - The component props.
 * @param props.meme - The meme object to display.
 * @param props.isOpen - Controls the visibility of the modal.
 * @param props.onClose - Handler to close the modal.
 * @returns The rendered MemeModal component.
 */
export default function MemeModal({ meme, isOpen, onClose }: MemeModalProps) {
  const [copied, setCopied] = useState(false);
  const [downloaded, setDownloaded] = useState(false);
  const [imageErrorState, setImageErrorState] = useState<{
    memeId?: string;
    hasError: boolean;
  }>({ hasError: false });
  const timeoutRefs = useRef<{
    copied?: ReturnType<typeof setTimeout>;
    downloaded?: ReturnType<typeof setTimeout>;
  }>({});
  const modalRef = useRef<HTMLDivElement>(null);
  const activeMemeId = meme?.id;
  const imageError = imageErrorState.hasError && imageErrorState.memeId === activeMemeId;
  const description = meme?.description || '';
  const scorePercent = typeof meme?.score === 'number' && meme.score > 0
    ? Math.round(meme.score * 100)
    : null;
  const scoreTone = scorePercent === null
    ? ''
    : scorePercent < 15
      ? styles.scoreLow
      : scorePercent < 45
        ? styles.scoreMedium
        : styles.scoreHigh;

  // 格式化标签
  const displayTags = useMemo(() => formatTags(meme?.tags), [meme?.tags]);

  useEffect(() => {
    if (isOpen) {
      modalRef.current?.focus();
    }
  }, [isOpen, activeMemeId]);

  // Cleanup timeouts on unmount
  useEffect(() => {
    const timeouts = timeoutRefs.current;
    return () => {
      if (timeouts.copied) {
        clearTimeout(timeouts.copied);
      }
      if (timeouts.downloaded) {
        clearTimeout(timeouts.downloaded);
      }
    };
  }, []);

  // Close on escape key
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };

    if (isOpen) {
      document.addEventListener('keydown', handleEscape);
      document.body.style.overflow = 'hidden';
    }

    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.body.style.overflow = '';
    };
  }, [isOpen, onClose]);

  const handleCopyLink = async () => {
    if (!meme?.url) return;
    try {
      await navigator.clipboard.writeText(meme.url);
      setCopied(true);
      if (timeoutRefs.current.copied) {
        clearTimeout(timeoutRefs.current.copied);
      }
      timeoutRefs.current.copied = setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      logError('Failed to copy', { error: err });
    }
  };

  const handleDownload = () => {
    if (!meme?.url) return;
    const a = document.createElement('a');
    a.href = meme.url;
    a.download = `meme-${meme.id}.${meme.format || 'jpg'}`;
    a.click();
    setDownloaded(true);
    if (timeoutRefs.current.downloaded) {
      clearTimeout(timeoutRefs.current.downloaded);
    }
    timeoutRefs.current.downloaded = setTimeout(() => setDownloaded(false), 2000);
  };

  const handleCopyImage = async () => {
    if (!meme?.url) return;
    try {
      const response = await fetch(meme.url);
      const blob = await response.blob();
      await navigator.clipboard.write([
        new ClipboardItem({ [blob.type]: blob })
      ]);
      setCopied(true);
      if (timeoutRefs.current.copied) {
        clearTimeout(timeoutRefs.current.copied);
      }
      timeoutRefs.current.copied = setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback to copying URL
      handleCopyLink();
    }
  };

  const handleImageError = () => {
    setImageErrorState({ memeId: activeMemeId, hasError: true });
  };

  return (
    <AnimatePresence>
      {isOpen && meme && (
        <motion.div
          className={styles.overlay}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={onClose}
        >
          <motion.div
            ref={modalRef}
            className={styles.modal}
            role="dialog"
            aria-modal="true"
            aria-labelledby="meme-modal-title"
            tabIndex={-1}
            initial={{ opacity: 0, scale: 0.9, y: 20 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: 10 }}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Close button */}
            <motion.button
              className={styles.closeBtn}
              onClick={onClose}
              aria-label="关闭详情"
              whileHover={{ scale: 1.1, rotate: 90 }}
              whileTap={{ scale: 0.9 }}
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </motion.button>

            {/* Image section */}
            <div className={styles.imageSection}>
              {imageError ? (
                <div className={styles.imageError}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <circle cx="12" cy="12" r="10" />
                    <line x1="12" y1="8" x2="12" y2="12" />
                    <line x1="12" y1="16" x2="12.01" y2="16" />
                  </svg>
                  <p>图片加载失败</p>
                </div>
              ) : (
                <motion.img
                  src={meme.url}
                  alt={description || 'Meme'}
                  className={styles.image}
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  transition={{ delay: 0.1 }}
                  onError={handleImageError}
                />
              )}
              {/* Score badge */}
              {scorePercent !== null && (
                <div className={`${styles.scoreBadge} ${scoreTone}`}>
                  匹配度 {scorePercent}%
                </div>
              )}
            </div>

            {/* Info section */}
            <div className={styles.infoSection}>
              <h3 id="meme-modal-title" className={styles.modalTitle}>表情详情</h3>

              {/* Actions */}
              <div className={styles.actions}>
                <motion.button
                  className={`${styles.actionBtn} ${styles.primary}`}
                  onClick={handleCopyImage}
                  aria-label="复制图片到剪贴板"
                  whileHover={{ scale: 1.02 }}
                  whileTap={{ scale: 0.98 }}
                >
                  {copied ? (
                    <>
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                        <polyline points="20,6 9,17 4,12" />
                      </svg>
                      已复制
                    </>
                  ) : (
                    <>
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <rect x="9" y="9" width="13" height="13" rx="2" />
                        <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" />
                      </svg>
                      复制图片
                    </>
                  )}
                </motion.button>

                <motion.button
                  className={styles.actionBtn}
                  onClick={handleDownload}
                  aria-label="下载表情图片"
                  whileHover={{ scale: 1.02 }}
                  whileTap={{ scale: 0.98 }}
                >
                  {downloaded ? (
                    <>
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                        <polyline points="20,6 9,17 4,12" />
                      </svg>
                      已下载
                    </>
                  ) : (
                    <>
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
                        <polyline points="7,10 12,15 17,10" />
                        <line x1="12" y1="15" x2="12" y2="3" />
                      </svg>
                      下载
                    </>
                  )}
                </motion.button>

                <motion.button
                  className={styles.actionBtn}
                  onClick={handleCopyLink}
                  aria-label="复制图片链接"
                  whileHover={{ scale: 1.02 }}
                  whileTap={{ scale: 0.98 }}
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M10 13a5 5 0 007.54.54l3-3a5 5 0 00-7.07-7.07l-1.72 1.71" />
                    <path d="M14 11a5 5 0 00-7.54-.54l-3 3a5 5 0 007.07 7.07l1.71-1.71" />
                  </svg>
                  复制链接
                </motion.button>
              </div>

              {/* Description */}
              {description && (
                <div className={styles.descriptionBox}>
                  <h4 className={styles.descriptionTitle}>
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <circle cx="12" cy="12" r="10" />
                      <path d="M9.09 9a3 3 0 015.83 1c0 2-3 3-3 3" />
                      <line x1="12" y1="17" x2="12.01" y2="17" />
                    </svg>
                    AI 识别描述
                  </h4>
                  <p className={styles.description}>{description}</p>
                </div>
              )}

              {/* Meta info */}
              <div className={styles.meta}>
                {meme.format && (
                  <span className={styles.metaItem}>
                    <span className={styles.metaLabel}>格式:</span>
                    <span className={styles.metaValue}>{meme.format.toUpperCase()}</span>
                  </span>
                )}
                {meme.width && meme.height && (
                  <span className={styles.metaItem}>
                    <span className={styles.metaLabel}>尺寸:</span>
                    <span className={styles.metaValue}>{meme.width} × {meme.height}</span>
                  </span>
                )}
              </div>

              {/* Tags */}
              {displayTags.length > 0 && (
                <div className={styles.tags}>
                  {displayTags.map((tag) => (
                    <span key={tag} className={styles.tag}>{tag}</span>
                  ))}
                </div>
              )}

            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
