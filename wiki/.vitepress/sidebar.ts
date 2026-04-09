import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { DefaultTheme } from 'vitepress'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const wikiRoot = path.resolve(__dirname, '..')

const excludeDirs = new Set(['node_modules', '.vitepress', 'public'])

/** Build left sidebar: Home + one collapsible group per namespace directory (bbclaw, …). */
export function buildWikiSidebar(): DefaultTheme.SidebarItem[] {
  const sidebar: DefaultTheme.SidebarItem[] = [{ text: 'Home', link: '/' }]

  let entries: fs.Dirent[]
  try {
    entries = fs.readdirSync(wikiRoot, { withFileTypes: true })
  } catch {
    return sidebar
  }

  const namespaces = entries
    .filter((e) => e.isDirectory() && !e.name.startsWith('.') && !excludeDirs.has(e.name))
    .sort((a, b) => a.name.localeCompare(b.name))

  for (const ns of namespaces) {
    const dir = path.join(wikiRoot, ns.name)
    let files: string[]
    try {
      files = fs.readdirSync(dir).filter((f) => f.endsWith('.md'))
    } catch {
      continue
    }

    files.sort((a, b) => {
      if (a === 'index.md') return -1
      if (b === 'index.md') return 1
      return a.localeCompare(b)
    })

    const items: DefaultTheme.SidebarItem[] = []
    for (const file of files) {
      if (file === 'index.md') {
        items.push({ text: 'Index', link: `/${ns.name}/` })
        continue
      }
      const stem = file.slice(0, -'.md'.length)
      items.push({
        text: formatPageTitle(stem),
        link: pageLink(ns.name, stem),
      })
    }

    if (items.length === 0) continue

    sidebar.push({
      text: ns.name,
      collapsed: true,
      items,
    })
  }

  return sidebar
}

function pageLink(namespace: string, stem: string): string {
  return `/${namespace}/${stem}`
}

function formatPageTitle(stem: string): string {
  let t = stem.replace(/__/g, ' · ')
  t = t.replace(/_/g, ' ')
  return t
}
