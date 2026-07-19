import { useEffect, useState, useRef, useMemo } from 'react';
import { motion, AnimatePresence, useReducedMotion } from 'framer-motion';
import { Check, CircleAlert, CircleHelp, Copy, Download, X } from 'lucide-react';
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
 * Allows downloading and copying the image.
 *
 * @param props - The component props.
 * @param props.meme - The meme object to display.
 * @param props.isOpen - Controls the visibility of the modal.
 * @param props.onClose - Handler to close the modal.
 * @returns The rendered MemeModal component.
 */
export default function MemeModal({ meme, isOpen, onClose }: MemeModalProps) {
  const shouldReduceMotion = useReducedMotion();
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
          transition={{ duration: shouldReduceMotion ? 0.1 : 0.16 }}
          onClick={onClose}
        >
          <motion.div
            ref={modalRef}
            className={styles.modal}
            role="dialog"
            aria-modal="true"
            aria-labelledby="meme-modal-title"
            tabIndex={-1}
            initial={{
              opacity: 0,
              transform: shouldReduceMotion ? 'none' : 'translateY(14px) scale(0.98)',
            }}
            animate={{ opacity: 1, transform: 'translateY(0) scale(1)' }}
            exit={{
              opacity: 0,
              transform: shouldReduceMotion ? 'none' : 'translateY(8px) scale(0.985)',
            }}
            transition={{
              duration: shouldReduceMotion ? 0.12 : 0.22,
              ease: [0.23, 1, 0.32, 1],
            }}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Close button */}
            <button
              type="button"
              className={styles.closeBtn}
              onClick={onClose}
              aria-label="关闭详情"
            >
              <X />
            </button>

            {/* Image section */}
            <div className={styles.imageSection}>
              {imageError ? (
                <div className={styles.imageError}>
                  <CircleAlert />
                  <p>图片加载失败</p>
                </div>
              ) : (
                <motion.img
                  layoutId={shouldReduceMotion ? undefined : `meme-image-${meme.id}`}
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
                <button
                  type="button"
                  className={`${styles.actionBtn} ${styles.primary}`}
                  onClick={handleCopyImage}
                  aria-label="复制图片到剪贴板"
                >
                  {copied ? (
                    <>
                      <Check />
                      已复制
                    </>
                  ) : (
                    <>
                      <Copy />
                      复制图片
                    </>
                  )}
                </button>

                <button
                  type="button"
                  className={styles.actionBtn}
                  onClick={handleDownload}
                  aria-label="下载表情图片"
                >
                  {downloaded ? (
                    <>
                      <Check />
                      已下载
                    </>
                  ) : (
                    <>
                      <Download />
                      下载
                    </>
                  )}
                </button>

              </div>

              {/* Description */}
              {description && (
                <div className={styles.descriptionBox}>
                  <h4 className={styles.descriptionTitle}>
                    <CircleHelp />
                    AI 识别描述
                  </h4>
                  <p className={styles.description}>{description}</p>
                </div>
              )}

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
