import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type Theme = 'dark' | 'light'

const applyTheme = (t: Theme) => {
  document.documentElement.dataset.theme = t
}

interface ThemeState {
  theme: Theme
  toggleTheme: () => void
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      theme: 'dark' as Theme,
      toggleTheme: () =>
        set((s) => {
          const next = s.theme === 'dark' ? 'light' : 'dark'
          applyTheme(next)
          return { theme: next }
        }),
    }),
    { name: 'theme-storage' }
  )
)

// Apply theme to DOM at module load so it takes effect before React renders
applyTheme(useThemeStore.getState().theme)
