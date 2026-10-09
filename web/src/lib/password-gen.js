// Random, easy-to-type passwords, the method of password-generator with
// lowercase and digits only ("-wd -l 12"): 12 characters, at least one of each
// class, shown in blocks of four joined by "." (e.g. "k3pa.7xmq.e2tn"). The
// dots are part of the password. 60 bits of entropy (12 characters from a pool
// of 32) -- ample behind bcrypt, which weft always writes.

// No easily confused letters (l, o), so 0 and 1 stay unambiguous; no y/z,
// which QWERTY and QWERTZ keyboards swap. No uppercase: nothing to shift.
export const LOWER = 'abcdefghijkmnpqrstuvwx'
export const DIGITS = '0123456789'

const LENGTH = 12
const BLOCK_SIZE = 4
const SEPARATOR = '.'

// randomIndex returns a uniformly distributed integer in [0, n), using
// rejection sampling to avoid modulo bias.
function randomIndex(n) {
  const limit = Math.floor(0x100000000 / n) * n
  const buf = new Uint32Array(1)
  for (;;) {
    crypto.getRandomValues(buf)
    if (buf[0] < limit) return buf[0] % n
  }
}

const pick = (alphabet) => alphabet[randomIndex(alphabet.length)]

// generatePassword returns a new password: one mandatory character per class,
// the rest from the combined pool, shuffled so the mandatory characters have no
// fixed position.
export function generatePassword() {
  const classes = [LOWER, DIGITS]
  const pool = classes.join('')
  const chars = classes.map(pick)
  while (chars.length < LENGTH) chars.push(pick(pool))
  // Fisher-Yates shuffle
  for (let i = chars.length - 1; i > 0; i--) {
    const j = randomIndex(i + 1)
    ;[chars[i], chars[j]] = [chars[j], chars[i]]
  }
  const blocks = []
  for (let i = 0; i < chars.length; i += BLOCK_SIZE) {
    blocks.push(chars.slice(i, i + BLOCK_SIZE).join(''))
  }
  return blocks.join(SEPARATOR)
}
