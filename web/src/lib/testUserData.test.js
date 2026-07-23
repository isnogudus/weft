import { describe, expect, it } from 'vitest'
import { UID_PATTERN } from './importModel.js'
import { generateTestUsers } from './testUserData.js'

describe('generateTestUsers', () => {
  it('names rows givenname_s### with zero-padded, sequential numbers', () => {
    const rows = generateTestUsers({ givenName: 'Anna', sn: 'Müller', start: 0, count: 3 })
    expect(rows.map((r) => r.uid)).toEqual(['anna_m000', 'anna_m001', 'anna_m002'])
    for (const r of rows) expect(r.uid).toMatch(UID_PATTERN)
  })

  it('respects a non-zero start and transliterates umlauts', () => {
    const rows = generateTestUsers({ givenName: 'Jürgen', sn: 'Örtel', start: 7, count: 2 })
    expect(rows.map((r) => r.uid)).toEqual(['juergen_o007', 'juergen_o008'])
  })

  it('carries givenName/sn through for the review table and validation', () => {
    const [row] = generateTestUsers({ givenName: 'Anna', sn: 'Müller', count: 1 })
    expect(row.givenName).toBe('Anna')
    expect(row.sn).toBe('Müller')
    expect(row.cn).toBe('')
  })

  it('only fills extra attributes that are actually configured', () => {
    const userAttrs = [{ attr: 'st' }, { attr: 'telephoneNumber' }]
    const [row] = generateTestUsers({ givenName: 'Anna', sn: 'Müller', count: 1, userAttrs })
    expect(Object.keys(row.extra).sort()).toEqual(['st', 'telephoneNumber'])
    expect(row.extra.st).toBeTruthy()
    expect(row.extra.telephoneNumber).toMatch(/^\+49 \d+ \d{4}$/)
  })

  it('fills no extra attributes when none are configured', () => {
    const [row] = generateTestUsers({ givenName: 'Anna', sn: 'Müller', count: 1 })
    expect(row.extra).toEqual({})
  })

  it('sets mail only when a domain is given', () => {
    const [withDomain] = generateTestUsers({ givenName: 'Anna', sn: 'Müller', count: 1, mailDomain: 'beispiel.de' })
    expect(withDomain.mail).toBe('anna.mueller@beispiel.de')
    const [withoutDomain] = generateTestUsers({ givenName: 'Anna', sn: 'Müller', count: 1 })
    expect(withoutDomain.mail).toBe('')
  })

  it('generates the requested count with no duplicate uids', () => {
    const rows = generateTestUsers({ givenName: 'Bob', sn: 'Smith', count: 50 })
    expect(rows).toHaveLength(50)
    expect(new Set(rows.map((r) => r.uid)).size).toBe(50)
  })

  it('leaves password blank by default, so each row gets its own passphrase later', () => {
    const rows = generateTestUsers({ givenName: 'Anna', sn: 'Müller', count: 3 })
    expect(rows.every((r) => r.password === '')).toBe(true)
  })

  it('applies a uniform password to every row when given', () => {
    const rows = generateTestUsers({ givenName: 'Anna', sn: 'Müller', count: 3, password: 'einheitlich-123' })
    expect(rows.every((r) => r.password === 'einheitlich-123')).toBe(true)
  })
})
