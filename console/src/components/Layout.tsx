import {
  Activity, Boxes, ChevronLeft, ChevronRight, CircleUserRound, DatabaseBackup,
  Fingerprint, KeyRound, LayoutDashboard, LogOut, Menu, ScrollText, Server, Settings, ShieldCheck, X,
} from 'lucide-react';
import { useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { useConnection } from '../connection/ConnectionContext';

const navigation = [
  { to: '/', label: 'Overview', icon: LayoutDashboard },
  { to: '/products', label: 'Products', icon: Boxes },
  { to: '/licenses', label: 'Licenses', icon: KeyRound },
  { to: '/devices', label: 'Devices', icon: Fingerprint },
  { to: '/signing-keys', label: 'Signing keys', icon: ShieldCheck },
  { to: '/vps-backups', label: 'VPS & backups', icon: DatabaseBackup },
  { to: '/audit', label: 'Audit logs', icon: ScrollText },
  { to: '/connection', label: 'Connection', icon: Settings },
];

const titles: Record<string, [string, string]> = {
  '/': ['Control center', 'Monitor licenses, devices, and infrastructure.'],
  '/products': ['Products', 'Configure license policy for every application.'],
  '/licenses': ['Licenses', 'Generate, inspect, and control customer access.'],
  '/devices': ['Devices', 'Review active machines and reset device bindings.'],
  '/signing-keys': ['Signing keys', 'Manage the public trust chain for offline leases.'],
  '/vps-backups': ['VPS & backups', 'Health, recovery, and migration readiness.'],
  '/audit': ['Audit logs', 'An immutable trail of administrative activity.'],
  '/connection': ['Connection', 'Manage the LicenseHub server session.'],
};

export function Layout() {
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const location = useLocation();
  const connection = useConnection();
  const [title, subtitle] = titles[location.pathname] ?? titles['/'];

  return (
    <div className={`shell ${collapsed ? 'nav-collapsed' : ''}`}>
      <button className="mobile-menu icon-button" aria-label="Open navigation" onClick={() => setMobileOpen(true)}><Menu /></button>
      {mobileOpen && <button className="scrim" aria-label="Close navigation" onClick={() => setMobileOpen(false)} />}
      <aside className={`sidebar ${mobileOpen ? 'mobile-open' : ''}`}>
        <div className="brand">
          <span className="brand-mark"><ShieldCheck size={21} /></span>
          {!collapsed && <div><strong>LicenseHub</strong><small>CONTROL PLANE</small></div>}
          <button className="mobile-close icon-button" aria-label="Close navigation" onClick={() => setMobileOpen(false)}><X /></button>
        </div>
        <nav>
          <span className="nav-label">{collapsed ? '•••' : 'Workspace'}</span>
          {navigation.map(({ to, label, icon: Icon }) => (
            <NavLink key={to} to={to} end={to === '/'} onClick={() => setMobileOpen(false)} title={collapsed ? label : undefined}>
              <Icon size={18} /><span>{label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="server-card">
          <span className="pulse"><span /></span>
          {!collapsed && <div><strong>{connection.mode === 'demo' ? 'Demo mode' : 'Production VPS'}</strong><small>{connection.mode === 'demo' ? 'Sample data · no server writes' : connection.serverUrl}</small></div>}
        </div>
        <button className="collapse-button" onClick={() => setCollapsed((value) => !value)} aria-label="Toggle navigation">
          {collapsed ? <ChevronRight size={17} /> : <><ChevronLeft size={17} /><span>Collapse</span></>}
        </button>
      </aside>
      <main className="main">
        <header className="topbar">
          <div><p className="eyebrow">LICENSE OPERATIONS</p><h1>{title}</h1><p>{subtitle}</p></div>
          <div className="top-actions">
            <div className={`health ${connection.mode === 'demo' ? 'demo-health' : ''}`}><Activity size={15} /><span>{connection.mode === 'demo' ? 'Demo data' : 'API connected'}</span></div>
            <div className="profile"><CircleUserRound size={29} /><div><strong>{connection.mode === 'demo' ? 'Demo operator' : connection.user?.name || 'Administrator'}</strong><small>{connection.mode === 'demo' ? 'Explicit Demo mode' : connection.user?.email}</small></div></div>
            <button className="icon-button" aria-label={connection.mode === 'demo' ? 'Exit Demo mode' : 'Log out'} onClick={() => void connection.logout()}><LogOut /></button>
          </div>
        </header>
        <section className="content"><Outlet /></section>
        <footer><Server size={13} /> {connection.mode === 'demo' ? 'Demo mode' : connection.serverUrl} <span>·</span> Console v0.1.0</footer>
      </main>
    </div>
  );
}
