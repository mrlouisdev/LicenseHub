import { Check, LoaderCircle, X } from 'lucide-react';
import type { PropsWithChildren, ReactNode } from 'react';
import type { AuditEvent, LicenseStatus } from '../types';

export function Modal({ title, description, onClose, children }: PropsWithChildren<{ title: string; description?: string; onClose: () => void }>) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
        <div className="modal-head"><div><h2 id="modal-title">{title}</h2>{description && <p>{description}</p>}</div><button className="icon-button" aria-label="Close" onClick={onClose}><X /></button></div>
        {children}
      </section>
    </div>
  );
}

export function Button({ children, variant = 'primary', busy, ...props }: PropsWithChildren<React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' | 'danger' | 'ghost'; busy?: boolean }>) {
  return <button {...props} disabled={props.disabled || busy} className={`button ${variant} ${props.className ?? ''}`}>{busy ? <LoaderCircle className="spin" size={16} /> : null}{children}</button>;
}

export function StatusBadge({ value }: { value: LicenseStatus | AuditEvent['severity'] | 'online' | 'offline' | 'active' | 'retiring' | 'verified' | 'running' }) {
  return <span className={`badge ${value}`}><i />{value}</span>;
}

export function Empty({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return <div className="empty">{icon}<strong>{title}</strong><p>{text}</p></div>;
}

export function Toast({ children }: PropsWithChildren) {
  return <div className="toast" role="status"><Check size={16} />{children}</div>;
}
