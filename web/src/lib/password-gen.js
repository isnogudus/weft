// Random, easy-to-type passwords, ported from the default mode of
// password-generator: 16 characters from lowercase, uppercase and digits, at
// least one of each, shown in blocks of four joined by "." (e.g.
// "x8GG.JpJN.LN40.t7qx"). The dots are part of the password. About 92 bits of
// entropy (16 characters from a pool of 54).

// No easily confused letters (l, o, I, O), so 0 and 1 stay unambiguous; no
// y/z/Y/Z, which QWERTY and QWERTZ keyboards swap.
export const LOWER = 'abcdefghijkmnpqrstuvwx'
export const UPPER = 'ABCDEFGHJKLMNPQRSTUVWX'
export const DIGITS = '0123456789'

const LENGTH = 16
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
  const classes = [LOWER, UPPER, DIGITS]
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
