import { motion, AnimatePresence } from 'framer-motion';
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
  const currentIndex = getStageIndex(stage);
  const isThinking = stage === 'thinking' || stage === 'query_expansion_start';

  return (
    <motion.div
      className={styles.container}
      initial={{ opacity: 0, y: -20, height: 0 }}
      animate={{ opacity: 1, y: 0, height: 'auto' }}
      exit={{ opacity: 0, y: -20, height: 0 }}
      transition={{ duration: 0.3, ease: [0.16, 1, 0.3, 1] }}
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
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              ) : index === currentIndex ? (
                <motion.div
                  className={styles.stepPulse}
                  animate={{ scale: [1, 1.3, 1] }}
                  transition={{ duration: 1, repeat: Infinity }}
                />
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
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.2 }}
      >
        {message}
      </motion.div>

      {/* Thinking Bubble */}
      <AnimatePresence>
        {(isThinking || thinkingText) && (
          <motion.div
            className={styles.thinkingContainer}
            initial={{ opacity: 0, y: 10, height: 0 }}
            animate={{ opacity: 1, y: 0, height: 'auto' }}
            exit={{ opacity: 0, y: 10, height: 0 }}
            transition={{ duration: 0.3 }}
          >
            <div className={styles.thinkingBubble}>
              <div className={styles.thinkingIcon}>💭</div>
              <div className={styles.thinkingContent}>
                {thinkingText || (
                  <span className={styles.thinkingPlaceholder}>
                    <motion.span
                      animate={{ opacity: [0.4, 1, 0.4] }}
                      transition={{ duration: 1.5, repeat: Infinity }}
                    >
                      思考中...
                    </motion.span>
                  </span>
                )}
                {isThinking && (
                  <motion.span
                    className={styles.cursor}
                    animate={{ opacity: [1, 0] }}
                    transition={{ duration: 0.5, repeat: Infinity }}
                  >
                    |
                  </motion.span>
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
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.3 }}
          >
            <span className={styles.expandedLabel}>理解为：</span>
            <span className={styles.expandedText}>{expandedQuery}</span>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Cancel Button */}
      {onCancel && (
        <motion.button
          className={styles.cancelBtn}
          onClick={onCancel}
          whileHover={{ scale: 1.05 }}
          whileTap={{ scale: 0.95 }}
        >
          取消
        </motion.button>
      )}
    </motion.div>
  );
}

