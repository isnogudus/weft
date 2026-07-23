// Async wrapper around the German passphrase generator. The word lists
// (~200 KB) are dynamically imported on first use so they stay out of the
// main bundle.

import { byteLen } from './importModel.js'

let mod = null

// generatePassword returns a passphrase within maxBytes (German umlauts count
// two bytes; two long adjectives can exceed a bcrypt-bound limit, so retry).
// The word list contains a few multi-word adjectives ("al dente"); passphrases
// with whitespace are rejected too, so the result is always one shell-safe token.
export async function generatePassword(maxBytes = 72) {
  mod ??= await import('./passphrase.js')
  for (;;) {
    const p = mod.generatePassphrase()
    if (byteLen(p) <= maxBytes && !/\s/.test(p)) return p
  }
}
