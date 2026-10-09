import { describe, expect, it } from 'vitest'
import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'
import { serverMsg, setLang } from './i18n.svelte.js'

// Every fixed error message the Go server sends must have a German
// translation, or the German UI silently shows it in English.
function goServerMessages() {
  const dir = fileURLToPath(new URL('../../../internal/server/', import.meta.url))
  const msgs = new Set()
  const patterns = [
    /writeError\(w, [^,]+, "((?:[^"\\]|\\.)*)"\)/g, // complete literals only
    /const \w+Msg = "((?:[^"\\]|\\.)*)"/g,
    /res\.Error = "((?:[^"\\]|\\.)*)"/g,
  ]
  for (const f of readdirSync(dir)) {
    if (!f.endsWith('.go') || f.endsWith('_test.go')) continue
    const src = readFileSync(join(dir, f), 'utf8')
    for (const re of patterns) {
      for (const m of src.matchAll(re)) msgs.add(JSON.parse('"' + m[1] + '"'))
    }
  }
  return [...msgs]
}

describe('serverMsg', () => {
  it('translates every fixed server message to German', () => {
    setLang('de')
    const msgs = goServerMessages()
    expect(msgs.length).toBeGreaterThan(20)
    const missing = msgs.filter((m) => serverMsg(m) === m)
    expect(missing).toEqual([])
  })
  it('passes messages through in English and for unknown text', () => {
    setLang('en')
    expect(serverMsg('not found')).toBe('not found')
    setLang('de')
    expect(serverMsg('uid "x" is invalid')).toBe('uid "x" is invalid')
    expect(serverMsg(undefined)).toBe(undefined)
  })
})
