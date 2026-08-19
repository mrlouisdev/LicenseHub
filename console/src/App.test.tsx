import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { App } from './App';

describe('LicenseHub Console', () => {
  it('opens first-run connection setup instead of silently using mock data', async () => {
    window.localStorage.clear();
    render(<MemoryRouter><App /></MemoryRouter>);
    expect(await screen.findByRole('heading', { name: 'Connect your control plane.' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /explore demo mode/i })).toBeInTheDocument();
  });

  it('renders the dashboard and primary metrics', async () => {
    render(<MemoryRouter><App initialDemo /></MemoryRouter>);
    expect(await screen.findByRole('heading', { name: 'Control center' })).toBeInTheDocument();
    expect(await screen.findByText('Active licenses')).toBeInTheDocument();
    expect(screen.getByText('Activation volume')).toBeInTheDocument();
  });

  it('creates a one-device product with a 72-hour lease', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter initialEntries={['/products']}><App initialDemo /></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: /new product/i }));
    await user.type(screen.getByLabelText('Product name'), 'Sentinel App');
    await user.type(screen.getByLabelText('Product code'), 'SENT');
    await user.click(screen.getByRole('button', { name: /create product/i }));
    const heading = await screen.findByRole('heading', { name: 'Sentinel App' });
    const card = heading.closest('article');
    expect(card).not.toBeNull();
    expect(within(card!).getByText('72h')).toBeInTheDocument();
    expect(within(card!).getByText('1', { selector: '.policy-grid strong' })).toBeInTheDocument();
  });

  it('generates a license through the typed adapter', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter initialEntries={['/licenses']}><App initialDemo /></MemoryRouter>);
    expect((await screen.findAllByText('Atlas Studio', { selector: 'td' })).length).toBeGreaterThan(0);
    await user.click(screen.getByRole('button', { name: /generate license/i }));
    await user.selectOptions(screen.getByLabelText('Product'), 'prd_atlas');
    await user.type(screen.getByLabelText('Customer / note'), 'Test Customer');
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: /generate license/i }));
    expect(await screen.findByRole('heading', { name: 'License generated' })).toBeInTheDocument();
    expect(within(screen.getByRole('dialog')).getByText(/Test Customer/)).toBeInTheDocument();
  });
});
