import { motion, AnimatePresence, useReducedMotion } from 'framer-motion';
import { Check, MessageCircleQuestion } from 'lucide-react';
import type { SearchStageSlug } from '../api';
import styles from './SearchProgress.module.css';

interface SearchProgressProps {
  stage: SearchStageSlug;
  message: string;
  thinkingText: string;
  expandedQuery?: string;
  onCancel?: () => void;
}

const STAGES: { key: SearchStageSlug; label: string }[] = [
  { key: 'query_expansion_start', label: '理解意图' },
  { key: 'embedding', label: '生成向量' },
  { key: 'searching', label: '搜索' },
  { key: 'enriching', label: '加载' },
];

function getStageIndex(stage: SearchStageSlug): number {
  if (stage === 'thinking' || stage === 'query_expansion_done') {
    return 0; // Still in query expansion phase
  }
  const index = STAGES.findIndex((s) => s.key === stage);
  return index >= 0 ? index : 0;
}

export default function SearchProgress({
  stage,
  message,
  thinkingText,
  expandedQuery,
  onCancel,
}: SearchProgressProps) {
  const shouldReduceMotion = useReducedMotion();
  const currentIndex = getStageIndex(stage);
  const isThinking = stage === 'thinking' || stage === 'query_expansion_start';

  return (
    <motion.div
      className={styles.container}
      initial={{
        opacity: 0,
        transform: shouldReduceMotion ? 'none' : 'translateY(-8px)',
      }}
      animate={{ opacity: 1, transform: 'translateY(0)' }}
      exit={{
        opacity: 0,
        transform: shouldReduceMotion ? 'none' : 'translateY(-6px)',
      }}
      transition={{
        duration: shouldReduceMotion ? 0.12 : 0.2,
        ease: [0.23, 1, 0.32, 1],
      }}
    >
      {/* Progress Steps */}
      <div className={styles.progressSteps}>
        {STAGES.map((s, index) => (
          <div
            key={s.key}
            className={`${styles.step} ${index <= currentIndex ? styles.stepActive : ''} ${
              index === currentIndex ? styles.stepCurrent : ''
            }`}
          >
            <div className={styles.stepDot}>
              {index < currentIndex ? (
                <Check />
              ) : index === currentIndex ? (
                <span className={styles.stepPulse} />
              ) : null}
            </div>
            <span className={styles.stepLabel}>{s.label}</span>
            {index < STAGES.length - 1 && <div className={styles.stepLine} />}
          </div>
        ))}
      </div>

      {/* Current Stage Message */}
      <motion.div
        className={styles.message}
        key={message}
        role="status"
        aria-live="polite"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: shouldReduceMotion ? 0.1 : 0.16 }}
      >
        {message}
      </motion.div>

      {/* Thinking Bubble */}
      <AnimatePresence>
        {(isThinking || thinkingText) && (
          <motion.div
            className={styles.thinkingContainer}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: shouldReduceMotion ? 0.1 : 0.16 }}
          >
            <div className={styles.thinkingBubble}>
              <div className={styles.thinkingIcon} aria-hidden="true">
                <MessageCircleQuestion />
              </div>
              <div className={styles.thinkingContent}>
                {thinkingText || (
                  <span className={styles.thinkingPlaceholder}>思考中...</span>
                )}
                {isThinking && (
                  <span className={styles.cursor}>|</span>
                )}
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Expanded Query Display */}
      <AnimatePresence>
        {expandedQuery && stage !== 'thinking' && stage !== 'query_expansion_start' && (
          <motion.div
            className={styles.expandedQuery}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: shouldReduceMotion ? 0.1 : 0.16 }}
          >
            <span className={styles.expandedLabel}>理解为：</span>
            <span className={styles.expandedText}>{expandedQuery}</span>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Cancel Button */}
      {onCancel && (
        <button
          type="button"
          className={styles.cancelBtn}
          onClick={onCancel}
        >
          取消
        </button>
      )}
    </motion.div>
  );
}
