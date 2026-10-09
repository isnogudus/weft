import { describe, expect, it } from 'vitest'
import { DIGITS, LOWER, generatePassword } from './password-gen.js'

const count = (s, alphabet) => [...s].filter((c) => alphabet.includes(c)).length

describe('generatePassword', () => {
  it('has three blocks of four characters joined by dots', () => {
    for (let i = 0; i < 300; i++) {
      expect(generatePassword()).toMatch(/^[0-9a-z]{4}(\.[0-9a-z]{4}){2}$/)
    }
  })

  it('contains at least one lowercase letter and one digit, nothing else', () => {
    for (let i = 0; i < 300; i++) {
      const p = generatePassword().replaceAll('.', '')
      const l = count(p, LOWER)
      const d = count(p, DIGITS)
      expect(l).toBeGreaterThanOrEqual(1)
      expect(d).toBeGreaterThanOrEqual(1)
      expect(l + d).toBe(12)
    }
  })

  it('never uses easily confused letters, y/z or uppercase', () => {
    for (let i = 0; i < 300; i++) {
      expect(generatePassword()).not.toMatch(/[loyzA-Z]/)
    }
  })

  it('reaches every character of the pool', () => {
    const seen = new Set()
    for (let i = 0; i < 2000; i++) {
      for (const c of generatePassword().replaceAll('.', '')) seen.add(c)
    }
    expect(seen.size).toBe(LOWER.length + DIGITS.length)
  })
})
