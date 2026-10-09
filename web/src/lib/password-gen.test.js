import { describe, expect, it } from 'vitest'
import { DIGITS, LOWER, UPPER, generatePassword } from './password-gen.js'

const count = (s, alphabet) => [...s].filter((c) => alphabet.includes(c)).length

describe('generatePassword', () => {
  it('has four blocks of four characters joined by dots', () => {
    for (let i = 0; i < 300; i++) {
      expect(generatePassword()).toMatch(/^[0-9A-Za-z]{4}(\.[0-9A-Za-z]{4}){3}$/)
    }
  })

  it('contains at least one lowercase letter, uppercase letter and digit', () => {
    for (let i = 0; i < 300; i++) {
      const p = generatePassword().replaceAll('.', '')
      const l = count(p, LOWER)
      const u = count(p, UPPER)
      const d = count(p, DIGITS)
      expect(l).toBeGreaterThanOrEqual(1)
      expect(u).toBeGreaterThanOrEqual(1)
      expect(d).toBeGreaterThanOrEqual(1)
      expect(l + u + d).toBe(16)
    }
  })

  it('never uses easily confused letters or y/z', () => {
    for (let i = 0; i < 300; i++) {
      expect(generatePassword()).not.toMatch(/[lIoOyzYZ]/)
    }
  })

  it('reaches every character of the pool', () => {
    const seen = new Set()
    for (let i = 0; i < 2000; i++) {
      for (const c of generatePassword().replaceAll('.', '')) seen.add(c)
    }
    expect(seen.size).toBe(LOWER.length + UPPER.length + DIGITS.length)
  })
})
