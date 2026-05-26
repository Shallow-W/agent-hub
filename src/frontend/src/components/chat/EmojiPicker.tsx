import React, { useCallback, useRef, useEffect } from 'react';
import styles from './EmojiPicker.module.css';

const EMOJIS = [
  '😀', '😁', '😂', '🤣', '😃', '😄', '😅', '😆',
  '😉', '😊', '😋', '😎', '😍', '🥰', '😘', '😗',
  '😏', '😒', '😞', '😔', '😟', '😕', '🙁', '😣',
  '😢', '😭', '😤', '😠', '😱', '😳', '🤔', '🤗',
  '🤫', '🤭', '🤢', '🤮', '🥵', '🥶', '😴', '🥱',
  '👍', '👎', '👏', '🙌', '🤝', '💪', '✌️', '🤞',
  '❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '🤍',
  '🔥', '⭐', '🌟', '✨', '🎉', '🎊', '💯', '🏆',
  '👀', '🙏', '💬', '📝', '🎯', '🚀', '💡', '⚡',
];

interface EmojiPickerProps {
  onSelect: (emoji: string) => void;
  onClose: () => void;
}

export const EmojiPicker: React.FC<EmojiPickerProps> = ({ onSelect, onClose }) => {
  const containerRef = useRef<HTMLDivElement>(null);

  const handleClickOutside = useCallback(
    (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        onClose();
      }
    },
    [onClose],
  );

  useEffect(() => {
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [handleClickOutside]);

  return (
    <div className={styles.emojiPicker} ref={containerRef}>
      <div className={styles.emojiGrid}>
        {EMOJIS.map((emoji) => (
          <button
            key={emoji}
            className={styles.emojiItem}
            onClick={() => onSelect(emoji)}
            type="button"
          >
            {emoji}
          </button>
        ))}
      </div>
    </div>
  );
};
