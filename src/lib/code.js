// A report's short code: six characters of Crockford's base32 (digits and capitals without I, L, O
// and U), shown as CET-7KQ2MX, from a hash of its slug. scripts/codes.mjs writes it into the report
// once, so it stays the same if the slug later changes. Every report must store its code.
import crypto from "node:crypto";

const ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
export function codeOf(seed) {
  const b = crypto.createHash("sha256").update(seed).digest();
  let n = ((b[0] << 22) | (b[1] << 14) | (b[2] << 6) | (b[3] >> 2)) >>> 0; // 30 bits
  let s = "";
  for (let i = 0; i < 6; i++) { s = ALPHABET[n & 31] + s; n >>>= 5; }
  return s;
}
