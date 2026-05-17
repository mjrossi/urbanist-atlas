import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Dateline } from './Dateline.tsx';

describe('Dateline', () => {
  it('renders the postal code and country with a resolved place label', () => {
    render(<Dateline postalCode="11217" country="US" placeLabel="Brooklyn, NY" />);
    expect(screen.getByText('11217')).toBeDefined();
    expect(screen.getByText('Brooklyn, NY')).toBeDefined();
    expect(screen.getByText('US')).toBeDefined();
  });

  it('omits the place block when no label is supplied', () => {
    const { container } = render(<Dateline postalCode="00000" country="US" />);
    expect(screen.getByText('00000')).toBeDefined();
    expect(container.querySelector('.dateline-place')).toBeNull();
  });
});
