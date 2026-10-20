import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router';

/**
 * A link to `#targetId` on the current page, made as a router navigation.
 * A native fragment link pushes a history entry with no state, which React
 * Router keys "default", the same key as the first-loaded entry, so
 * ScrollRestoration would put back that entry's saved offset instead of
 * scrolling to the target.
 */
export function JumpLink({
  targetId,
  children,
}: {
  targetId: string;
  children: ReactNode;
}) {
  const { search } = useLocation();
  return <Link to={{ search, hash: targetId }}>{children}</Link>;
}
