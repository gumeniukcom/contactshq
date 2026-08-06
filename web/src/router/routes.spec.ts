import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

/**
 * SyncView.vue existed for months with no route pointing at it, which made every
 * /sync/providers endpoint unreachable from the UI — a whole feature that was built, shipped
 * and invisible. This test is what stops that recurring.
 *
 * Reading the router source rather than importing it: the route definitions are lazy
 * `() => import(...)` thunks, so the file paths are only visible as text.
 */

// Paths from the project root: vitest runs with cwd = web/, and import.meta.url does not
// resolve to a real filesystem path under the jsdom environment.
const VIEWS_DIR = join(process.cwd(), 'src/views')
const ROUTER_SOURCE = readFileSync(join(process.cwd(), 'src/router/index.ts'), 'utf8')

/**
 * Files under views/ that are components rather than screens. They are rendered by a parent
 * view, so a route would make no sense; listing them explicitly means a new orphan has to be
 * justified here rather than slipping through.
 */
const NOT_SCREENS = new Set(['CardDAVStepConfig.vue', 'GoogleStepConfig.vue'])

function collectViews(dir: string, prefix = ''): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      out.push(...collectViews(join(dir, entry.name), `${prefix}${entry.name}/`))
      continue
    }
    if (entry.name.endsWith('.vue')) out.push(prefix + entry.name)
  }
  return out
}

describe('router', () => {
  it('has a route for every screen under views/', () => {
    const orphans = collectViews(VIEWS_DIR).filter((path) => {
      const filename = path.split('/').pop() as string
      if (NOT_SCREENS.has(filename)) return false
      return !ROUTER_SOURCE.includes(`views/${path}`)
    })

    expect(orphans).toEqual([])
  })

  it('routes the sync providers screen', () => {
    expect(ROUTER_SOURCE).toContain('views/sync/SyncView.vue')
  })
})
