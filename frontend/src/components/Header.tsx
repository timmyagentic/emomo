import { motion } from 'framer-motion';
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
  const formattedCount = new Intl.NumberFormat('en-US').format(memeCount);

  return (
    <motion.header
      className={styles.header}
      initial={{ opacity: 0, y: -20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5 }}
    >
      <div className={styles.container}>
        {/* Logo */}
        <motion.button
          className={styles.logo}
          onClick={onLogoClick}
          aria-label="返回首页"
          whileHover={{ scale: 1.02 }}
          whileTap={{ scale: 0.98 }}
        >
          <span className={styles.logoIcon}>
            <motion.span
              animate={{ rotate: [0, 5, -5, 0] }}
              transition={{ duration: 4, repeat: Infinity }}
            >
              😸
            </motion.span>
          </span>
          <span className={styles.logoText}>Emomo</span>
        </motion.button>

        {/* Right section */}
        <div className={styles.right}>
          {/* Stats */}
          <div className={styles.stats}>
            <span className={styles.statNumber}>{formattedCount}</span>
            <span className={styles.statLabel}>表情包</span>
          </div>
        </div>
      </div>
    </motion.header>
  );
}
