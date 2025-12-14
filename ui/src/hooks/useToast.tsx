import type { ReactNode } from 'react';
import {
  createContext,
  useContext,
  useState,
  useCallback,
} from 'react';

import {
  ToastWithIcon,
  ToastProvider,
  ToastViewport,
} from '../components/ui/Toast';

type ToastVariant = 'default' | 'destructive' | 'success';

interface Toast {
  id: string;
  title: string;
  description?: string;
  variant: ToastVariant;
  duration: number;
}

interface ToastOptions {
  title: string;
  description?: string;
  variant?: ToastVariant;
  duration?: number;
}

interface ToastContextValue {
  toast: (options: ToastOptions) => string;
  dismiss: (toastId: string) => void;
}

const ToastContext = createContext<ToastContextValue | undefined>(undefined);

export const useToast = (): ToastContextValue => {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within a ToastProviderComponent');
  }
  return context;
};

interface ToastProviderComponentProps {
  children: ReactNode;
}

export const ToastProviderComponent = ({
  children,
}: ToastProviderComponentProps) => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const toast = useCallback(
    ({
      title,
      description,
      variant = 'default',
      duration = 5000,
    }: ToastOptions): string => {
      const id = Math.random().toString(36).substr(2, 9);
      const newToast: Toast = {
        id,
        title,
        ...(description !== undefined && { description }),
        variant,
        duration,
      };

      setToasts((prevToasts) => [...prevToasts, newToast]);

      // Auto remove toast after duration
      setTimeout(() => {
        setToasts((prevToasts) => prevToasts.filter((t) => t.id !== id));
      }, duration);

      return id;
    },
    []
  );

  const dismiss = useCallback((toastId: string): void => {
    setToasts((prevToasts) => prevToasts.filter((t) => t.id !== toastId));
  }, []);

  return (
    <ToastProvider swipeDirection="right">
      <ToastContext.Provider value={{ toast, dismiss }}>
        {children}
        <ToastViewport />
        {toasts.map((toastItem) => (
          <ToastWithIcon
            key={toastItem.id}
            variant={toastItem.variant}
            title={toastItem.title}
            {...(toastItem.description !== undefined && { description: toastItem.description })}
            onOpenChange={(open) => {
              if (!open) {
                dismiss(toastItem.id);
              }
            }}
          />
        ))}
      </ToastContext.Provider>
    </ToastProvider>
  );
};
