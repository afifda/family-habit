import type { ChildAvatar as ChildAvatarName } from '../api/client';

const symbols: Record<ChildAvatarName, string> = {
  fox: '🦊',
  bear: '🐻',
  rabbit: '🐰',
  owl: '🦉',
  cat: '🐱',
  elephant: '🐘',
  panda: '🐼',
  koala: '🐨',
};

export function ChildAvatar({
  avatar,
  color,
  size = 'normal',
}: {
  avatar: ChildAvatarName;
  color: string;
  size?: 'normal' | 'large';
}) {
  return (
    <span
      className={`avatar child-avatar ${size === 'large' ? 'avatar-large' : ''}`}
      style={{ backgroundColor: color }}
      aria-hidden="true"
    >
      {symbols[avatar]}
    </span>
  );
}
