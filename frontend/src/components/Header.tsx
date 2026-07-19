import { motion, useReducedMotion } from 'framer-motion';
import { Cat } from 'lucide-react';
import styles from './Header.module.css';

/**
 * Props for the Header component.
 */
interface HeaderProps {
  /**
   * Number of active memes available in the backend.
   */
  memeCount?: number;
  /**
   * Callback function triggered when the logo is clicked.
   * Typically used to reset the view or navigate home.
   */
  onLogoClick?: () => void;
}

/**
 * The application header component.
 * Displays the logo, statistics, and external links (e.g., GitHub).
 *
 * @param props - The component props.
 * @param props.onLogoClick - Handler for logo click events.
 * @returns The rendered Header component.
 */
export default function Header({ memeCount = 5791, onLogoClick }: HeaderProps) {
  const shouldReduceMotion = useReducedMotion();
  const formattedCount = new Intl.NumberFormat('en-US').format(memeCount);

  return (
    <motion.header
      className={styles.header}
      initial={{
        opacity: 0,
        transform: shouldReduceMotion ? 'none' : 'translateY(-8px)',
      }}
      animate={{ opacity: 1, transform: 'translateY(0)' }}
      transition={{ duration: 0.22, ease: [0.23, 1, 0.32, 1] }}
    >
      <div className={styles.container}>
        <button
          type="button"
          className={styles.logo}
          onClick={onLogoClick}
          aria-label="返回首页"
        >
          <span className={styles.logoIcon} aria-hidden="true">
            <Cat />
          </span>
          <span className={styles.logoText}>Emomo</span>
        </button>

        <div className={styles.right}>
          <div className={styles.stats} aria-label={`${formattedCount} 个表情包`}>
            <span className={styles.statNumber}>{formattedCount}</span>
            <span className={styles.statLabel}>表情包</span>
          </div>
        </div>
      </div>
    </motion.header>
  );
}
