import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TagChip } from './TagChip.tsx';

describe('TagChip', () => {
  it('renders its label inside a .tag-chip span', () => {
    render(<TagChip label="cycling" />);
    const chip = screen.getByText('cycling');
    expect(chip.tagName).toBe('SPAN');
    expect(chip.classList.contains('tag-chip')).toBe(true);
  });
});
