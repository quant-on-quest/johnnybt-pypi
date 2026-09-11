import { useToast } from '@nuxt/ui/composables'

/** Thin wrapper so pages stay easy to test without the UApp provider. */
export function useNotify() {
  const toast = useToast()
  return {
    success: (title: string, description?: string) => toast.add({ title, description, color: 'success', icon: 'i-lucide-check' }),
    error: (title: string, description?: string) => toast.add({ title, description, color: 'error', icon: 'i-lucide-alert-triangle' }),
  }
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}
