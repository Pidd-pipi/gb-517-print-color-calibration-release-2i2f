import type { ReactNode } from 'react';
import { Button } from 'antd';
export function UiButton({ children, onClick, disabled = false, danger = false }: { children: ReactNode; onClick?: () => void; disabled?: boolean; danger?: boolean }) {
  return <Button type="primary" danger={danger} onClick={onClick} disabled={disabled}>{children}</Button>;
}
