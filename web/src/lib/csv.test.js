import { describe, expect, it } from 'vitest'
import { parseCSV, sniffDelimiter, toCSV } from './csv.js'

describe('sniffDelimiter', () => {
  it('prefers the most frequent delimiter in the first line', () => {
    expect(sniffDelimiter('a;b;c\n1;2;3')).toBe(';')
    expect(sniffDelimiter('a,b,c\n')).toBe(',')
    expect(sniffDelimiter('a\tb\tc')).toBe('\t')
  })
  it('ignores delimiters inside quotes and defaults to comma', () => {
    expect(sniffDelimiter('"a;b;c",x\n')).toBe(',')
    expect(sniffDelimiter('abc\n')).toBe(',')
  })
})

describe('parseCSV', () => {
  it('parses simple rows with CRLF and LF', () => {
    expect(parseCSV('a,b\r\nc,d\ne,f')).toEqual([['a', 'b'], ['c', 'd'], ['e', 'f']])
  })
  it('handles quotes, escaped quotes and embedded delimiters/newlines', () => {
    expect(parseCSV('"a,x",b\n"say ""hi""","l1\nl2"')).toEqual([
      ['a,x', 'b'],
      ['say "hi"', 'l1\nl2'],
    ])
  })
  it('strips a UTF-8 BOM and drops blank lines', () => {
    expect(parseCSV('﻿uid,sn\n\nalice,A\n')).toEqual([['uid', 'sn'], ['alice', 'A']])
  })
  it('sniffs semicolons from German Excel exports', () => {
    expect(parseCSV('Nachname;Vorname\nMüller;Änne')).toEqual([
      ['Nachname', 'Vorname'],
      ['Müller', 'Änne'],
    ])
  })
})

describe('toCSV', () => {
  it('round-trips through parseCSV', () => {
    const rows = [['uid', 'pw'], ['alice', 'kühne-Tür,42'], ['bob', 'with "q" and\nnewline']]
    expect(parseCSV(toCSV(rows), ',')).toEqual(rows)
  })
})
