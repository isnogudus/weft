// Minimal RFC-4180 CSV parsing for the bulk import, plus a serializer for the
// one-time password download. German Excel exports use ";" as the delimiter,
// so the delimiter is sniffed from the first line unless given explicitly.

// sniffDelimiter counts candidate delimiters outside quotes in the first
// record line; the most frequent wins, comma on a tie or all-zero.
export function sniffDelimiter(text) {
  const counts = { ',': 0, ';': 0, '\t': 0 }
  let inQuotes = false
  for (const c of text) {
    if (inQuotes) {
      if (c === '"') inQuotes = false
    } else if (c === '"') inQuotes = true
    else if (c === '\n' || c === '\r') break
    else if (c in counts) counts[c]++
  }
  let best = ','
  for (const d of [';', '\t']) if (counts[d] > counts[best]) best = d
  return best
}

// parseCSV returns an array of rows (arrays of strings). Handles quoted
// fields, escaped quotes (""), embedded delimiters/newlines, CRLF and a UTF-8
// BOM. Blank lines are dropped.
export function parseCSV(text, delim = sniffDelimiter(text)) {
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1)
  const rows = []
  let row = []
  let field = ''
  let inQuotes = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (inQuotes) {
      if (c === '"') {
        if (text[i + 1] === '"') { field += '"'; i++ }
        else inQuotes = false
      } else field += c
    } else if (c === '"') inQuotes = true
    else if (c === delim) { row.push(field); field = '' }
    else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++
      row.push(field); rows.push(row); row = []; field = ''
    } else field += c
  }
  if (field !== '' || row.length) { row.push(field); rows.push(row) }
  return rows.filter((r) => !(r.length === 1 && r[0].trim() === ''))
}

// toCSV serializes rows with comma delimiter and CRLF line ends, quoting where
// needed. The caller prepends a BOM if the file is meant for Excel.
export function toCSV(rows) {
  const esc = (v) => {
    const s = String(v ?? '')
    return /[",\n\r;]/.test(s) ? '"' + s.replaceAll('"', '""') + '"' : s
  }
  return rows.map((r) => r.map(esc).join(',')).join('\r\n') + '\r\n'
}
