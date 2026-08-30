import { useEffect, useRef, useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import {
  previewFeedback,
  submitFeedback,
  type FeedbackPreviewView,
  type FeedbackReceiptView,
} from '../api';
import { logError } from '../utils/logger';
import styles from './FeedbackModal.module.css';

interface FeedbackModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export default function FeedbackModal({ isOpen, onClose }: FeedbackModalProps) {
  const [description, setDescription] = useState('');
  const [preview, setPreview] = useState<FeedbackPreviewView | null>(null);
  const [previewDescription, setPreviewDescription] = useState('');
  const [receipt, setReceipt] = useState<FeedbackReceiptView | null>(null);
  const [error, setError] = useState('');
  const [isPreviewing, setIsPreviewing] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const modalRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen) {
      modalRef.current?.focus();
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
      setDescription('');
      setPreview(null);
      setPreviewDescription('');
      setReceipt(null);
      setError('');
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !isSubmitting) onClose();
    };
    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isOpen, isSubmitting, onClose]);

  const handleDescriptionChange = (value: string) => {
    setDescription(value);
    setPreview(null);
    setPreviewDescription('');
    setReceipt(null);
    setError('');
  };

  const handlePreview = async () => {
    const value = description.trim();
    if (!value) {
      setError('请先填写反馈内容。');
      return;
    }
    setIsPreviewing(true);
    setError('');
    setReceipt(null);
    try {
      const nextPreview = await previewFeedback(value);
      setPreview(nextPreview);
      setPreviewDescription(value);
    } catch (nextError) {
      logError('Feedback preview failed', { error: nextError });
      setError('暂时无法生成脱敏预览，请稍后重试。');
    } finally {
      setIsPreviewing(false);
    }
  };

  const handleSubmit = async () => {
    if (!preview || !preview.submissionEnabled || !previewDescription) return;
    setIsSubmitting(true);
    setError('');
    try {
      setReceipt(await submitFeedback(previewDescription));
    } catch (nextError) {
      logError('Feedback submission failed', { error: nextError });
      setError('提交失败。你可以稍后重试，或使用公开反馈入口。');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          className={styles.overlay}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={() => !isSubmitting && onClose()}
        >
          <motion.div
            ref={modalRef}
            className={styles.modal}
            role="dialog"
            aria-modal="true"
            aria-labelledby="feedback-modal-title"
            tabIndex={-1}
            initial={{ opacity: 0, scale: 0.96, y: 16 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.97, y: 8 }}
            transition={{ type: 'spring', damping: 28, stiffness: 320 }}
            onClick={(event) => event.stopPropagation()}
          >
            <div className={styles.header}>
              <div>
                <h2 id="feedback-modal-title">发送反馈</h2>
                <p>先查看服务端生成的完整脱敏预览，再决定是否提交。</p>
              </div>
              <button type="button" className={styles.close} onClick={onClose} aria-label="关闭反馈">
                ×
              </button>
            </div>

            <div className={styles.content}>
              <label className={styles.label} htmlFor="feedback-description">
                反馈内容
              </label>
              <textarea
                id="feedback-description"
                className={styles.textarea}
                value={description}
                maxLength={4000}
                rows={5}
                disabled={isSubmitting}
                onChange={(event) => handleDescriptionChange(event.target.value)}
                placeholder="告诉我们哪里不好用，或希望增加什么能力"
              />

              {!preview && (
                <button
                  type="button"
                  className={styles.primary}
                  disabled={isPreviewing || !description.trim()}
                  onClick={handlePreview}
                >
                  {isPreviewing ? '正在脱敏…' : '预览脱敏内容'}
                </button>
              )}

              {preview && (
                <section className={styles.preview} aria-label="完整脱敏预览">
                  <h3>即将提交的完整内容</h3>
                  <dl>
                    <div><dt>产品</dt><dd>{preview.environment.product}</dd></div>
                    <div><dt>版本</dt><dd>{preview.environment.version || '未提供'}</dd></div>
                    <div><dt>平台</dt><dd>{preview.environment.os} / {preview.environment.arch}</dd></div>
                    <div><dt>Agent</dt><dd>{preview.environment.agent || '未提供'}</dd></div>
                    <div><dt>描述</dt><dd className={styles.longValue}>{preview.description}</dd></div>
                    <div>
                      <dt>近期错误</dt>
                      <dd className={styles.longValue}>
                        {preview.recentError ? `${preview.recentError.text} (${preview.recentError.occurredAt})` : '无'}
                      </dd>
                    </div>
                    <div>
                      <dt>能力缺口</dt>
                      <dd className={styles.longValue}>
                        {preview.capabilityGaps.length > 0 ? preview.capabilityGaps.join('、') : '无'}
                      </dd>
                    </div>
                  </dl>

                  {receipt ? (
                    <div className={styles.success}>
                      已提交。{' '}
                      <a href={receipt.referenceUrl} target="_blank" rel="noreferrer">查看反馈记录</a>
                    </div>
                  ) : (
                    <div className={styles.actions}>
                      <button type="button" className={styles.secondary} disabled={isSubmitting} onClick={() => handleDescriptionChange(description)}>
                        返回修改
                      </button>
                      {preview.submissionEnabled ? (
                        <button type="button" className={styles.primary} disabled={isSubmitting} onClick={handleSubmit}>
                          {isSubmitting ? '正在提交…' : '确认并提交这份脱敏反馈'}
                        </button>
                      ) : (
                        <span className={styles.unavailable}>当前未配置反馈 Relay，不会自动提交。</span>
                      )}
                    </div>
                  )}

                  {preview.publicFallbackUrl && !receipt && (
                    <a className={styles.fallback} href={preview.publicFallbackUrl} target="_blank" rel="noreferrer">
                      使用公开反馈入口
                    </a>
                  )}
                </section>
              )}

              {error && <p className={styles.error} role="alert">{error}</p>}
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
