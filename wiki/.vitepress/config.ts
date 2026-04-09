import { defineConfig } from 'vitepress'
import { buildWikiSidebar } from './sidebar'

export default defineConfig({
  title: 'LLM Wiki',
  description: 'Compiled wiki pages from llm-wiki',
  ignoreDeadLinks: true,
  themeConfig: {
    nav: [{ text: 'Home', link: '/' }],
    sidebar: buildWikiSidebar(),
    search: { provider: 'local' },
  },
})
